package service

import (
	"context"
	"fmt"
	"time"

	"backend/internal/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// --- DTOs ---

type CreateInvoiceRequest struct {
	ReferenceType string `json:"reference_type" binding:"required,oneof=ORDER_IMPORT ORDER_EXPORT EXPENSE"`
	ReferenceID   string `json:"reference_id" binding:"required"`
	TaxRuleID     string `json:"tax_rule_id"` // Optional: user-selected tax rule
	Subtotal      string `json:"subtotal" binding:"required"`
	SideFees      string `json:"side_fees"` // Optional, defaults to 0
	Note          string `json:"note"`
}

type InvoiceFilter struct {
	ApprovalStatus string // PENDING, APPROVED, REJECTED or empty for all
	Page           int
	Limit          int
}

type InvoiceResponse struct {
	ID             string  `json:"id"`
	InvoiceNo      string  `json:"invoice_no"`
	ReferenceType  string  `json:"reference_type"`
	ReferenceID    string  `json:"reference_id"`
	TaxRuleID      *string `json:"tax_rule_id"`
	TaxType        *string `json:"tax_type"`
	TaxRate        *string `json:"tax_rate"`
	Subtotal       string  `json:"subtotal"`
	TaxAmount      string  `json:"tax_amount"`
	SideFees       string  `json:"side_fees"`
	TotalAmount    string  `json:"total_amount"`
	ApprovalStatus string  `json:"approval_status"`
	ApprovedBy     *string `json:"approved_by"`
	ApprovedAt     *string `json:"approved_at"`
	Note           string  `json:"note"`
	CreatedAt      string  `json:"created_at"`
}

// --- Interface ---

type InvoiceService interface {
	CreateInvoice(ctx context.Context, req CreateInvoiceRequest) (InvoiceResponse, error)
	ListInvoices(ctx context.Context, filter InvoiceFilter) ([]InvoiceResponse, int64, error)
	ApproveInvoice(ctx context.Context, id string, userID string) (InvoiceResponse, error)
	RejectInvoice(ctx context.Context, id string, userID string) (InvoiceResponse, error)
}

type invoiceService struct {
	db *gorm.DB
}

func NewInvoiceService(db *gorm.DB) InvoiceService {
	return &invoiceService{db: db}
}

// --- Implementation ---

func (s *invoiceService) CreateInvoice(ctx context.Context, req CreateInvoiceRequest) (InvoiceResponse, error) {
	// Parse subtotal
	subtotal, err := decimal.NewFromString(req.Subtotal)
	if err != nil {
		return InvoiceResponse{}, fmt.Errorf("invalid subtotal: %w", err)
	}

	// Parse side_fees (default to 0)
	sideFees := decimal.Zero
	if req.SideFees != "" {
		sideFees, err = decimal.NewFromString(req.SideFees)
		if err != nil {
			return InvoiceResponse{}, fmt.Errorf("invalid side_fees: %w", err)
		}
	}

	// Parse reference_id
	refID, err := uuid.Parse(req.ReferenceID)
	if err != nil {
		return InvoiceResponse{}, fmt.Errorf("invalid reference_id: %w", err)
	}

	// Validate reference exists
	switch req.ReferenceType {
	case model.RefTypeOrderImport, model.RefTypeOrderExport:
		var order model.Order
		if err := s.db.WithContext(ctx).First(&order, "id = ?", refID).Error; err != nil {
			return InvoiceResponse{}, fmt.Errorf("referenced order not found: %w", err)
		}
	case model.RefTypeExpense:
		var expense model.Expense
		if err := s.db.WithContext(ctx).First(&expense, "id = ?", refID).Error; err != nil {
			return InvoiceResponse{}, fmt.Errorf("referenced expense not found: %w", err)
		}
	}

	// Calculate tax
	taxAmount := decimal.Zero
	var taxRuleID *uuid.UUID
	if req.TaxRuleID != "" {
		parsed, parseErr := uuid.Parse(req.TaxRuleID)
		if parseErr != nil {
			return InvoiceResponse{}, fmt.Errorf("invalid tax_rule_id: %w", parseErr)
		}
		taxRuleID = &parsed

		// Fetch tax rule rate
		var taxRule model.TaxRule
		if err := s.db.WithContext(ctx).First(&taxRule, "id = ?", parsed).Error; err != nil {
			return InvoiceResponse{}, fmt.Errorf("tax rule not found: %w", err)
		}
		taxAmount = subtotal.Mul(taxRule.Rate)
	}

	totalAmount := subtotal.Add(taxAmount).Add(sideFees)

	// Generate invoice number: INV-YYYYMMDD-XXXXX
	invoiceNo, err := s.generateInvoiceNo(ctx)
	if err != nil {
		return InvoiceResponse{}, fmt.Errorf("failed to generate invoice number: %w", err)
	}

	invoice := model.Invoice{
		InvoiceNo:      invoiceNo,
		ReferenceType:  req.ReferenceType,
		ReferenceID:    refID,
		TaxRuleID:      taxRuleID,
		Subtotal:       subtotal,
		TaxAmount:      taxAmount,
		SideFees:       sideFees,
		TotalAmount:    totalAmount,
		ApprovalStatus: model.ApprovalPending,
		Note:           req.Note,
	}

	if err := s.db.WithContext(ctx).Create(&invoice).Error; err != nil {
		return InvoiceResponse{}, fmt.Errorf("failed to create invoice: %w", err)
	}

	// Reload with relations
	if err := s.db.WithContext(ctx).Preload("TaxRule").First(&invoice, "id = ?", invoice.ID).Error; err != nil {
		return InvoiceResponse{}, fmt.Errorf("failed to reload invoice: %w", err)
	}

	return toInvoiceResponse(invoice), nil
}

func (s *invoiceService) ListInvoices(ctx context.Context, filter InvoiceFilter) ([]InvoiceResponse, int64, error) {
	var total int64
	countQuery := s.db.WithContext(ctx).Model(&model.Invoice{})
	if filter.ApprovalStatus != "" {
		countQuery = countQuery.Where("approval_status = ?", filter.ApprovalStatus)
	}
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count invoices: %w", err)
	}

	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	offset := (filter.Page - 1) * filter.Limit

	query := s.db.WithContext(ctx).Preload("TaxRule").Order("created_at DESC").Offset(offset).Limit(filter.Limit)
	if filter.ApprovalStatus != "" {
		query = query.Where("approval_status = ?", filter.ApprovalStatus)
	}

	var invoices []model.Invoice
	if err := query.Find(&invoices).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch invoices: %w", err)
	}

	result := make([]InvoiceResponse, 0, len(invoices))
	for _, inv := range invoices {
		result = append(result, toInvoiceResponse(inv))
	}
	return result, total, nil
}

