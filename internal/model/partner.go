package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PartnerType enum constants
const (
	PartnerTypeCustomer = "CUSTOMER"
	PartnerTypeSupplier = "SUPPLIER"
	PartnerTypeBoth     = "BOTH"
)

// AddressType enum constants
const (
	AddressTypeBilling  = "BILLING"
	AddressTypeShipping = "SHIPPING"
	AddressTypeOrigin   = "ORIGIN"
)

// Partner represents a customer, supplier, or both
type Partner struct {
	ID            uuid.UUID        `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name          string           `gorm:"type:varchar(255);not null" json:"name"`
	Type          string           `gorm:"type:varchar(20);not null;index" json:"type"` // CUSTOMER, SUPPLIER, BOTH
	TaxCode       string           `gorm:"type:varchar(50)" json:"tax_code"`
	CompanyName   string           `gorm:"type:varchar(255)" json:"company_name"`
	BankAccount   string           `gorm:"type:varchar(100)" json:"bank_account"`
	ContactPerson string           `gorm:"type:varchar(255)" json:"contact_person"`
	Phone         string           `gorm:"type:varchar(50)" json:"phone"`
	Email         string           `gorm:"type:varchar(255)" json:"email"`
	IsActive      bool             `gorm:"default:true" json:"is_active"`
	Addresses     []PartnerAddress `gorm:"foreignKey:PartnerID;constraint:OnDelete:CASCADE" json:"addresses"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
	DeletedAt     gorm.DeletedAt   `gorm:"index" json:"-"`
}

// PartnerAddress represents a partner's address (Billing, Shipping, Origin)
type PartnerAddress struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PartnerID   uuid.UUID `gorm:"type:uuid;not null;index" json:"partner_id"`
	AddressType string    `gorm:"type:varchar(20);not null" json:"address_type"` // BILLING, SHIPPING, ORIGIN
	FullAddress string    `gorm:"type:text;not null" json:"full_address"`
	IsDefault   bool      `gorm:"default:false" json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
