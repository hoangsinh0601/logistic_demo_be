package service

import (
	"context"
	"fmt"
	"time"

	"backend/internal/model"
	"backend/internal/repository"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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
	InvoiceNo      string // partial match on invoice_no
	ReferenceType  string // ORDER_IMPORT, ORDER_EXPORT, EXPENSE or empty for all
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
	PartnerID      *string `json:"partner_id"`
	CompanyName    string  `json:"company_name"`
	TaxCode        string  `json:"tax_code"`
	BillingAddress string  `json:"billing_address"`
	CreatedAt      string  `json:"created_at"`
}

// UpdateInvoiceRequest allows editing partner hard-copy fields on PENDING invoices
type UpdateInvoiceRequest struct {
	CompanyName    *string `json:"company_name"`
	TaxCode        *string `json:"tax_code"`
	BillingAddress *string `json:"billing_address"`
	Note           *string `json:"note"`
}

// --- Interface ---

type InvoiceService interface {
	CreateInvoice(ctx context.Context, req CreateInvoiceRequest) (InvoiceResponse, error)
	ListInvoices(ctx context.Context, filter InvoiceFilter) ([]InvoiceResponse, int64, error)
	ApproveInvoice(ctx context.Context, id string, userID string) (InvoiceResponse, error)
	RejectInvoice(ctx context.Context, id string, userID string) (InvoiceResponse, error)
	UpdateInvoice(ctx context.Context, id string, req UpdateInvoiceRequest) (InvoiceResponse, error)
}

type invoiceService struct {
	invoiceRepo repository.InvoiceRepository
	taxRuleRepo repository.TaxRuleRepository
	orderRepo   repository.OrderRepository
	expenseRepo repository.ExpenseRepository
	partnerRepo repository.PartnerRepository
	txManager   repository.TransactionManager
}

func NewInvoiceService(
	invoiceRepo repository.InvoiceRepository,
	taxRuleRepo repository.TaxRuleRepository,
	orderRepo repository.OrderRepository,
	expenseRepo repository.ExpenseRepository,
	partnerRepo repository.PartnerRepository,
	txManager repository.TransactionManager,
) InvoiceService {
	return &invoiceService{
		invoiceRepo: invoiceRepo,
		taxRuleRepo: taxRuleRepo,
		orderRepo:   orderRepo,
		expenseRepo: expenseRepo,
		partnerRepo: partnerRepo,
		txManager:   txManager,
	}
}

// --- Implementation ---

