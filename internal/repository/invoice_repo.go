package repository

import (
	"context"

	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InvoiceRepository interface {
	Create(ctx context.Context, invoice *model.Invoice) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Invoice, error)
	FindByIDWithTaxRule(ctx context.Context, id uuid.UUID) (*model.Invoice, error)
	List(ctx context.Context, status string, page, limit int) ([]model.Invoice, int64, error)
	UpdateApproval(ctx context.Context, invoice *model.Invoice) error
	CountByPrefix(ctx context.Context, prefix string) (int64, error)
}

type invoiceRepository struct {
	db *gorm.DB
}

func NewInvoiceRepository(db *gorm.DB) InvoiceRepository {
	return &invoiceRepository{db: db}
}

func (r *invoiceRepository) Create(ctx context.Context, invoice *model.Invoice) error {
	return GetDB(ctx, r.db).Create(invoice).Error
}

func (r *invoiceRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Invoice, error) {
	var invoice model.Invoice
	if err := GetDB(ctx, r.db).First(&invoice, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &invoice, nil
}

func (r *invoiceRepository) FindByIDWithTaxRule(ctx context.Context, id uuid.UUID) (*model.Invoice, error) {
	var invoice model.Invoice
	if err := GetDB(ctx, r.db).Preload("TaxRule").First(&invoice, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &invoice, nil
}

func (r *invoiceRepository) List(ctx context.Context, status string, page, limit int) ([]model.Invoice, int64, error) {
	var invoices []model.Invoice
	var total int64

	db := GetDB(ctx, r.db)
	query := db.Model(&model.Invoice{})
	if status != "" {
		query = query.Where("approval_status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	fetchQuery := db.Preload("TaxRule")
	if status != "" {
		fetchQuery = fetchQuery.Where("approval_status = ?", status)
	}
	if err := fetchQuery.Order("created_at desc").Offset(offset).Limit(limit).Find(&invoices).Error; err != nil {
		return nil, 0, err
	}

	return invoices, total, nil
}

func (r *invoiceRepository) UpdateApproval(ctx context.Context, invoice *model.Invoice) error {
	return GetDB(ctx, r.db).Save(invoice).Error
}

func (r *invoiceRepository) CountByPrefix(ctx context.Context, prefix string) (int64, error) {
	var count int64
	if err := GetDB(ctx, r.db).Model(&model.Invoice{}).Where("invoice_no LIKE ?", prefix+"%").Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
