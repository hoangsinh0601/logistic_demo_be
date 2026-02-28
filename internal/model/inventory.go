package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Product represents an item in the inventory
type Product struct {
	ID           uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SKU          string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"sku"`
	Name         string         `gorm:"type:varchar(255);not null" json:"name"`
	CurrentStock int            `gorm:"type:int;default:0;not null" json:"current_stock"`
	Price        float64        `gorm:"type:decimal(10,2);not null" json:"price"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// OrderType Enum Simulation
const (
	OrderTypeImport = "IMPORT"
	OrderTypeExport = "EXPORT"
)

// OrderStatus constants
const (
	OrderStatusPendingApproval = "PENDING_APPROVAL"
	OrderStatusCompleted       = "COMPLETED"
	OrderStatusRejected        = "REJECTED"
)

// Order represents an inventory transaction request (Import/Export)
type Order struct {
	ID                uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderCode         string          `gorm:"type:varchar(100);uniqueIndex;not null" json:"order_code"`
	Type              string          `gorm:"type:varchar(20);not null" json:"type"` // IMPORT, EXPORT
	Status            string          `gorm:"type:varchar(50);default:'COMPLETED'" json:"status"`
	Note              string          `gorm:"type:text" json:"note"`
	PartnerID         *uuid.UUID      `gorm:"type:uuid;index" json:"partner_id"`
	Partner           *Partner        `gorm:"foreignKey:PartnerID" json:"partner,omitempty"`
	OriginAddressID   *uuid.UUID      `gorm:"type:uuid" json:"origin_address_id"`
	OriginAddress     *PartnerAddress `gorm:"foreignKey:OriginAddressID" json:"origin_address,omitempty"`
	ShippingAddressID *uuid.UUID      `gorm:"type:uuid" json:"shipping_address_id"`
	ShippingAddress   *PartnerAddress `gorm:"foreignKey:ShippingAddressID" json:"shipping_address,omitempty"`
	Items             []OrderItem     `gorm:"foreignKey:OrderID" json:"items"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// OrderItem represents a line item within an Order
type OrderItem struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderID   uuid.UUID `gorm:"type:uuid;not null;index" json:"order_id"`
	ProductID uuid.UUID `gorm:"type:uuid;not null;index" json:"product_id"`
	Product   Product   `gorm:"foreignKey:ProductID" json:"-"`
	Quantity  int       `gorm:"type:int;not null" json:"quantity"`
	UnitPrice float64   `gorm:"type:decimal(10,2);not null" json:"unit_price"`
}

// TransactionType Enum Simulation
const (
	TxTypeIn  = "IN"
	TxTypeOut = "OUT"
)

// InventoryTransaction (Thẻ kho) records stock changes strictly
type InventoryTransaction struct {
	ID              uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ProductID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"product_id"`
	OrderID         *uuid.UUID `gorm:"type:uuid;index" json:"order_id"`                   // Nullable in case of manual adjustments
	TransactionType string     `gorm:"type:varchar(10);not null" json:"transaction_type"` // IN, OUT
	QuantityChanged int        `gorm:"type:int;not null" json:"quantity_changed"`
	StockAfter      int        `gorm:"type:int;not null" json:"stock_after"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