func (s *invoiceService) CreateInvoice(ctx context.Context, req CreateInvoiceRequest) (InvoiceResponse, error) {
	subtotal, err := decimal.NewFromString(req.Subtotal)
	if err != nil {
		return InvoiceResponse{}, fmt.Errorf("invalid subtotal: %w", err)
	}

	sideFees := decimal.Zero
	if req.SideFees != "" {
		sideFees, err = decimal.NewFromString(req.SideFees)
		if err != nil {
			return InvoiceResponse{}, fmt.Errorf("invalid side_fees: %w", err)
		}
	}

	refID, err := uuid.Parse(req.ReferenceID)
	if err != nil {
		return InvoiceResponse{}, fmt.Errorf("invalid reference_id: %w", err)
	}

	// Validate reference exists
	switch req.ReferenceType {
	case model.RefTypeOrderImport, model.RefTypeOrderExport:
		if _, err := s.orderRepo.FindByIDWithItems(ctx, refID); err != nil {
			return InvoiceResponse{}, fmt.Errorf("referenced order not found: %w", err)
		}
	case model.RefTypeExpense:
		if _, err := s.expenseRepo.FindByID(ctx, refID); err != nil {
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

		taxRule, err := s.taxRuleRepo.FindByID(ctx, parsed)
		if err != nil {
			return InvoiceResponse{}, fmt.Errorf("tax rule not found: %w", err)
		}
		taxAmount = subtotal.Mul(taxRule.Rate)
	}

	totalAmount := subtotal.Add(taxAmount).Add(sideFees)

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

	// Auto-fill partner hard-copy fields from the Order's partner (if applicable)
	if req.ReferenceType == model.RefTypeOrderImport || req.ReferenceType == model.RefTypeOrderExport {
		order, orderErr := s.orderRepo.FindByIDWithItems(ctx, refID)
		if orderErr == nil && order.PartnerID != nil {
			partner, partnerErr := s.partnerRepo.FindByID(ctx, *order.PartnerID)
			if partnerErr == nil {
				invoice.PartnerID = order.PartnerID
				invoice.CompanyName = partner.CompanyName
				invoice.TaxCode = partner.TaxCode
				// Find first BILLING address
				for _, addr := range partner.Addresses {
					if addr.AddressType == model.AddressTypeBilling {
						invoice.BillingAddress = addr.FullAddress
						break
					}
				}
			}
		}
	}

	if err := s.invoiceRepo.Create(ctx, &invoice); err != nil {
		return InvoiceResponse{}, fmt.Errorf("failed to create invoice: %w", err)
	}

	// Reload with relations
	reloaded, err := s.invoiceRepo.FindByIDWithTaxRule(ctx, invoice.ID)
	if err != nil {
		return InvoiceResponse{}, fmt.Errorf("failed to reload invoice: %w", err)
	}

	return toInvoiceResponse(*reloaded), nil
}

func (s *invoiceService) ListInvoices(ctx context.Context, filter InvoiceFilter) ([]InvoiceResponse, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}

	invoices, total, err := s.invoiceRepo.List(ctx, repository.InvoiceListFilter{
		ApprovalStatus: filter.ApprovalStatus,
		InvoiceNo:      filter.InvoiceNo,
		ReferenceType:  filter.ReferenceType,
		Page:           filter.Page,
		Limit:          filter.Limit,
	})
	if err != nil {
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

	var invoice *model.Invoice
	err = s.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		var findErr error
		invoice, findErr = s.invoiceRepo.FindByID(txCtx, invoiceID)
		if findErr != nil {
			return fmt.Errorf("invoice not found: %w", findErr)
		}

		if invoice.ApprovalStatus != model.ApprovalPending {
			return fmt.Errorf("invoice is already %s", invoice.ApprovalStatus)
		}

		now := time.Now()
		invoice.ApprovalStatus = status
		invoice.ApprovedBy = &approverID
		invoice.ApprovedAt = &now

		if updateErr := s.invoiceRepo.UpdateApproval(txCtx, invoice); updateErr != nil {
			return fmt.Errorf("failed to update invoice: %w", updateErr)
		}

		return nil
	})

	if err != nil {
		return InvoiceResponse{}, err
	}

	// Reload with relations outside transaction
	reloaded, err := s.invoiceRepo.FindByIDWithTaxRule(ctx, invoice.ID)
	if err != nil {
		return InvoiceResponse{}, fmt.Errorf("failed to reload invoice: %w", err)
	}

	return toInvoiceResponse(*reloaded), nil
}

func (s *invoiceService) generateInvoiceNo(ctx context.Context) (string, error) {
	today := time.Now().Format("20060102")
	prefix := "INV-" + today + "-"

	count, err := s.invoiceRepo.CountByPrefix(ctx, prefix)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s%05d", prefix, count+1), nil
}

// --- Helpers ---

// UpdateInvoice allows editing partner hard-copy fields on a PENDING invoice before issuing
func (s *invoiceService) UpdateInvoice(ctx context.Context, id string, req UpdateInvoiceRequest) (InvoiceResponse, error) {
	invoiceID, err := uuid.Parse(id)
	if err != nil {
		return InvoiceResponse{}, fmt.Errorf("invalid invoice id: %w", err)
	}

	invoice, err := s.invoiceRepo.FindByID(ctx, invoiceID)
	if err != nil {
		return InvoiceResponse{}, fmt.Errorf("invoice not found: %w", err)
	}

	if invoice.ApprovalStatus != model.ApprovalPending {
		return InvoiceResponse{}, fmt.Errorf("cannot edit invoice with status %s", invoice.ApprovalStatus)
	}

	if req.CompanyName != nil {
		invoice.CompanyName = *req.CompanyName
	}
	if req.TaxCode != nil {
		invoice.TaxCode = *req.TaxCode
	}
	if req.BillingAddress != nil {
		invoice.BillingAddress = *req.BillingAddress
	}
	if req.Note != nil {
		invoice.Note = *req.Note
	}

	if err := s.invoiceRepo.Update(ctx, invoice); err != nil {
		return InvoiceResponse{}, fmt.Errorf("failed to update invoice: %w", err)
	}

	reloaded, err := s.invoiceRepo.FindByIDWithTaxRule(ctx, invoiceID)
	if err != nil {
		return InvoiceResponse{}, fmt.Errorf("failed to reload invoice: %w", err)
	}

	return toInvoiceResponse(*reloaded), nil
}

// --- Mapping ---

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
		CompanyName:    inv.CompanyName,
		TaxCode:        inv.TaxCode,
		BillingAddress: inv.BillingAddress,
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
	if inv.PartnerID != nil {
		s := inv.PartnerID.String()
		resp.PartnerID = &s
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
