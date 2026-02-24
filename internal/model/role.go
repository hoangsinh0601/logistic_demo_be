package model

import (
	"time"

	"github.com/google/uuid"
)

// Role represents a user role with associated permissions
type Role struct {
	ID          uuid.UUID    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name        string       `gorm:"type:varchar(50);uniqueIndex;not null" json:"name"`
	Description string       `gorm:"type:text" json:"description"`
	IsSystem    bool         `gorm:"default:false" json:"is_system"` // Prevent deletion of built-in roles
	Permissions []Permission `gorm:"many2many:role_permissions;" json:"permissions"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Permission represents a single permission that can be assigned to roles
type Permission struct {
	ID    uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Code  string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"code"` // e.g. "tax_rules.read"
	Name  string    `gorm:"type:varchar(255);not null" json:"name"`
	Group string    `gorm:"type:varchar(50);not null;index" json:"group"` // "tax", "users", "inventory"...
}
