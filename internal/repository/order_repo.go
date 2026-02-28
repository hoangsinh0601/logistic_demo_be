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
	List(ctx context.Context, page, limit int) ([]model.Order, int64, error)
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
	if err := GetDB(ctx, r.db).
		Preload("Items").
		Preload("Partner").
		Preload("Partner.Addresses").
		Preload("OriginAddress").
		Preload("ShippingAddress").
		First(&order, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *orderRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	return GetDB(ctx, r.db).Model(&model.Order{}).Where("id = ?", id).Update("status", status).Error
}

func (r *orderRepository) List(ctx context.Context, page, limit int) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64

	db := GetDB(ctx, r.db)
	if err := db.Model(&model.Order{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := db.
		Preload("Items").
		Preload("Partner").
		Preload("Partner.Addresses").
		Preload("OriginAddress").
		Preload("ShippingAddress").
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}
