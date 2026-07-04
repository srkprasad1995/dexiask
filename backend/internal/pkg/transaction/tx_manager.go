package transaction

import (
	"context"

	"gorm.io/gorm"
)

type contextKey string

const txKey contextKey = "tx"

// TxManager wraps a *gorm.DB and propagates transactions through context.
type TxManager struct {
	db *gorm.DB
}

// NewTxManager creates a new TxManager.
func NewTxManager(db *gorm.DB) *TxManager { return &TxManager{db: db} }

// GetDB returns the transaction-scoped *gorm.DB when called inside
// InTransaction, otherwise the base DB with context applied.
func (tm *TxManager) GetDB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(txKey).(*gorm.DB); ok {
		return tx
	}
	return tm.db.WithContext(ctx)
}

// InTransaction runs fn inside a database transaction. Nested calls reuse the
// outer transaction.
func (tm *TxManager) InTransaction(ctx context.Context, fn func(context.Context) error) error {
	if _, ok := ctx.Value(txKey).(*gorm.DB); ok {
		return fn(ctx)
	}
	return tm.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txKey, tx))
	})
}
