package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// DocumentType enum constants
const (
	DocTypeVATInvoice    = "VAT_INVOICE"
	DocTypeDirectInvoice = "DIRECT_INVOICE"
	DocTypeRetailReceipt = "RETAIL_RECEIPT"
	DocTypeNone          = "NONE"
)

// FCTType enum constants
const (
	FCTTypeNet   = "NET"
	FCTTypeGross = "GROSS"
)

// Expense represents a payment/cost entry with multi-currency support (base: USD)
type Expense struct {
	ID       uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderID  *uuid.UUID `gorm:"type:uuid;index" json:"order_id"`
	VendorID *uuid.UUID `gorm:"type:uuid;index" json:"vendor_id"`

	// Currency & Exchange Rate
	Currency           string          `gorm:"type:varchar(10);not null;default:'USD'" json:"currency"`
	ExchangeRate       decimal.Decimal `gorm:"type:decimal(18,6);not null;default:1" json:"exchange_rate"`                          // 1 if USD
	OriginalAmount     decimal.Decimal `gorm:"type:decimal(18,4);not null" json:"original_amount"`                                  // Amount in original currency
	ConvertedAmountUSD decimal.Decimal `gorm:"column:converted_amount_usd;type:decimal(18,4);not null" json:"converted_amount_usd"` // = original_amount * exchange_rate

	// FCT (Foreign Contractor Tax)
	IsForeignVendor bool            `gorm:"default:false" json:"is_foreign_vendor"`
	FCTType         string          `gorm:"type:varchar(10)" json:"fct_type"`                                 // NET or GROSS
	FCTRate         decimal.Decimal `gorm:"type:decimal(10,4);default:0" json:"fct_rate"`                     // Rate fetched from tax_rules
	FCTAmount       decimal.Decimal `gorm:"column:fct_amount;type:decimal(18,4);default:0" json:"fct_amount"` // Tax amount in USD
	TotalPayable    decimal.Decimal `gorm:"type:decimal(18,4);default:0" json:"total_payable"`                // Final amount in original currency

	// VAT
	VATRate   decimal.Decimal `gorm:"type:decimal(10,4);default:0" json:"vat_rate"`                     // Rate fetched from tax_rules
	VATAmount decimal.Decimal `gorm:"column:vat_amount;type:decimal(18,4);default:0" json:"vat_amount"` // VAT amount in USD

	// Document & Deductibility (Rào chắn chi phí hợp lệ)
	DocumentType        string  `gorm:"type:varchar(30);not null;default:'NONE'" json:"document_type"` // VAT_INVOICE, DIRECT_INVOICE, RETAIL_RECEIPT, NONE
	VendorTaxCode       *string `gorm:"type:varchar(50)" json:"vendor_tax_code"`
	DocumentURL         string  `gorm:"type:text" json:"document_url"`
	IsDeductibleExpense bool    `gorm:"default:false" json:"is_deductible_expense"`

	Description string    `gorm:"type:text" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
