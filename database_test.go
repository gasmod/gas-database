package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/gasmod/gas"
	_ "modernc.org/sqlite"
)

// Compile-time interface checks.
var (
	_ gas.Module           = (*Module)(nil)
	_ gas.DatabaseProvider = (*Module)(nil)
)

func newTestModule(t *testing.T) *Module {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	m := New(WithConfig(Config{
		Driver: "sqlite",
		DSN:    dsn,
	}))
	if err := m.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

func TestName(t *testing.T) {
	m := &Module{}
	if m.Name() != "gas-database" {
		t.Fatalf("expected gas-database, got %s", m.Name())
	}
}

func TestNew_Defaults(t *testing.T) {
	m := New()
	defaults := DefaultConfig()
	if m.cfg.Driver != defaults.Driver {
		t.Errorf("Driver = %q, want %q", m.cfg.Driver, defaults.Driver)
	}
	if m.cfg.MaxOpenConns != defaults.MaxOpenConns {
		t.Errorf("MaxOpenConns = %d, want %d", m.cfg.MaxOpenConns, defaults.MaxOpenConns)
	}
	if m.cfg.MaxIdleConns != defaults.MaxIdleConns {
		t.Errorf("MaxIdleConns = %d, want %d", m.cfg.MaxIdleConns, defaults.MaxIdleConns)
	}
	if m.cfg.ConnMaxLifetime != defaults.ConnMaxLifetime {
		t.Errorf("ConnMaxLifetime = %v, want %v", m.cfg.ConnMaxLifetime, defaults.ConnMaxLifetime)
	}
	if m.cfg.ConnMaxIdleTime != defaults.ConnMaxIdleTime {
		t.Errorf("ConnMaxIdleTime = %v, want %v", m.cfg.ConnMaxIdleTime, defaults.ConnMaxIdleTime)
	}
}

func TestNew_WithConfig(t *testing.T) {
	cfg := Config{
		Driver:          "sqlite",
		DSN:             ":memory:",
		MaxOpenConns:    10,
		MaxIdleConns:    3,
		ConnMaxLifetime: 15 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}
	m := New(WithConfig(cfg))
	if m.cfg.Driver != "sqlite" {
		t.Errorf("Driver = %q, want sqlite", m.cfg.Driver)
	}
	if m.cfg.DSN != ":memory:" {
		t.Errorf("DSN = %q, want :memory:", m.cfg.DSN)
	}
	if m.cfg.MaxOpenConns != 10 {
		t.Errorf("MaxOpenConns = %d, want 10", m.cfg.MaxOpenConns)
	}
	if m.cfg.MaxIdleConns != 3 {
		t.Errorf("MaxIdleConns = %d, want 3", m.cfg.MaxIdleConns)
	}
	if m.cfg.ConnMaxLifetime != 15*time.Minute {
		t.Errorf("ConnMaxLifetime = %v, want 15m", m.cfg.ConnMaxLifetime)
	}
	if m.cfg.ConnMaxIdleTime != 2*time.Minute {
		t.Errorf("ConnMaxIdleTime = %v, want 2m", m.cfg.ConnMaxIdleTime)
	}
}

func TestInit_NoDSN(t *testing.T) {
	m := New()
	if err := m.Init(); err == nil {
		t.Fatal("expected error for missing DSN")
	}
}

func TestInit_Close_Lifecycle(t *testing.T) {
	m := newTestModule(t)
	if m.DB() == nil {
		t.Fatal("DB() should not be nil after Init")
	}
}

func TestDB_ReturnsConnection(t *testing.T) {
	m := newTestModule(t)
	db := m.DB()
	if db == nil {
		t.Fatal("DB() returned nil")
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping via DB(): %v", err)
	}
}

func TestPing(t *testing.T) {
	m := newTestModule(t)
	if err := m.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestPing_NotInitialized(t *testing.T) {
	m := New()
	if err := m.Ping(context.Background()); err == nil {
		t.Fatal("expected error for uninitialized module")
	}
}

func TestQuery(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	_, err := m.Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	_, err = m.Exec(ctx, "INSERT INTO test (id, name) VALUES (?, ?)", 1, "alice")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	rows, err := m.Query(ctx, "SELECT id, name FROM test WHERE id = ?", 1)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("expected a row")
	}

	var id int
	var name string
	if err := rows.Scan(&id, &name); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if id != 1 || name != "alice" {
		t.Errorf("got (%d, %q), want (1, alice)", id, name)
	}

	if rows.Next() {
		t.Error("expected no more rows")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("Rows.Err: %v", err)
	}
}

func TestExec(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	_, err := m.Exec(ctx, "CREATE TABLE test2 (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	result, err := m.Exec(ctx, "INSERT INTO test2 (id, val) VALUES (?, ?)", 1, "hello")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("RowsAffected: %v", err)
	}
	if affected != 1 {
		t.Errorf("RowsAffected = %d, want 1", affected)
	}
}

func TestQuery_Closed(t *testing.T) {
	m := newTestModule(t)
	m.Close()

	_, err := m.Query(context.Background(), "SELECT 1")
	if err == nil {
		t.Fatal("expected error when module is closed")
	}
}

func TestExec_Closed(t *testing.T) {
	m := newTestModule(t)
	m.Close()

	_, err := m.Exec(context.Background(), "SELECT 1")
	if err == nil {
		t.Fatal("expected error when module is closed")
	}
}

func TestBeginTx(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	_, err := m.Exec(ctx, "CREATE TABLE tx_test (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	tx, err := m.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}

	_, err = tx.ExecContext(ctx, "INSERT INTO tx_test (id, val) VALUES (?, ?)", 1, "tx-value")
	if err != nil {
		t.Fatalf("INSERT in tx: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	rows, err := m.Query(ctx, "SELECT val FROM tx_test WHERE id = ?", 1)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("expected a row after commit")
	}
	var val string
	if err := rows.Scan(&val); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if val != "tx-value" {
		t.Errorf("val = %q, want tx-value", val)
	}
}

func TestBeginTx_Closed(t *testing.T) {
	m := newTestModule(t)
	m.Close()

	_, err := m.BeginTx(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when module is closed")
	}
}

func TestWithTx_Commit(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	_, err := m.Exec(ctx, "CREATE TABLE withtx_test (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	err = m.WithTx(ctx, nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO withtx_test (id, val) VALUES (?, ?)", 1, "committed")
		return err
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}

	rows, err := m.Query(ctx, "SELECT val FROM withtx_test WHERE id = ?", 1)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("expected row after WithTx commit")
	}
	var val string
	if err := rows.Scan(&val); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if val != "committed" {
		t.Errorf("val = %q, want committed", val)
	}
}

func TestWithTx_Rollback(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	_, err := m.Exec(ctx, "CREATE TABLE withtx_rb (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	err = m.WithTx(ctx, nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO withtx_rb (id, val) VALUES (?, ?)", 1, "rolled-back")
		if err != nil {
			return err
		}
		return sql.ErrNoRows // simulate an error to trigger rollback
	})
	if err == nil {
		t.Fatal("expected error from WithTx")
	}

	rows, err := m.Query(ctx, "SELECT val FROM withtx_rb WHERE id = ?", 1)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		t.Error("expected no rows after rollback")
	}
}

func TestWithTx_Panic(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()

	_, err := m.Exec(ctx, "CREATE TABLE withtx_panic (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic to propagate")
		}

		// Verify the insert was rolled back.
		rows, err := m.Query(ctx, "SELECT val FROM withtx_panic WHERE id = ?", 1)
		if err != nil {
			t.Fatalf("Query after panic: %v", err)
		}
		defer rows.Close()
		if rows.Next() {
			t.Error("expected no rows after panic rollback")
		}
	}()

	_ = m.WithTx(ctx, nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO withtx_panic (id, val) VALUES (?, ?)", 1, "panic-value")
		if err != nil {
			return err
		}
		panic("test panic")
	})
}

func TestWithTx_Closed(t *testing.T) {
	m := newTestModule(t)
	m.Close()

	err := m.WithTx(context.Background(), nil, func(_ *sql.Tx) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error when module is closed")
	}
}

func TestDBTX_Satisfied(t *testing.T) {
	m := newTestModule(t)

	// *sql.DB satisfies DBTX.
	var _ DBTX = m.DB()

	// *sql.Tx satisfies DBTX.
	tx, err := m.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	var _ DBTX = tx
	tx.Rollback()
}
