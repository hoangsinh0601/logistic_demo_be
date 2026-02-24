package service

import (
	"context"
	"fmt"
	"time"

	"backend/internal/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
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
	ConvertedAmountVND  string  `json:"converted_amount_vnd"`
	IsForeignVendor     bool    `json:"is_foreign_vendor"`
	FCTType             string  `json:"fct_type"`
	FCTRate             string  `json:"fct_rate"`
	FCTAmountVND        string  `json:"fct_amount_vnd"`
	TotalPayable        string  `json:"total_payable"`
	DocumentType        string  `json:"document_type"`
	VendorTaxCode       *string `json:"vendor_tax_code"`
	DocumentURL         string  `json:"document_url"`
	IsDeductibleExpense bool    `json:"is_deductible_expense"`
	Description         string  `json:"description"`
	CreatedAt           string  `json:"created_at"`
}

// --- Interface ---

type ExpenseService interface {
	CreateExpense(ctx context.Context, req CreateExpenseRequest) (ExpenseResponse, error)
	GetExpenses(ctx context.Context) ([]ExpenseResponse, error)
}

type expenseService struct {
	db         *gorm.DB
	taxService TaxService
}

func NewExpenseService(db *gorm.DB, taxService TaxService) ExpenseService {
	return &expenseService{db: db, taxService: taxService}
}

// --- Implementation ---

func (s *expenseService) CreateExpense(ctx context.Context, req CreateExpenseRequest) (ExpenseResponse, error) {
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
	convertedAmountVND := originalAmount.Mul(exchangeRate)

	// ---- FCT Logic ----
	fctRate := decimal.Zero
	fctAmountVND := decimal.Zero
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
			// NET: fct_amount = converted_vnd * fct_rate
			fctAmountVND = convertedAmountVND.Mul(fctRate)
		case model.FCTTypeGross:
			// GROSS: fct_amount = converted_vnd * fct_rate / (1 + fct_rate)
			fctAmountVND = convertedAmountVND.Mul(fctRate).Div(decimal.NewFromInt(1).Add(fctRate))
		}

		// Total payable = original_amount + FCT in original currency
		fctInOriginal := fctAmountVND.Div(exchangeRate)
		totalPayable = originalAmount.Add(fctInOriginal)
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
		ConvertedAmountVND:  convertedAmountVND,
		IsForeignVendor:     req.IsForeignVendor,
		FCTType:             req.FCTType,
		FCTRate:             fctRate,
		FCTAmountVND:        fctAmountVND,
		TotalPayable:        totalPayable,
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

	// ---- DB Transaction ----
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if createErr := tx.Create(&expense).Error; createErr != nil {
			return fmt.Errorf("failed to create expense: %w", createErr)
		}
		return nil
	})

	if err != nil {
		return ExpenseResponse{}, err
	}

	return toExpenseResponse(expense), nil
}

func (s *expenseService) GetExpenses(ctx context.Context) ([]ExpenseResponse, error) {
	var expenses []model.Expense
	if err := s.db.WithContext(ctx).Order("created_at DESC").Find(&expenses).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch expenses: %w", err)
	}

	result := make([]ExpenseResponse, 0, len(expenses))
	for _, e := range expenses {
		result = append(result, toExpenseResponse(e))
	}
	return result, nil
}

// --- Helpers ---

func toExpenseResponse(e model.Expense) ExpenseResponse {
	resp := ExpenseResponse{
		ID:                  e.ID.String(),
		Currency:            e.Currency,
		ExchangeRate:        e.ExchangeRate.StringFixed(6),
		OriginalAmount:      e.OriginalAmount.StringFixed(4),
		ConvertedAmountVND:  e.ConvertedAmountVND.StringFixed(4),
		IsForeignVendor:     e.IsForeignVendor,
		FCTType:             e.FCTType,
		FCTRate:             e.FCTRate.StringFixed(4),
		FCTAmountVND:        e.FCTAmountVND.StringFixed(4),
		TotalPayable:        e.TotalPayable.StringFixed(4),
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
