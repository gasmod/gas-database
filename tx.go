package database

import (
	"context"
	"database/sql"
	"fmt"
)

// DBTX is the interface that sqlc-generated code expects. Both *sql.DB
// and *sql.Tx satisfy it, making it easy to swap between pooled and
// transactional access.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// BeginTx starts a new database transaction. The caller is responsible
// for calling Commit or Rollback on the returned *sql.Tx.
func (m *Module) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if m.closed.Load() {
		return nil, fmt.Errorf("%s: module is closed", m.Name())
	}
	tx, err := m.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("%s: begin tx: %w", m.Name(), err)
	}
	return tx, nil
}

// WithTx executes fn within a transaction. If fn returns nil the
// transaction is committed; otherwise it is rolled back. Any panic
// inside fn also triggers a rollback.
func (m *Module) WithTx(ctx context.Context, opts *sql.TxOptions, fn func(*sql.Tx) error) (err error) {
	if m.closed.Load() {
		return fmt.Errorf("%s: module is closed", m.Name())
	}

	tx, err := m.db.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("%s: begin tx: %w", m.Name(), err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()

	err = fn(tx)
	return err
}
