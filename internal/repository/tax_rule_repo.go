package repository

import (
	"context"
	"time"

	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TaxRuleRepository interface {
	Create(ctx context.Context, rule *model.TaxRule) error
	Update(ctx context.Context, rule *model.TaxRule) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.TaxRule, error)
	List(ctx context.Context, page, limit int) ([]model.TaxRule, int64, error)
	FindActiveByType(ctx context.Context, taxType string, targetDate time.Time) (*model.TaxRule, error)
	FindOverlapping(ctx context.Context, taxType string, from time.Time, to *time.Time, excludeID *uuid.UUID) (int64, error)
}

type taxRuleRepository struct {
	db *gorm.DB
}

func NewTaxRuleRepository(db *gorm.DB) TaxRuleRepository {
	return &taxRuleRepository{db: db}
}

func (r *taxRuleRepository) Create(ctx context.Context, rule *model.TaxRule) error {
	return GetDB(ctx, r.db).Create(rule).Error
}

func (r *taxRuleRepository) Update(ctx context.Context, rule *model.TaxRule) error {
	return GetDB(ctx, r.db).Save(rule).Error
}

func (r *taxRuleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return GetDB(ctx, r.db).Where("id = ?", id).Delete(&model.TaxRule{}).Error
}

func (r *taxRuleRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.TaxRule, error) {
	var rule model.TaxRule
	if err := GetDB(ctx, r.db).First(&rule, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *taxRuleRepository) List(ctx context.Context, page, limit int) ([]model.TaxRule, int64, error) {
	var rules []model.TaxRule
	var total int64

	db := GetDB(ctx, r.db)
	if err := db.Model(&model.TaxRule{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := db.Order("effective_from desc").Offset(offset).Limit(limit).Find(&rules).Error; err != nil {
		return nil, 0, err
	}

	return rules, total, nil
}

func (r *taxRuleRepository) FindActiveByType(ctx context.Context, taxType string, targetDate time.Time) (*model.TaxRule, error) {
	var rule model.TaxRule
	if err := GetDB(ctx, r.db).
		Where("tax_type = ? AND effective_from <= ? AND (effective_to IS NULL OR effective_to >= ?)", taxType, targetDate, targetDate).
		Order("effective_from DESC").
		First(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *taxRuleRepository) FindOverlapping(ctx context.Context, taxType string, from time.Time, to *time.Time, excludeID *uuid.UUID) (int64, error) {
	var count int64
	query := GetDB(ctx, r.db).Model(&model.TaxRule{}).Where("tax_type = ?", taxType)

	if excludeID != nil {
		query = query.Where("id != ?", *excludeID)
	}

	if to != nil {
		// New rule has end date: overlap if existing.from <= new.to AND (existing.to IS NULL OR existing.to >= new.from)
		query = query.Where("effective_from <= ? AND (effective_to IS NULL OR effective_to >= ?)", *to, from)
	} else {
		// New rule has no end date: overlap if (existing.to IS NULL OR existing.to >= new.from)
		query = query.Where("(effective_to IS NULL OR effective_to >= ?)", from)
	}

	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
