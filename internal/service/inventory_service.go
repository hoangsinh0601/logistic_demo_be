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
	"gorm.io/gorm/clause"
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
	GetProducts(ctx context.Context) ([]ProductResponse, error)
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

// GetProducts limits results to current stock lookup
func (s *inventoryService) GetProducts(ctx context.Context) ([]ProductResponse, error) {
	var products []model.Product
	if err := s.db.WithContext(ctx).Find(&products).Error; err != nil {
		return nil, err
	}

	var res []ProductResponse
	for _, p := range products {
		res = append(res, ProductResponse{
			ID:           p.ID.String(),
			SKU:          p.SKU,
			Name:         p.Name,
			CurrentStock: p.CurrentStock,
			Price:        p.Price,
		})
	}

	return res, nil
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

// CreateOrder processes an IMPORT or EXPORT transaction within a strict ACID Boundary
func (s *inventoryService) CreateOrder(ctx context.Context, userID string, req CreateOrderRequest) error {
	type wsUpdate struct {
		ProductID string
		NewStock  int
	}
	var updates []wsUpdate

	// Start a Database Transaction
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Check if OrderCode already exists
		var existing model.Order
		if err := tx.Where("order_code = ?", req.OrderCode).First(&existing).Error; err == nil {
			return errors.New("order_code already exists")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// 2. Insert into `orders`
		order := model.Order{
			OrderCode: req.OrderCode,
			Type:      req.Type,
			Note:      req.Note,
			Status:    "COMPLETED", // Assuming orders complete instantly for now
		}
		if err := tx.Create(&order).Error; err != nil {
			return fmt.Errorf("failed to create order: %w", err)
		}

		// 3. Process each Order Item
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

			// Lock the product row for UPDATE using `clause.Locking` to guarantee consistency under concurrency
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", itemReq.ProductID).First(&product).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("product not found: %s", itemReq.ProductID)
				}
				return fmt.Errorf("failed to lock product %s: %w", itemReq.ProductID, err)
			}

			// Validate Export capacity
			if req.Type == model.OrderTypeExport && product.CurrentStock < itemReq.Quantity {
				return fmt.Errorf("insufficient stock for product %s (current: %d, requested: %d)", product.Name, product.CurrentStock, itemReq.Quantity)
			}

			// Add to product names array
			productNames = append(productNames, product.Name)

			// Store item details for audit logging
			auditItems = append(auditItems, OrderItemAudit{
				ProductID:   itemReq.ProductID,
				ProductName: product.Name,
				Quantity:    itemReq.Quantity,
				UnitPrice:   itemReq.UnitPrice,
			})

			// 4. Insert into `order_items`
			orderItem := model.OrderItem{
				OrderID:   order.ID,
				ProductID: product.ID,
				Quantity:  itemReq.Quantity,
				UnitPrice: itemReq.UnitPrice,
			}
			if err := tx.Create(&orderItem).Error; err != nil {
				return fmt.Errorf("failed to create order item: %w", err)
			}

			// 5. Calculate new stock
			modifier := 1
			if req.Type == model.OrderTypeExport {
				modifier = -1
			}

			quantityChanged := itemReq.Quantity * modifier
			stockAfter := product.CurrentStock + quantityChanged

			// 6. Update `current_stock` in `products` record
			if err := tx.Model(&product).Update("current_stock", stockAfter).Error; err != nil {
				return fmt.Errorf("failed to update stock for product %s: %w", product.Name, err)
			}

			// 7. Insert into `inventory_transactions`
			txType := model.TxTypeIn
			if req.Type == model.OrderTypeExport {
				txType = model.TxTypeOut
			}

			invTx := model.InventoryTransaction{
				ProductID:       product.ID,
				OrderID:         &order.ID,
				TransactionType: txType,
				QuantityChanged: quantityChanged,
				StockAfter:      stockAfter,
			}
			if err := tx.Create(&invTx).Error; err != nil {
				return fmt.Errorf("failed to record inventory transaction: %w", err)
			}

			// Stage WS Broadcast payload
			updates = append(updates, wsUpdate{
				ProductID: product.ID.String(),
				NewStock:  stockAfter,
			})
		}

		// Insert Audit Log for Order Creating Hook
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

		// 8. Commit Transaction (Triggered automatically by returning nil in GORM's Transaction helper)
		return nil
	})

	// 9. After successful commit, Broadcast WebSocket Events
	if err == nil && s.hub != nil {
		for _, u := range updates {
			msg := InventoryEvent{
				Event: "INVENTORY_UPDATED",
				Data: map[string]interface{}{
					"product_id": u.ProductID,
					"new_stock":  u.NewStock,
				},
			}
			payload, _ := json.Marshal(msg)

			// Send asynchronously
			go func(data []byte) {
				s.hub.Broadcast <- data
			}(payload)
		}
	}

	return err
}
