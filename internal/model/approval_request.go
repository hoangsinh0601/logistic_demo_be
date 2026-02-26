package model

import (
	"time"

	"github.com/google/uuid"
)

// ApprovalRequestType enum constants
const (
	ApprovalReqTypeCreateOrder   = "CREATE_ORDER"
	ApprovalReqTypeCreateProduct = "CREATE_PRODUCT"
	ApprovalReqTypeCreateExpense = "CREATE_EXPENSE"
)

// ApprovalRequest represents a pending approval for any economic activity.
// Only after approval does the system create invoices and update statistics.
type ApprovalRequest struct {
	ID              uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	RequestType     string     `gorm:"type:varchar(30);not null;index" json:"request_type"` // CREATE_ORDER, CREATE_PRODUCT, CREATE_EXPENSE
	ReferenceID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"reference_id"`        // FK to orders.id / products.id / expenses.id
	RequestData     string     `gorm:"type:jsonb;not null" json:"request_data"`              // Full snapshot of the request payload
	Status          string     `gorm:"type:varchar(20);not null;default:'PENDING';index" json:"status"`
	RequestedBy     *uuid.UUID `gorm:"type:uuid;index" json:"requested_by"`
	Requester       *User      `gorm:"foreignKey:RequestedBy" json:"requester,omitempty"`
	ApprovedBy      *uuid.UUID `gorm:"type:uuid" json:"approved_by"`
	Approver        *User      `gorm:"foreignKey:ApprovedBy" json:"approver,omitempty"`
	ApprovedAt      *time.Time `json:"approved_at"`
	RejectionReason string     `gorm:"type:text" json:"rejection_reason"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