func (s *invoiceService) ApproveInvoice(ctx context.Context, id string, userID string) (InvoiceResponse, error) {
	return s.updateApproval(ctx, id, userID, model.ApprovalApproved)
}

func (s *invoiceService) RejectInvoice(ctx context.Context, id string, userID string) (InvoiceResponse, error) {
	return s.updateApproval(ctx, id, userID, model.ApprovalRejected)
}

func (s *invoiceService) updateApproval(ctx context.Context, id string, userID string, status string) (InvoiceResponse, error) {
	invoiceID, err := uuid.Parse(id)
	if err != nil {
		return InvoiceResponse{}, fmt.Errorf("invalid invoice id: %w", err)
	}

	approverID, err := uuid.Parse(userID)
	if err != nil {
		return InvoiceResponse{}, fmt.Errorf("invalid user id: %w", err)
	}

	var invoice model.Invoice
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the row to prevent concurrent approval
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invoice, "id = ?", invoiceID).Error; err != nil {
			return fmt.Errorf("invoice not found: %w", err)
		}

		if invoice.ApprovalStatus != model.ApprovalPending {
			return fmt.Errorf("invoice is already %s", invoice.ApprovalStatus)
		}

		now := time.Now()
		invoice.ApprovalStatus = status
		invoice.ApprovedBy = &approverID
		invoice.ApprovedAt = &now

		if err := tx.Save(&invoice).Error; err != nil {
			return fmt.Errorf("failed to update invoice: %w", err)
		}

		return nil
	})

	if err != nil {
		return InvoiceResponse{}, err
	}

	// Reload with relations outside transaction
	if err := s.db.WithContext(ctx).Preload("TaxRule").First(&invoice, "id = ?", invoice.ID).Error; err != nil {
		return InvoiceResponse{}, fmt.Errorf("failed to reload invoice: %w", err)
	}

	return toInvoiceResponse(invoice), nil
}

func (s *invoiceService) generateInvoiceNo(ctx context.Context) (string, error) {
	today := time.Now().Format("20060102")
	prefix := "INV-" + today + "-"

	// Use advisory lock to prevent concurrent duplicate invoice numbers
	var count int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Acquire advisory lock based on hash of today's date
		tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", prefix)

		if err := tx.Model(&model.Invoice{}).
			Where("invoice_no LIKE ?", prefix+"%").
			Count(&count).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s%05d", prefix, count+1), nil
}

// --- Helpers ---

func toInvoiceResponse(inv model.Invoice) InvoiceResponse {
	resp := InvoiceResponse{
		ID:             inv.ID.String(),
		InvoiceNo:      inv.InvoiceNo,
		ReferenceType:  inv.ReferenceType,
		ReferenceID:    inv.ReferenceID.String(),
		Subtotal:       inv.Subtotal.StringFixed(4),
		TaxAmount:      inv.TaxAmount.StringFixed(4),
		SideFees:       inv.SideFees.StringFixed(4),
		TotalAmount:    inv.TotalAmount.StringFixed(4),
		ApprovalStatus: inv.ApprovalStatus,
		Note:           inv.Note,
		CreatedAt:      inv.CreatedAt.Format(time.RFC3339),
	}

	if inv.TaxRuleID != nil {
		s := inv.TaxRuleID.String()
		resp.TaxRuleID = &s
	}
	if inv.TaxRule != nil {
		resp.TaxType = &inv.TaxRule.TaxType
		rate := inv.TaxRule.Rate.StringFixed(4)
		resp.TaxRate = &rate
	}
	if inv.ApprovedBy != nil {
		s := inv.ApprovedBy.String()
		resp.ApprovedBy = &s
	}
	if inv.ApprovedAt != nil {
		s := inv.ApprovedAt.Format(time.RFC3339)
		resp.ApprovedAt = &s
	}

	return resp
}
