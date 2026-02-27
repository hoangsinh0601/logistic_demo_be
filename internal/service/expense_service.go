package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"backend/internal/model"
	"backend/internal/repository"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// --- DTOs ---

type CreateExpenseRequest struct {
	OrderID  string `json:"order_id"`
	VendorID string `json:"vendor_id"`

	Currency       string `json:"currency" binding:"required"`
	ExchangeRate   string `json:"exchange_rate" binding:"required"` // Decimal string
	OriginalAmount string `json:"original_amount" binding:"required"`

	IsForeignVendor bool   `json:"is_foreign_vendor"`
	FCTType         string `json:"fct_type"` // NET or GROSS

	DocumentType  string  `json:"document_type" binding:"required,oneof=VAT_INVOICE DIRECT_INVOICE RETAIL_RECEIPT NONE"`
	VendorTaxCode *string `json:"vendor_tax_code"`
	DocumentURL   string  `json:"document_url"`
	Description   string  `json:"description"`
}

type ExpenseResponse struct {
	ID                  string  `json:"id"`
	OrderID             *string `json:"order_id"`
	VendorID            *string `json:"vendor_id"`
	Currency            string  `json:"currency"`
	ExchangeRate        string  `json:"exchange_rate"`
	OriginalAmount      string  `json:"original_amount"`
	ConvertedAmountUSD  string  `json:"converted_amount_usd"`
	IsForeignVendor     bool    `json:"is_foreign_vendor"`
	FCTType             string  `json:"fct_type"`
	FCTRate             string  `json:"fct_rate"`
	FCTAmount           string  `json:"fct_amount"`
	TotalPayable        string  `json:"total_payable"`
	VATRate             string  `json:"vat_rate"`
	VATAmount           string  `json:"vat_amount"`
	DocumentType        string  `json:"document_type"`
	VendorTaxCode       *string `json:"vendor_tax_code"`
	DocumentURL         string  `json:"document_url"`
	IsDeductibleExpense bool    `json:"is_deductible_expense"`
	Description         string  `json:"description"`
	CreatedAt           string  `json:"created_at"`
}

// --- Interface ---

type ExpenseService interface {
	CreateExpense(ctx context.Context, userID string, req CreateExpenseRequest) (ExpenseResponse, error)
	GetExpenses(ctx context.Context, page, limit int) ([]ExpenseResponse, int64, error)
}

type expenseService struct {
	expenseRepo  repository.ExpenseRepository
	auditRepo    repository.AuditRepository
	approvalRepo repository.ApprovalRepository
	txManager    repository.TransactionManager
	taxService   TaxService
}

func NewExpenseService(
	expenseRepo repository.ExpenseRepository,
	auditRepo repository.AuditRepository,
	approvalRepo repository.ApprovalRepository,
	txManager repository.TransactionManager,
	taxService TaxService,
) ExpenseService {
	return &expenseService{
		expenseRepo:  expenseRepo,
		auditRepo:    auditRepo,
		approvalRepo: approvalRepo,
		txManager:    txManager,
		taxService:   taxService,
	}
}

// --- Implementation ---

