package repository

import (
	"context"

	"backend/internal/model"

	"gorm.io/gorm"
)

type AuditRepository interface {
	Log(ctx context.Context, entry *model.AuditLog) error
	List(ctx context.Context, page, limit int) ([]model.AuditLog, int64, error)
}

type auditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) AuditRepository {
	return &auditRepository{db: db}
}

func (r *auditRepository) Log(ctx context.Context, entry *model.AuditLog) error {
	return GetDB(ctx, r.db).Create(entry).Error
}

func (r *auditRepository) List(ctx context.Context, page, limit int) ([]model.AuditLog, int64, error) {
	var logs []model.AuditLog
	var total int64

	db := GetDB(ctx, r.db)
	if err := db.Model(&model.AuditLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := db.Preload("User").Order("created_at desc").Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}
