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
	List(ctx context.Context, filter InvoiceListFilter) ([]model.Invoice, int64, error)
	UpdateApproval(ctx context.Context, invoice *model.Invoice) error
	Update(ctx context.Context, invoice *model.Invoice) error
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
	if err := GetDB(ctx, r.db).Preload("TaxRule").Preload("Partner").First(&invoice, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &invoice, nil
}

// InvoiceListFilter holds filters for listing invoices
type InvoiceListFilter struct {
	ApprovalStatus string
	InvoiceNo      string
	ReferenceType  string
	Page           int
	Limit          int
}

func (r *invoiceRepository) List(ctx context.Context, filter InvoiceListFilter) ([]model.Invoice, int64, error) {
	var invoices []model.Invoice
	var total int64

	db := GetDB(ctx, r.db)
	query := db.Model(&model.Invoice{})
	if filter.ApprovalStatus != "" {
		query = query.Where("approval_status = ?", filter.ApprovalStatus)
	}
	if filter.InvoiceNo != "" {
		query = query.Where("invoice_no ILIKE ?", "%"+filter.InvoiceNo+"%")
	}
	if filter.ReferenceType != "" {
		query = query.Where("reference_type = ?", filter.ReferenceType)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.Limit
	fetchQuery := db.Preload("TaxRule").Preload("Partner")
	if filter.ApprovalStatus != "" {
		fetchQuery = fetchQuery.Where("approval_status = ?", filter.ApprovalStatus)
	}
	if filter.InvoiceNo != "" {
		fetchQuery = fetchQuery.Where("invoice_no ILIKE ?", "%"+filter.InvoiceNo+"%")
	}
	if filter.ReferenceType != "" {
		fetchQuery = fetchQuery.Where("reference_type = ?", filter.ReferenceType)
	}
	if err := fetchQuery.Order("created_at desc").Offset(offset).Limit(filter.Limit).Find(&invoices).Error; err != nil {
		return nil, 0, err
	}

	return invoices, total, nil
}

func (r *invoiceRepository) UpdateApproval(ctx context.Context, invoice *model.Invoice) error {
	return GetDB(ctx, r.db).Save(invoice).Error
}

func (r *invoiceRepository) Update(ctx context.Context, invoice *model.Invoice) error {
	return GetDB(ctx, r.db).Save(invoice).Error
}

func (r *invoiceRepository) CountByPrefix(ctx context.Context, prefix string) (int64, error) {
	var count int64
	if err := GetDB(ctx, r.db).Model(&model.Invoice{}).Where("invoice_no LIKE ?", prefix+"%").Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