func (s *expenseService) CreateExpense(ctx context.Context, userID string, req CreateExpenseRequest) (ExpenseResponse, error) {
	// Parse decimal fields
	originalAmount, err := decimal.NewFromString(req.OriginalAmount)
	if err != nil {
		return ExpenseResponse{}, fmt.Errorf("invalid original_amount: %w", err)
	}

	exchangeRate, err := decimal.NewFromString(req.ExchangeRate)
	if err != nil {
		return ExpenseResponse{}, fmt.Errorf("invalid exchange_rate: %w", err)
	}

	if exchangeRate.LessThanOrEqual(decimal.Zero) {
		return ExpenseResponse{}, fmt.Errorf("exchange_rate must be greater than 0")
	}

	// ---- Currency Conversion ----
	convertedAmountUSD := originalAmount.Mul(exchangeRate)

	// ---- FCT Logic ----
	fctRate := decimal.Zero
	fctAmount := decimal.Zero
	totalPayable := originalAmount

	if req.IsForeignVendor {
		if req.FCTType != model.FCTTypeNet && req.FCTType != model.FCTTypeGross {
			return ExpenseResponse{}, fmt.Errorf("fct_type must be NET or GROSS when is_foreign_vendor is true")
		}

		// Fetch active FCT rate from tax_rules
		activeRate, fctErr := s.taxService.CalculateActiveTax(ctx, model.TaxTypeFCT, time.Now())
		if fctErr != nil {
			return ExpenseResponse{}, fmt.Errorf("failed to get active FCT rate: %w", fctErr)
		}
		fctRate = activeRate

		switch req.FCTType {
		case model.FCTTypeNet:
			fctAmount = convertedAmountUSD.Mul(fctRate)
		case model.FCTTypeGross:
			fctAmount = convertedAmountUSD.Mul(fctRate).Div(decimal.NewFromInt(1).Add(fctRate))
		}

		fctInOriginal := fctAmount.Div(exchangeRate)
		totalPayable = originalAmount.Add(fctInOriginal)
	}

	// ---- VAT Logic ----
	vatRate := decimal.Zero
	vatAmount := decimal.Zero

	if req.DocumentType == model.DocTypeVATInvoice {
		vatType := model.TaxTypeVATInland
		if req.IsForeignVendor {
			vatType = model.TaxTypeVATIntl
		}
		activeVAT, vatErr := s.taxService.CalculateActiveTax(ctx, vatType, time.Now())
		if vatErr == nil {
			vatRate = activeVAT
			vatAmount = convertedAmountUSD.Mul(vatRate)
		}
	}

	// ---- Deductibility Logic ----
	isDeductible := false
	if req.DocumentType == model.DocTypeVATInvoice {
		if req.VendorTaxCode == nil || *req.VendorTaxCode == "" {
			return ExpenseResponse{}, fmt.Errorf("vendor_tax_code is required when document_type is VAT_INVOICE")
		}
		isDeductible = true
	}

	// ---- Build Model ----
	expense := model.Expense{
		Currency:            req.Currency,
		ExchangeRate:        exchangeRate,
		OriginalAmount:      originalAmount,
		ConvertedAmountUSD:  convertedAmountUSD,
		IsForeignVendor:     req.IsForeignVendor,
		FCTType:             req.FCTType,
		FCTRate:             fctRate,
		FCTAmount:           fctAmount,
		TotalPayable:        totalPayable,
		VATRate:             vatRate,
		VATAmount:           vatAmount,
		DocumentType:        req.DocumentType,
		VendorTaxCode:       req.VendorTaxCode,
		DocumentURL:         req.DocumentURL,
		IsDeductibleExpense: isDeductible,
		Description:         req.Description,
	}

	// Parse optional UUIDs
	if req.OrderID != "" {
		parsed, parseErr := uuid.Parse(req.OrderID)
		if parseErr != nil {
			return ExpenseResponse{}, fmt.Errorf("invalid order_id: %w", parseErr)
		}
		expense.OrderID = &parsed
	}
	if req.VendorID != "" {
		parsed, parseErr := uuid.Parse(req.VendorID)
		if parseErr != nil {
			return ExpenseResponse{}, fmt.Errorf("invalid vendor_id: %w", parseErr)
		}
		expense.VendorID = &parsed
	}

	// Parse user UUID for audit/approval
	var userUUID *uuid.UUID
	if userID != "" {
		parsed, parseErr := uuid.Parse(userID)
		if parseErr == nil {
			userUUID = &parsed
		}
	}

	// ---- DB Transaction via TransactionManager ----
	err = s.txManager.RunInTx(ctx, func(txCtx context.Context) error {
		if createErr := s.expenseRepo.Create(txCtx, &expense); createErr != nil {
			return fmt.Errorf("failed to create expense: %w", createErr)
		}

		// Audit log for expense creation
		expenseAuditDetails, _ := json.Marshal(map[string]interface{}{
			"currency":          req.Currency,
			"exchange_rate":     req.ExchangeRate,
			"original_amount":   req.OriginalAmount,
			"is_foreign_vendor": req.IsForeignVendor,
			"document_type":     req.DocumentType,
			"description":       req.Description,
		})
		expenseAudit := &model.AuditLog{
			UserID:     userUUID,
			Action:     model.ActionCreateExpense,
			EntityID:   expense.ID.String(),
			EntityName: req.Description,
			Details:    string(expenseAuditDetails),
		}
		if auditErr := s.auditRepo.Log(txCtx, expenseAudit); auditErr != nil {
			return fmt.Errorf("failed to write expense audit log: %w", auditErr)
		}

		// Create ApprovalRequest for this expense
		requestData, _ := json.Marshal(map[string]interface{}{
			"currency":          req.Currency,
			"exchange_rate":     req.ExchangeRate,
			"original_amount":   req.OriginalAmount,
			"is_foreign_vendor": req.IsForeignVendor,
			"fct_type":          req.FCTType,
			"document_type":     req.DocumentType,
			"description":       req.Description,
		})

		approvalReq := &model.ApprovalRequest{
			RequestType: model.ApprovalReqTypeCreateExpense,
			ReferenceID: expense.ID,
			RequestData: string(requestData),
			RequestedBy: userUUID,
			Status:      model.ApprovalPending,
		}
		if createErr := s.approvalRepo.Create(txCtx, approvalReq); createErr != nil {
			return fmt.Errorf("failed to create approval request: %w", createErr)
		}

		// Audit log for approval request
		auditDetails, _ := json.Marshal(map[string]interface{}{
			"request_type": model.ApprovalReqTypeCreateExpense,
			"reference_id": expense.ID.String(),
			"description":  req.Description,
		})
		audit := &model.AuditLog{
			UserID:     userUUID,
			Action:     model.ActionCreateApprovalRequest,
			EntityID:   approvalReq.ID.String(),
			EntityName: model.ApprovalReqTypeCreateExpense,
			Details:    string(auditDetails),
		}
		if auditErr := s.auditRepo.Log(txCtx, audit); auditErr != nil {
			return fmt.Errorf("failed to write audit log: %w", auditErr)
		}

		return nil
	})

	if err != nil {
		return ExpenseResponse{}, err
	}

	return toExpenseResponse(expense), nil
}

