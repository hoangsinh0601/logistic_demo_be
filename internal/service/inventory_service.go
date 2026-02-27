package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"backend/internal/model"
	"backend/internal/repository"
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
	productRepo  repository.ProductRepository
	orderRepo    repository.OrderRepository
	approvalRepo repository.ApprovalRepository
	auditRepo    repository.AuditRepository
	txManager    repository.TransactionManager
	hub          *ws.Hub
}

func NewInventoryService(
	productRepo repository.ProductRepository,
	orderRepo repository.OrderRepository,
	approvalRepo repository.ApprovalRepository,
	auditRepo repository.AuditRepository,
	txManager repository.TransactionManager,
	hub *ws.Hub,
) InventoryService {
	return &inventoryService{
		productRepo:  productRepo,
		orderRepo:    orderRepo,
		approvalRepo: approvalRepo,
		auditRepo:    auditRepo,
		txManager:    txManager,
		hub:          hub,
	}
}

func (s *inventoryService) GetProducts(ctx context.Context, page, limit int) ([]ProductResponse, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	products, total, err := s.productRepo.List(ctx, page, limit)
	if err != nil {
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

func (s *inventoryService) CreateProduct(ctx context.Context, userID string, req CreateProductRequest) (ProductResponse, error) {
	product := model.Product{
		SKU:          req.SKU,
		Name:         req.Name,
		Price:        req.Price,
		CurrentStock: 0,
	}

	err := s.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.productRepo.Create(txCtx, &product); err != nil {
			return fmt.Errorf("failed to create product: %w", err)
		}

		var uid *uuid.UUID
		if parsed, err := uuid.Parse(userID); err == nil {
			uid = &parsed
		}

		details, _ := json.Marshal(req)
		audit := &model.AuditLog{
			UserID:     uid,
			Action:     model.ActionCreateProduct,
			EntityID:   product.ID.String(),
			EntityName: product.Name,
			Details:    string(details),
		}
		if err := s.auditRepo.Log(txCtx, audit); err != nil {
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

func (s *inventoryService) UpdateProduct(ctx context.Context, userID string, id string, req UpdateProductRequest) (ProductResponse, error) {
	productID, err := uuid.Parse(id)
	if err != nil {
		return ProductResponse{}, fmt.Errorf("invalid product id: %w", err)
	}

	product, err := s.productRepo.FindByID(ctx, productID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ProductResponse{}, errors.New("product not found")
		}
		return ProductResponse{}, fmt.Errorf("database error: %w", err)
	}

	product.SKU = req.SKU
	product.Name = req.Name
	product.Price = req.Price

	err = s.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.productRepo.Update(txCtx, product); err != nil {
			return fmt.Errorf("failed to update product: %w", err)
		}

		var uid *uuid.UUID
		if parsed, err := uuid.Parse(userID); err == nil {
			uid = &parsed
		}

		details, _ := json.Marshal(req)
		audit := &model.AuditLog{
			UserID:     uid,
			Action:     model.ActionUpdateProduct,
			EntityID:   product.ID.String(),
			EntityName: product.Name,
			Details:    string(details),
		}
		if err := s.auditRepo.Log(txCtx, audit); err != nil {
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

func (s *inventoryService) DeleteProduct(ctx context.Context, userID string, id string) error {
	productID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid product id: %w", err)
	}

	product, err := s.productRepo.FindByID(ctx, productID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("product not found")
		}
		return fmt.Errorf("database error: %w", err)
	}

	return s.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.productRepo.Delete(txCtx, productID); err != nil {
			return fmt.Errorf("failed to delete product: %w", err)
		}

		var uid *uuid.UUID
		if parsed, err := uuid.Parse(userID); err == nil {
			uid = &parsed
		}

		audit := &model.AuditLog{
			UserID:     uid,
			Action:     model.ActionDeleteProduct,
			EntityID:   product.ID.String(),
			EntityName: product.Name,
			Details:    `{"deleted": true}`,
		}
		if err := s.auditRepo.Log(txCtx, audit); err != nil {
			return fmt.Errorf("failed to write audit log: %w", err)
		}
		return nil
	})
}

func (s *inventoryService) CreateOrder(ctx context.Context, userID string, req CreateOrderRequest) error {
	return s.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		// 1. Validate product exists for each item
		var productNames []string
		type OrderItemAudit struct {
			ProductID   string  `json:"product_id"`
			ProductName string  `json:"product_name"`
			Quantity    int     `json:"quantity"`
			UnitPrice   float64 `json:"unit_price"`
		}
		var auditItems []OrderItemAudit

		for _, itemReq := range req.Items {
			pid, parseErr := uuid.Parse(itemReq.ProductID)
			if parseErr != nil {
				return fmt.Errorf("invalid product_id: %w", parseErr)
			}
			product, findErr := s.productRepo.FindByID(txCtx, pid)
			if findErr != nil {
				if errors.Is(findErr, gorm.ErrRecordNotFound) {
					return fmt.Errorf("product not found: %s", itemReq.ProductID)
				}
				return fmt.Errorf("failed to find product %s: %w", itemReq.ProductID, findErr)
			}

			productNames = append(productNames, product.Name)
			auditItems = append(auditItems, OrderItemAudit{
				ProductID:   itemReq.ProductID,
				ProductName: product.Name,
				Quantity:    itemReq.Quantity,
				UnitPrice:   itemReq.UnitPrice,
			})
		}

		// 2. Create order
		order := model.Order{
			OrderCode: req.OrderCode,
			Type:      req.Type,
			Note:      req.Note,
			Status:    model.OrderStatusPendingApproval,
		}
		if err := s.orderRepo.Create(txCtx, &order); err != nil {
			return fmt.Errorf("failed to create order: %w", err)
		}

		// 3. Create order items
		for _, itemReq := range req.Items {
			pid, _ := uuid.Parse(itemReq.ProductID)
			orderItem := &model.OrderItem{
				OrderID:   order.ID,
				ProductID: pid,
				Quantity:  itemReq.Quantity,
				UnitPrice: itemReq.UnitPrice,
			}
			if err := s.orderRepo.CreateItem(txCtx, orderItem); err != nil {
				return fmt.Errorf("failed to create order item: %w", err)
			}
		}

		// 4. Audit log
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
		audit := &model.AuditLog{
			UserID:     uid,
			Action:     actionType,
			EntityID:   order.ID.String(),
			EntityName: strings.Join(productNames, ", "),
			Details:    string(details),
		}
		if err := s.auditRepo.Log(txCtx, audit); err != nil {
			return fmt.Errorf("failed to record audit transaction: %w", err)
		}

		// 5. Create ApprovalRequest
		requestData, _ := json.Marshal(map[string]interface{}{
			"order_code":  req.OrderCode,
			"type":        req.Type,
			"note":        req.Note,
			"items":       auditItems,
			"tax_rule_id": req.TaxRuleID,
			"side_fees":   req.SideFees,
		})

		approvalReq := &model.ApprovalRequest{
			RequestType: model.ApprovalReqTypeCreateOrder,
			ReferenceID: order.ID,
			RequestData: string(requestData),
			Status:      model.ApprovalPending,
			RequestedBy: uid,
		}
		if err := s.approvalRepo.Create(txCtx, approvalReq); err != nil {
			return fmt.Errorf("failed to create approval request: %w", err)
		}

		// 6. Audit log for approval request
		approvalDetails, _ := json.Marshal(map[string]interface{}{
			"request_type": model.ApprovalReqTypeCreateOrder,
			"reference_id": order.ID.String(),
			"order_code":   req.OrderCode,
		})
		approvalAudit := &model.AuditLog{
			UserID:     uid,
			Action:     model.ActionCreateApprovalRequest,
			EntityID:   approvalReq.ID.String(),
			EntityName: model.ApprovalReqTypeCreateOrder,
			Details:    string(approvalDetails),
		}
		if err := s.auditRepo.Log(txCtx, approvalAudit); err != nil {
			return fmt.Errorf("failed to record approval audit: %w", err)
		}

		return nil
	})
}
