package database

import (
	"log"

	"backend/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// NewConnection initializes a new connection pool using GORM
func NewConnection(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Auto-migrate core models
	err = db.AutoMigrate(
		&model.User{},
		&model.Product{},
		&model.Order{},
		&model.OrderItem{},
		&model.InventoryTransaction{},
		&model.RefreshToken{},
		&model.AuditLog{},
		&model.TaxRule{},
		&model.Expense{},
		&model.Role{},
		&model.Permission{},
		&model.Invoice{},
		&model.ApprovalRequest{},
		&model.Partner{},
		&model.PartnerAddress{},
	)
	if err != nil {
		log.Println("WARNING: Failed to auto-migrate models:", err)
	}

	return db, nil
}
