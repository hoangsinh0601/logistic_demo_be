package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"backend/internal/model"
	ws "backend/internal/websocket"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DTOs
type OrderItemRequest struct {
	ProductID string  `json:"product_id" binding:"required"`
	Quantity  int     `json:"quantity" binding:"required,gt=0"`
	UnitPrice float64 `json:"unit_price" binding:"required,gt=0"`
}

type CreateOrderRequest struct {
	OrderCode string             `json:"order_code" binding:"required"`
	Type      string             `json:"type" binding:"required,oneof=IMPORT EXPORT"`
	Note      string             `json:"note"`
	Items     []OrderItemRequest `json:"items" binding:"required,min=1,dive"`
	TaxRuleID string             `json:"tax_rule_id"` // Optional: user-selected tax rule for invoice
	SideFees  string             `json:"side_fees"`   // Optional: additional fees
}

type CreateProductRequest struct {
	SKU   string  `json:"sku" binding:"required"`
	Name  string  `json:"name" binding:"required"`
	Price float64 `json:"price" binding:"required,min=0"`
}

type UpdateProductRequest struct {
	SKU   string  `json:"sku" binding:"required"`
	Name  string  `json:"name" binding:"required"`
	Price float64 `json:"price" binding:"required,min=0"`
}

type ProductResponse struct {
	ID           string  `json:"id"`
	SKU          string  `json:"sku"`
	Name         string  `json:"name"`
	CurrentStock int     `json:"current_stock"`
	Price        float64 `json:"price"`
}

// Websocket Payload
type InventoryEvent struct {
	Event string                 `json:"event"`
	Data  map[string]interface{} `json:"data"`
}

type InventoryService interface {
	GetProducts(ctx context.Context, page, limit int) ([]ProductResponse, int64, error)
	CreateProduct(ctx context.Context, userID string, req CreateProductRequest) (ProductResponse, error)
	UpdateProduct(ctx context.Context, userID string, id string, req UpdateProductRequest) (ProductResponse, error)
	DeleteProduct(ctx context.Context, userID string, id string) error
	CreateOrder(ctx context.Context, userID string, req CreateOrderRequest) error
}

type inventoryService struct {
	db  *gorm.DB
	hub *ws.Hub
}

// NewInventoryService returns a new instance of InventoryService
func NewInventoryService(db *gorm.DB, hub *ws.Hub) InventoryService {
	return &inventoryService{db: db, hub: hub}
}

// GetProducts returns paginated products with current stock
func (s *inventoryService) GetProducts(ctx context.Context, page, limit int) ([]ProductResponse, int64, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&model.Product{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	offset := (page - 1) * limit

	var products []model.Product
	if err := s.db.WithContext(ctx).Order("created_at DESC").Offset(offset).Limit(limit).Find(&products).Error; err != nil {
		return nil, 0, err
	}

	res := make([]ProductResponse, 0, len(products))
	for _, p := range products {
		res = append(res, ProductResponse{
			ID:           p.ID.String(),
			SKU:          p.SKU,
			Name:         p.Name,
			CurrentStock: p.CurrentStock,
			Price:        p.Price,
		})
	}

	return res, total, nil
}

// CreateProduct creates a new product in the system and logs the action
func (s *inventoryService) CreateProduct(ctx context.Context, userID string, req CreateProductRequest) (ProductResponse, error) {
	product := model.Product{
		SKU:          req.SKU,
		Name:         req.Name,
		Price:        req.Price,
		CurrentStock: 0,
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&product).Error; err != nil {
			return fmt.Errorf("failed to create product: %w", err)
		}

		// Log Action
		var uid *uuid.UUID
		if parsed, err := uuid.Parse(userID); err == nil {
			uid = &parsed
		}

		details, _ := json.Marshal(req)
		audit := model.AuditLog{
			UserID:     uid,
			Action:     model.ActionCreateProduct,
			EntityID:   product.ID.String(),
			EntityName: product.Name,
			Details:    string(details),
		}
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("failed to write audit log: %w", err)
		}

		return nil
	})

	if err != nil {
		return ProductResponse{}, err
	}

	return ProductResponse{
		ID:           product.ID.String(),
		SKU:          product.SKU,
		Name:         product.Name,
		CurrentStock: product.CurrentStock,
		Price:        product.Price,
	}, nil
}

// UpdateProduct updates product details like price and name, excluding stock mutations
func (s *inventoryService) UpdateProduct(ctx context.Context, userID string, id string, req UpdateProductRequest) (ProductResponse, error) {
	var product model.Product
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&product).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ProductResponse{}, errors.New("product not found")
		}
		return ProductResponse{}, fmt.Errorf("database error: %w", err)
	}

	product.SKU = req.SKU
	product.Name = req.Name
	product.Price = req.Price

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&product).Error; err != nil {
			return fmt.Errorf("failed to update product: %w", err)
		}

		var uid *uuid.UUID
		if parsed, err := uuid.Parse(userID); err == nil {
			uid = &parsed
		}

		details, _ := json.Marshal(req)
		audit := model.AuditLog{
			UserID:     uid,
			Action:     model.ActionUpdateProduct,
			EntityID:   product.ID.String(),
			EntityName: product.Name,
			Details:    string(details),
		}
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("failed to write audit log: %w", err)
		}
		return nil
	})

	if err != nil {
		return ProductResponse{}, err
	}

	return ProductResponse{
		ID:           product.ID.String(),
		SKU:          product.SKU,
		Name:         product.Name,
		CurrentStock: product.CurrentStock,
		Price:        product.Price,
	}, nil
}

