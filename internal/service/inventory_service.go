package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"backend/internal/model"
	ws "backend/internal/websocket"

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
	CreateProduct(ctx context.Context, req CreateProductRequest) (ProductResponse, error)
	UpdateProduct(ctx context.Context, id string, req UpdateProductRequest) (ProductResponse, error)
	DeleteProduct(ctx context.Context, id string) error
	CreateOrder(ctx context.Context, req CreateOrderRequest) error
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

// CreateProduct creates a new product in the system
func (s *inventoryService) CreateProduct(ctx context.Context, req CreateProductRequest) (ProductResponse, error) {
	product := model.Product{
		SKU:          req.SKU,
		Name:         req.Name,
		Price:        req.Price,
		CurrentStock: 0,
	}

	if err := s.db.WithContext(ctx).Create(&product).Error; err != nil {
		// Basic check for unique violation loosely
		return ProductResponse{}, fmt.Errorf("failed to create product: %w", err)
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
func (s *inventoryService) UpdateProduct(ctx context.Context, id string, req UpdateProductRequest) (ProductResponse, error) {
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

	if err := s.db.WithContext(ctx).Save(&product).Error; err != nil {
		return ProductResponse{}, fmt.Errorf("failed to update product: %w", err)
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
func (s *inventoryService) DeleteProduct(ctx context.Context, id string) error {
	var product model.Product
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&product).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("product not found")
		}
		return fmt.Errorf("database error: %w", err)
	}

	if err := s.db.WithContext(ctx).Delete(&product).Error; err != nil {
		return fmt.Errorf("failed to delete product: %w", err)
	}

	return nil
}

// CreateOrder processes an IMPORT or EXPORT transaction within a strict ACID Boundary
func (s *inventoryService) CreateOrder(ctx context.Context, req CreateOrderRequest) error {
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
