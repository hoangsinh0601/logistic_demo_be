package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// TaxType enum constants
const (
	TaxTypeVATInland = "VAT_INLAND"
	TaxTypeVATIntl   = "VAT_INTL"
	TaxTypeFCT       = "FCT"
)

// TaxRule stores tax rates with temporal validity
type TaxRule struct {
	ID            uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	TaxType       string          `gorm:"type:varchar(20);not null;index" json:"tax_type"` // VAT_INLAND, VAT_INTL, FCT
	Rate          decimal.Decimal `gorm:"type:decimal(10,4);not null" json:"rate"`         // e.g. 0.10 = 10%
	EffectiveFrom time.Time       `gorm:"type:date;not null;index" json:"effective_from"`  // Start date
	EffectiveTo   *time.Time      `gorm:"type:date;index" json:"effective_to"`             // End date, nullable = currently active
	Description   string          `gorm:"type:text" json:"description"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}
