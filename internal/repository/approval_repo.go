package repository

import (
	"context"

	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ApprovalRepository interface {
	Create(ctx context.Context, req *model.ApprovalRequest) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.ApprovalRequest, error)
	FindByIDWithRelations(ctx context.Context, id uuid.UUID) (*model.ApprovalRequest, error)
	List(ctx context.Context, status string, page, limit int) ([]model.ApprovalRequest, int64, error)
	Update(ctx context.Context, req *model.ApprovalRequest) error
}

type approvalRepository struct {
	db *gorm.DB
}

func NewApprovalRepository(db *gorm.DB) ApprovalRepository {
	return &approvalRepository{db: db}
}

func (r *approvalRepository) Create(ctx context.Context, req *model.ApprovalRequest) error {
	return GetDB(ctx, r.db).Create(req).Error
}

func (r *approvalRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.ApprovalRequest, error) {
	var req model.ApprovalRequest
	if err := GetDB(ctx, r.db).First(&req, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *approvalRepository) FindByIDWithRelations(ctx context.Context, id uuid.UUID) (*model.ApprovalRequest, error) {
	var req model.ApprovalRequest
	if err := GetDB(ctx, r.db).Preload("Requester").Preload("Approver").First(&req, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *approvalRepository) List(ctx context.Context, status string, page, limit int) ([]model.ApprovalRequest, int64, error) {
	var requests []model.ApprovalRequest
	var total int64

	db := GetDB(ctx, r.db)
	query := db.Model(&model.ApprovalRequest{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	fetchQuery := db.Preload("Requester").Preload("Approver")
	if status != "" {
		fetchQuery = fetchQuery.Where("status = ?", status)
	}
	if err := fetchQuery.Order("created_at DESC").Offset(offset).Limit(limit).Find(&requests).Error; err != nil {
		return nil, 0, err
	}

	return requests, total, nil
}

func (r *approvalRepository) Update(ctx context.Context, req *model.ApprovalRequest) error {
	return GetDB(ctx, r.db).Save(req).Error
}
