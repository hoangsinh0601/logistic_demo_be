package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents the central user entity for logic and database structure
type User struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Username  string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"username"`
	Email     string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	Phone     string         `gorm:"type:varchar(20);not null" json:"phone"`
	Password  string         `gorm:"type:varchar(255);not null" json:"-"`   // Omit password from JSON requests/responses
	Role      string         `gorm:"type:varchar(50);not null" json:"role"` // admin, manager, staff
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"` // GORM soft delete
}

// RefreshToken stores long-lived tokens allowing users to request new access tokens
type RefreshToken struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	User      User      `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Token     string    `gorm:"type:varchar(255);uniqueIndex;not null" json:"token"`
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}
