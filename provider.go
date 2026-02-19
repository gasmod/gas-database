package database

import (
	"context"
	"fmt"

	"github.com/gasmod/gas"
)

// Compile-time check: Module implements gas.DatabaseProvider.
var _ gas.DatabaseProvider = (*Module)(nil)

// Query executes a query that returns rows. The returned gas.Rows is
// backed by *sql.Rows which natively satisfies the interface.
func (m *Module) Query(ctx context.Context, query string, args ...any) (gas.Rows, error) {
	if m.closed.Load() {
		return nil, fmt.Errorf("%s: module is closed", m.Name())
	}
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: query: %w", m.Name(), err)
	}
	return rows, nil
}

// Exec executes a query that doesn't return rows. The returned
// gas.Result is backed by sql.Result which natively satisfies the interface.
func (m *Module) Exec(ctx context.Context, query string, args ...any) (gas.Result, error) {
	if m.closed.Load() {
		return nil, fmt.Errorf("%s: module is closed", m.Name())
	}
	result, err := m.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: exec: %w", m.Name(), err)
	}
	return result, nil
}
