package database_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/gasmod/gas"
	database "github.com/gasmod/gas-database"

	_ "modernc.org/sqlite"
)

// Compile-time interface checks.
var (
	_ gas.Service          = (*database.Service)(nil)
	_ gas.DatabaseProvider = (*database.Service)(nil)
)

func newTestService(t *testing.T) *database.Service {
	t.Helper()

	cfg := database.DefaultConfig()
	cfg.DatabaseDriver = "sqlite"
	cfg.DatabaseDSN = filepath.Join(t.TempDir(), "test.db")

	s := database.New(database.WithConfig(cfg))(nil)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestName(t *testing.T) {
	s := database.New()(nil)
	if s.Name() != "gas-database" {
		t.Fatalf("expected gas-database, got %s", s.Name())
	}
}

func TestInit_NoDSN(t *testing.T) {
	s := database.New()(nil)
	if err := s.Init(); err == nil {
		t.Fatal("expected error for missing DSN")
	}
}

func TestInit_Close_Lifecycle(t *testing.T) {
	s := newTestService(t)
	if s.DB() == nil {
		t.Fatal("DB() should not be nil after Init")
	}
}

func TestDB_ReturnsConnection(t *testing.T) {
	s := newTestService(t)
	db := s.DB()
	if db == nil {
		t.Fatal("DB() returned nil")
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping via DB(): %v", err)
	}
}

func TestPing(t *testing.T) {
	s := newTestService(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestPing_NotInitialized(t *testing.T) {
	s := database.New()(nil)
	if err := s.Ping(context.Background()); err == nil {
		t.Fatal("expected error for uninitialized service")
	}
}

func TestQuery(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	_, err := s.Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	_, err = s.Exec(ctx, "INSERT INTO test (id, name) VALUES (?, ?)", 1, "alice")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	rows, err := s.Query(ctx, "SELECT id, name FROM test WHERE id = ?", 1)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("expected a row")
	}

	var id int
	var name string
	if err = rows.Scan(&id, &name); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if id != 1 || name != "alice" {
		t.Errorf("got (%d, %q), want (1, alice)", id, name)
	}

	if rows.Next() {
		t.Error("expected no more rows")
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("Rows.Err: %v", err)
	}
}

func TestExec(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	_, err := s.Exec(ctx, "CREATE TABLE test2 (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	result, err := s.Exec(ctx, "INSERT INTO test2 (id, val) VALUES (?, ?)", 1, "hello")
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
	s := newTestService(t)
	s.Close()

	_, err := s.Query(context.Background(), "SELECT 1")
	if err == nil {
		t.Fatal("expected error when service is closed")
	}
}

func TestExec_Closed(t *testing.T) {
	s := newTestService(t)
	s.Close()

	_, err := s.Exec(context.Background(), "SELECT 1")
	if err == nil {
		t.Fatal("expected error when service is closed")
	}
}

func TestBeginTx(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	_, err := s.Exec(ctx, "CREATE TABLE tx_test (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	tx, err := s.BeginTx(ctx, nil)
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

	rows, err := s.Query(ctx, "SELECT val FROM tx_test WHERE id = ?", 1)
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
	s := newTestService(t)
	s.Close()

	_, err := s.BeginTx(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when service is closed")
	}
}

func TestWithTx_Commit(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	_, err := s.Exec(ctx, "CREATE TABLE withtx_test (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	err = s.WithTx(ctx, nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO withtx_test (id, val) VALUES (?, ?)", 1, "committed")
		return err
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}

	rows, err := s.Query(ctx, "SELECT val FROM withtx_test WHERE id = ?", 1)
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
	s := newTestService(t)
	ctx := context.Background()

	_, err := s.Exec(ctx, "CREATE TABLE withtx_rb (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	err = s.WithTx(ctx, nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO withtx_rb (id, val) VALUES (?, ?)", 1, "rolled-back")
		if err != nil {
			return err
		}
		return sql.ErrNoRows // simulate an error to trigger rollback
	})
	if err == nil {
		t.Fatal("expected error from WithTx")
	}

	rows, err := s.Query(ctx, "SELECT val FROM withtx_rb WHERE id = ?", 1)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		t.Error("expected no rows after rollback")
	}
}

func TestWithTx_Panic(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	_, err := s.Exec(ctx, "CREATE TABLE withtx_panic (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic to propagate")
		}

		// Verify the insert was rolled back.
		rows, err := s.Query(ctx, "SELECT val FROM withtx_panic WHERE id = ?", 1)
		if err != nil {
			t.Fatalf("Query after panic: %v", err)
		}
		defer rows.Close()
		if rows.Next() {
			t.Error("expected no rows after panic rollback")
		}
	}()

	_ = s.WithTx(ctx, nil, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO withtx_panic (id, val) VALUES (?, ?)", 1, "panic-value")
		if err != nil {
			return err
		}
		panic("test panic")
	})
}

func TestWithTx_Closed(t *testing.T) {
	s := newTestService(t)
	s.Close()

	err := s.WithTx(context.Background(), nil, func(_ *sql.Tx) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error when service is closed")
	}
}

func TestDBTX_Satisfied(t *testing.T) {
	s := newTestService(t)

	// *sql.DB satisfies DBTX.
	var _ database.DBTX = s.DB()

	// *sql.Tx satisfies DBTX.
	tx, err := s.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	var _ database.DBTX = tx
	tx.Rollback()
}