func (s *expenseService) GetExpenses(ctx context.Context, page, limit int) ([]ExpenseResponse, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	expenses, total, err := s.expenseRepo.List(ctx, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch expenses: %w", err)
	}

	result := make([]ExpenseResponse, 0, len(expenses))
	for _, e := range expenses {
		result = append(result, toExpenseResponse(e))
	}
	return result, total, nil
}

// --- Helpers ---

func toExpenseResponse(e model.Expense) ExpenseResponse {
	resp := ExpenseResponse{
		ID:                  e.ID.String(),
		Currency:            e.Currency,
		ExchangeRate:        e.ExchangeRate.StringFixed(6),
		OriginalAmount:      e.OriginalAmount.StringFixed(4),
		ConvertedAmountUSD:  e.ConvertedAmountUSD.StringFixed(4),
		IsForeignVendor:     e.IsForeignVendor,
		FCTType:             e.FCTType,
		FCTRate:             e.FCTRate.StringFixed(4),
		FCTAmount:           e.FCTAmount.StringFixed(4),
		TotalPayable:        e.TotalPayable.StringFixed(4),
		VATRate:             e.VATRate.StringFixed(4),
		VATAmount:           e.VATAmount.StringFixed(4),
		DocumentType:        e.DocumentType,
		VendorTaxCode:       e.VendorTaxCode,
		DocumentURL:         e.DocumentURL,
		IsDeductibleExpense: e.IsDeductibleExpense,
		Description:         e.Description,
		CreatedAt:           e.CreatedAt.Format(time.RFC3339),
	}

	if e.OrderID != nil {
		s := e.OrderID.String()
		resp.OrderID = &s
	}
	if e.VendorID != nil {
		s := e.VendorID.String()
		resp.VendorID = &s
	}

	return resp
}
