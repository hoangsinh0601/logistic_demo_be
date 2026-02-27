package repository

import (
	"context"

	"gorm.io/gorm"
)

type contextKey string

const txKey contextKey = "gorm_tx"

// TransactionManager manages database transactions via context injection.
type TransactionManager interface {
	RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error
}

type transactionManager struct {
	db *gorm.DB
}

func NewTransactionManager(db *gorm.DB) TransactionManager {
	return &transactionManager{db: db}
}

func (t *transactionManager) RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	return t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, txKey, tx)
		return fn(txCtx)
	})
}

// GetDB extracts the transaction DB from context if present, otherwise returns root DB.
func GetDB(ctx context.Context, rootDB *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey).(*gorm.DB); ok {
		return tx.WithContext(ctx)
	}
	return rootDB.WithContext(ctx)
}
