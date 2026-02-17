package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// Module manages a database connection and implements both gas.Module
// and gas.DatabaseProvider. In ModeSQL it wraps *sql.DB with any driver.
// In ModePgx it creates a native pgxpool.Pool and derives *sql.DB from
// it via the pgx stdlib adapter, so DB() always works regardless of mode.
type Module struct {
	db     *sql.DB
	pool   *pgxpool.Pool // non-nil only in ModePgx
	cfg    *Config
	closed atomic.Bool
}

// Option configures a Module.
type Option func(*Module)

// WithConfig sets the database configuration.
func WithConfig(cfg *Config) Option {
	return func(m *Module) {
		m.cfg = cfg
	}
}

// New creates a Module with the given options. Call Init() to open the
// connection.
func New(opts ...Option) *Module {
	m := &Module{
		cfg: DefaultConfig(),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "gas-database"
}

// Init opens the database connection, configures the pool, and pings
// the database to verify connectivity.
func (m *Module) Init() error {
	if m.cfg.DSN == "" {
		return fmt.Errorf("gas-database: DSN is required")
	}

	switch m.cfg.Mode {
	case ModePgx:
		if err := m.initPgx(); err != nil {
			return err
		}
	case ModeSQL, "":
		if err := m.initSQL(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("gas-database: unknown mode %q", m.cfg.Mode)
	}

	m.closed.Store(false)
	return nil
}

func (m *Module) initSQL() error {
	db, err := sql.Open(m.cfg.Driver, m.cfg.DSN)
	if err != nil {
		return fmt.Errorf("gas-database: open: %w", err)
	}

	db.SetMaxOpenConns(int(m.cfg.MaxOpenConns))
	db.SetMaxIdleConns(m.cfg.MaxIdleConns)
	db.SetConnMaxLifetime(m.cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(m.cfg.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), defaultPingTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("gas-database: ping: %w", err)
	}

	m.db = db
	return nil
}

func (m *Module) initPgx() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPingTimeout)
	defer cancel()

	poolCfg, err := pgxpool.ParseConfig(m.cfg.DSN)
	if err != nil {
		return fmt.Errorf("gas-database: parse pgx config: %w", err)
	}

	if m.cfg.MaxOpenConns > 0 {
		poolCfg.MaxConns = m.cfg.MaxOpenConns
	}
	if m.cfg.ConnMaxLifetime > 0 {
		poolCfg.MaxConnLifetime = m.cfg.ConnMaxLifetime
	}
	if m.cfg.ConnMaxIdleTime > 0 {
		poolCfg.MaxConnIdleTime = m.cfg.ConnMaxIdleTime
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("gas-database: pgxpool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("gas-database: ping: %w", err)
	}

	m.pool = pool
	m.db = stdlib.OpenDBFromPool(pool)
	return nil
}

// Close closes the underlying database connections.
func (m *Module) Close() error {
	m.closed.Store(true)

	if m.db != nil {
		if err := m.db.Close(); err != nil {
			return fmt.Errorf("gas-database: close: %w", err)
		}
	}

	if m.pool != nil {
		m.pool.Close()
	}

	return nil
}

// DB returns the underlying *sql.DB. This satisfies gas.DatabaseProvider
// and works in both ModeSQL and ModePgx (via stdlib adapter).
func (m *Module) DB() *sql.DB {
	return m.db
}

// Pool returns the native pgxpool.Pool. Returns nil when running in
// ModeSQL. Consuming modules that want native pgx access can define a
// local interface (e.g., type PgxProvider interface { Pool() *pgxpool.Pool })
// and type-assert the DatabaseProvider.
func (m *Module) Pool() *pgxpool.Pool {
	return m.pool
}

// Ping verifies the database connection is still alive.
func (m *Module) Ping(ctx context.Context) error {
	if m.pool != nil {
		if err := m.pool.Ping(ctx); err != nil {
			return fmt.Errorf("gas-database: ping: %w", err)
		}
		return nil
	}
	if m.db == nil {
		return fmt.Errorf("gas-database: not initialized")
	}
	if err := m.db.PingContext(ctx); err != nil {
		return fmt.Errorf("gas-database: ping: %w", err)
	}
	return nil
}
