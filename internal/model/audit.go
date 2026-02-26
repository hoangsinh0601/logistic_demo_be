package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	ActionCreateProduct  = "CREATE_PRODUCT"
	ActionUpdateProduct  = "UPDATE_PRODUCT"
	ActionDeleteProduct  = "DELETE_PRODUCT"
	ActionCreateOrderIn  = "CREATE_ORDER_IMPORT"
	ActionCreateOrderOut = "CREATE_ORDER_EXPORT"
	ActionCreateTaxRule  = "CREATE_TAX_RULE"
	ActionUpdateTaxRule  = "UPDATE_TAX_RULE"
	ActionDeleteTaxRule  = "DELETE_TAX_RULE"

	// Approval workflow actions
	ActionCreateApprovalRequest     = "CREATE_APPROVAL_REQUEST"
	ActionApproveRequest            = "APPROVE_REQUEST"
	ActionRejectRequest             = "REJECT_REQUEST"
	ActionCreateInvoiceFromApproval = "CREATE_INVOICE_FROM_APPROVAL"
	ActionCreateExpense             = "CREATE_EXPENSE"
)

// AuditLog tracks Who, What, and When for critical system changes
type AuditLog struct {
	ID         uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID     *uuid.UUID `gorm:"type:uuid;index" json:"user_id"` // Nullable gracefully if automated bot
	User       *User      `gorm:"foreignKey:UserID" json:"user"`
	Action     string     `gorm:"type:varchar(50);not null;index" json:"action"`
	EntityID   string     `gorm:"type:varchar(50);index" json:"entity_id"`        // Reference string (uuid/code)
	EntityName string     `gorm:"type:varchar(255)" json:"entity_name,omitempty"` // Human readable name
	Details    string     `gorm:"type:jsonb" json:"details"`                      // Serialized JSON payload of the action
	CreatedAt  time.Time  `gorm:"index" json:"created_at"`
}
