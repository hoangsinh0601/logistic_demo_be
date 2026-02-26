package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ReferenceType enum constants
const (
	RefTypeOrderImport = "ORDER_IMPORT"
	RefTypeOrderExport = "ORDER_EXPORT"
	RefTypeExpense     = "EXPENSE"
)

// ApprovalStatus enum constants
const (
	ApprovalPending  = "PENDING"
	ApprovalApproved = "APPROVED"
	ApprovalRejected = "REJECTED"
)

// Invoice represents a financial document generated from orders or expenses.
// Only APPROVED invoices count toward revenue statistics.
type Invoice struct {
	ID             uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	InvoiceNo      string          `gorm:"type:varchar(30);uniqueIndex;not null" json:"invoice_no"`
	ReferenceType  string          `gorm:"type:varchar(20);not null;index" json:"reference_type"` // ORDER_IMPORT, ORDER_EXPORT, EXPENSE
	ReferenceID    uuid.UUID       `gorm:"type:uuid;not null;index" json:"reference_id"`          // FK to orders.id or expenses.id
	TaxRuleID      *uuid.UUID      `gorm:"type:uuid;index" json:"tax_rule_id"`                    // FK to tax_rules.id (nullable)
	TaxRule        *TaxRule        `gorm:"foreignKey:TaxRuleID" json:"tax_rule,omitempty"`
	Subtotal       decimal.Decimal `gorm:"type:decimal(18,4);not null" json:"subtotal"`             // Pre-tax amount
	TaxAmount      decimal.Decimal `gorm:"type:decimal(18,4);not null;default:0" json:"tax_amount"` // Computed from tax rule
	SideFees       decimal.Decimal `gorm:"type:decimal(18,4);not null;default:0" json:"side_fees"`  // Additional fees
	TotalAmount    decimal.Decimal `gorm:"type:decimal(18,4);not null" json:"total_amount"`         // subtotal + tax_amount + side_fees
	ApprovalStatus string          `gorm:"type:varchar(20);not null;default:'PENDING';index" json:"approval_status"`
	ApprovedBy     *uuid.UUID      `gorm:"type:uuid" json:"approved_by"`
	Approver       *User           `gorm:"foreignKey:ApprovedBy" json:"approver,omitempty"`
	ApprovedAt     *time.Time      `json:"approved_at"`
	Note           string          `gorm:"type:text" json:"note"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}