// DeleteProduct soft-deletes the product from the database
func (s *inventoryService) DeleteProduct(ctx context.Context, userID string, id string) error {
	var product model.Product
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&product).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("product not found")
		}
		return fmt.Errorf("database error: %w", err)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&product).Error; err != nil {
			return fmt.Errorf("failed to delete product: %w", err)
		}

		var uid *uuid.UUID
		if parsed, err := uuid.Parse(userID); err == nil {
			uid = &parsed
		}

		audit := model.AuditLog{
			UserID:     uid,
			Action:     model.ActionDeleteProduct,
			EntityID:   product.ID.String(),
			EntityName: product.Name,
			Details:    `{"deleted": true}`,
		}
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("failed to write audit log: %w", err)
		}
		return nil
	})
}

// CreateOrder creates an order with PENDING_APPROVAL status and an ApprovalRequest.
// Stock updates, inventory transactions, and invoice creation are deferred to the approval workflow.
func (s *inventoryService) CreateOrder(ctx context.Context, userID string, req CreateOrderRequest) error {
	// Start a Database Transaction
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Check if OrderCode already exists
		var existing model.Order
		if err := tx.Where("order_code = ?", req.OrderCode).First(&existing).Error; err == nil {
			return errors.New("order_code already exists")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// 2. Insert into `orders` with PENDING_APPROVAL status
		order := model.Order{
			OrderCode: req.OrderCode,
			Type:      req.Type,
			Note:      req.Note,
			Status:    model.OrderStatusPendingApproval,
		}
		if err := tx.Create(&order).Error; err != nil {
			return fmt.Errorf("failed to create order: %w", err)
		}

		// 3. Validate products and insert order items (NO stock update yet)
		var productNames []string
		type OrderItemAudit struct {
			ProductID   string  `json:"product_id"`
			ProductName string  `json:"product_name"`
			Quantity    int     `json:"quantity"`
			UnitPrice   float64 `json:"unit_price"`
		}
		var auditItems []OrderItemAudit

		for _, itemReq := range req.Items {
			var product model.Product

			// Validate product exists (no locking needed since we don't update stock)
			if err := tx.Where("id = ?", itemReq.ProductID).First(&product).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("product not found: %s", itemReq.ProductID)
				}
				return fmt.Errorf("failed to find product %s: %w", itemReq.ProductID, err)
			}

			productNames = append(productNames, product.Name)
			auditItems = append(auditItems, OrderItemAudit{
				ProductID:   itemReq.ProductID,
				ProductName: product.Name,
				Quantity:    itemReq.Quantity,
				UnitPrice:   itemReq.UnitPrice,
			})

			// Insert order item
			orderItem := model.OrderItem{
				OrderID:   order.ID,
				ProductID: product.ID,
				Quantity:  itemReq.Quantity,
				UnitPrice: itemReq.UnitPrice,
			}
			if err := tx.Create(&orderItem).Error; err != nil {
				return fmt.Errorf("failed to create order item: %w", err)
			}
		}

		// 4. Insert Audit Log for Order
		var uid *uuid.UUID
		if parsed, err := uuid.Parse(userID); err == nil {
			uid = &parsed
		}

		actionType := model.ActionCreateOrderIn
		if req.Type == model.OrderTypeExport {
			actionType = model.ActionCreateOrderOut
		}

		auditDetails := map[string]interface{}{
			"order_code": req.OrderCode,
			"type":       req.Type,
			"note":       req.Note,
			"items":      auditItems,
		}
		details, _ := json.Marshal(auditDetails)
		audit := model.AuditLog{
			UserID:     uid,
			Action:     actionType,
			EntityID:   order.ID.String(),
			EntityName: strings.Join(productNames, ", "),
			Details:    string(details),
		}
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("failed to record audit transaction: %w", err)
		}

		// 5. Create ApprovalRequest for this order
		requestData, _ := json.Marshal(map[string]interface{}{
			"order_code":  req.OrderCode,
			"type":        req.Type,
			"note":        req.Note,
			"items":       auditItems,
			"tax_rule_id": req.TaxRuleID,
			"side_fees":   req.SideFees,
		})

		approvalReq := model.ApprovalRequest{
			RequestType: model.ApprovalReqTypeCreateOrder,
			ReferenceID: order.ID,
			RequestData: string(requestData),
			Status:      model.ApprovalPending,
			RequestedBy: uid,
		}
		if err := tx.Create(&approvalReq).Error; err != nil {
			return fmt.Errorf("failed to create approval request: %w", err)
		}

		// 6. Audit log for approval request creation
		approvalDetails, _ := json.Marshal(map[string]interface{}{
			"request_type": model.ApprovalReqTypeCreateOrder,
			"reference_id": order.ID.String(),
			"order_code":   req.OrderCode,
		})
		approvalAudit := model.AuditLog{
			UserID:     uid,
			Action:     model.ActionCreateApprovalRequest,
			EntityID:   approvalReq.ID.String(),
			EntityName: model.ApprovalReqTypeCreateOrder,
			Details:    string(approvalDetails),
		}
		if err := tx.Create(&approvalAudit).Error; err != nil {
			return fmt.Errorf("failed to record approval audit: %w", err)
		}

		return nil
	})

	return err
}
