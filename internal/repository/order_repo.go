package repository

import (
	"context"

	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrderRepository interface {
	Create(ctx context.Context, order *model.Order) error
	CreateItem(ctx context.Context, item *model.OrderItem) error
	FindByIDWithItems(ctx context.Context, id uuid.UUID) (*model.Order, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
}

type orderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) OrderRepository {
	return &orderRepository{db: db}
}

func (r *orderRepository) Create(ctx context.Context, order *model.Order) error {
	return GetDB(ctx, r.db).Create(order).Error
}

func (r *orderRepository) CreateItem(ctx context.Context, item *model.OrderItem) error {
	return GetDB(ctx, r.db).Create(item).Error
}

func (r *orderRepository) FindByIDWithItems(ctx context.Context, id uuid.UUID) (*model.Order, error) {
	var order model.Order
	if err := GetDB(ctx, r.db).Preload("Items").First(&order, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *orderRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	return GetDB(ctx, r.db).Model(&model.Order{}).Where("id = ?", id).Update("status", status).Error
}
