package repository

import (
	"context"

	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ExpenseRepository interface {
	Create(ctx context.Context, expense *model.Expense) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Expense, error)
	List(ctx context.Context, page, limit int) ([]model.Expense, int64, error)
}

type expenseRepository struct {
	db *gorm.DB
}

func NewExpenseRepository(db *gorm.DB) ExpenseRepository {
	return &expenseRepository{db: db}
}

func (r *expenseRepository) Create(ctx context.Context, expense *model.Expense) error {
	return GetDB(ctx, r.db).Create(expense).Error
}

func (r *expenseRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Expense, error) {
	var expense model.Expense
	if err := GetDB(ctx, r.db).First(&expense, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &expense, nil
}

func (r *expenseRepository) List(ctx context.Context, page, limit int) ([]model.Expense, int64, error) {
	var expenses []model.Expense
	var total int64

	db := GetDB(ctx, r.db)
	if err := db.Model(&model.Expense{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := db.Order("created_at desc").Offset(offset).Limit(limit).Find(&expenses).Error; err != nil {
		return nil, 0, err
	}

	return expenses, total, nil
}
