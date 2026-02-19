package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"

	"github.com/gasmod/gas"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	moduleName = "gas-database"
)

// Module manages a database connection and implements both gas.Module
// and gas.DatabaseProvider. In ModeSQL it wraps *sql.DB with any driver.
// In ModePgx it creates a native pgxpool.Pool and derives *sql.DB from
// it via the pgx stdlib adapter, so DB() always works regardless of mode.
type Module struct {
	db                   *sql.DB
	pool                 *pgxpool.Pool // non-nil only in ModePgx
	cfg                  *Config
	cfgProvider          gas.ConfigProvider
	customConfigProvided bool
	closed               atomic.Bool
}

// Option configures a Module.
type Option func(*Module)

// WithConfig sets the database configuration.
func WithConfig(cfg *Config) Option {
	return func(m *Module) {
		m.cfg = cfg
		m.customConfigProvided = true
	}
}

// WithConfigProvider sets a configuration provider for the Module.
func WithConfigProvider(provider gas.ConfigProvider) Option {
	return func(m *Module) {
		m.cfgProvider = provider
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
	return moduleName
}

// Init opens the database connection, configures the pool, and pings
// the database to verify connectivity.
func (m *Module) Init() error {
	if !m.customConfigProvided {
		// no custom config provided, try to bind from config-module
		if m.cfgProvider != nil {
			if err := m.cfgProvider.Bind(m.cfg); err != nil {
				return fmt.Errorf("%s: config binding: %w", m.Name(), err)
			}
		}
	}

	if err := m.cfg.Validate(); err != nil {
		return err
	}

	switch m.cfg.DatabaseMode {
	case ModePgx:
		if err := m.initPgx(); err != nil {
			return err
		}
	case ModeSQL, "":
		if err := m.initSQL(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%s: unknown mode %q", m.Name(), m.cfg.DatabaseMode)
	}

	m.closed.Store(false)
	return nil
}

func (m *Module) initSQL() error {
	db, err := sql.Open(m.cfg.DatabaseDriver, m.cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("%s: open: %w", m.Name(), err)
	}

	db.SetMaxOpenConns(int(m.cfg.DatabaseMaxOpenConns))
	db.SetMaxIdleConns(m.cfg.DatabaseMaxIdleConns)
	db.SetConnMaxLifetime(m.cfg.DatabaseConnMaxLifetime)
	db.SetConnMaxIdleTime(m.cfg.DatabaseConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), defaultPingTimeout)
	defer cancel()

	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("%s: ping: %w", m.Name(), err)
	}

	m.db = db
	return nil
}

func (m *Module) initPgx() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPingTimeout)
	defer cancel()

	poolCfg, err := pgxpool.ParseConfig(m.cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("%s: parse pgx config: %w", m.Name(), err)
	}

	if m.cfg.DatabaseMaxOpenConns > 0 {
		poolCfg.MaxConns = m.cfg.DatabaseMaxOpenConns
	}
	if m.cfg.DatabaseConnMaxLifetime > 0 {
		poolCfg.MaxConnLifetime = m.cfg.DatabaseConnMaxLifetime
	}
	if m.cfg.DatabaseConnMaxIdleTime > 0 {
		poolCfg.MaxConnIdleTime = m.cfg.DatabaseConnMaxIdleTime
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("%s: pgxpool: %w", m.Name(), err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("%s: ping: %w", m.Name(), err)
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
			return fmt.Errorf("%s: close: %w", m.Name(), err)
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
			return fmt.Errorf("%s: ping: %w", m.Name(), err)
		}
		return nil
	}
	if m.db == nil {
		return fmt.Errorf("%s: not initialized", m.Name())
	}
	if err := m.db.PingContext(ctx); err != nil {
		return fmt.Errorf("%s: ping: %w", m.Name(), err)
	}
	return nil
}
