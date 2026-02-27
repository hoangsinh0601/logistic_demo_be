package repository

import (
	"context"

	"backend/internal/model"

	"gorm.io/gorm"
)

type InventoryTxRepository interface {
	Create(ctx context.Context, tx *model.InventoryTransaction) error
}

type inventoryTxRepository struct {
	db *gorm.DB
}

func NewInventoryTxRepository(db *gorm.DB) InventoryTxRepository {
	return &inventoryTxRepository{db: db}
}

func (r *inventoryTxRepository) Create(ctx context.Context, tx *model.InventoryTransaction) error {
	return GetDB(ctx, r.db).Create(tx).Error
}
