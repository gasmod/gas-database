package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync/atomic"

	"github.com/gasmod/gas"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	serviceName = "gas-database"
)

// Service manages a database connection and implements both gas.Service
// and gas.DatabaseProvider. In ModeSQL it wraps *sql.DB with any driver.
// In ModePgx it creates a native pgxpool.Pool and derives *sql.DB from
// it via the pgx stdlib adapter, so DB() always works regardless of mode.
type Service struct {
	db                   *sql.DB
	pool                 *pgxpool.Pool // non-nil only in ModePgx
	connector            driver.Connector
	cfg                  *Config
	cfgProvider          gas.ConfigProvider
	customConfigProvided bool
	closed               atomic.Bool
}

// Option configures a Service.
type Option func(*Service)

// WithConfig sets the database configuration.
func WithConfig(cfg *Config) Option {
	return func(s *Service) {
		s.cfg = cfg
		s.customConfigProvided = true
	}
}

// WithConnector sets a driver.Connector for ModeSQL. When provided,
// sql.OpenDB(connector) is used instead of sql.Open(driver, dsn), and
// DatabaseDriver / DatabaseDSN are not required.
func WithConnector(c driver.Connector) Option {
	return func(s *Service) {
		s.connector = c
	}
}

// New captures options and returns a DI-injectable constructor.
// The returned func receives gas.ConfigProvider from the DI container.
func New(opts ...Option) func(gas.ConfigProvider) *Service {
	return func(cfgProvider gas.ConfigProvider) *Service {
		s := &Service{
			cfg:         DefaultConfig(),
			cfgProvider: cfgProvider,
		}
		for _, opt := range opts {
			opt(s)
		}
		return s
	}
}

// Name returns the service identifier.
func (s *Service) Name() string {
	return serviceName
}

// Init opens the database connection, configures the pool, and pings
// the database to verify connectivity.
func (s *Service) Init() error {
	if !s.customConfigProvided {
		// no custom config provided, try to bind from config service
		if s.cfgProvider != nil {
			if err := s.cfgProvider.Bind(s.cfg); err != nil {
				return fmt.Errorf("%s: config binding: %w", s.Name(), err)
			}
		}
	}

	s.cfg.hasConnector = s.connector != nil

	if err := s.cfg.Validate(); err != nil {
		return err
	}

	switch s.cfg.DatabaseMode {
	case ModePgx:
		if err := s.initPgx(); err != nil {
			return err
		}
	case ModeSQL, "":
		if err := s.initSQL(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%s: unknown mode %q", s.Name(), s.cfg.DatabaseMode)
	}

	s.closed.Store(false)
	return nil
}

func (s *Service) initSQL() error {
	var db *sql.DB
	if s.connector != nil {
		db = sql.OpenDB(s.connector)
	} else {
		var err error
		db, err = sql.Open(s.cfg.DatabaseDriver, s.cfg.DatabaseDSN)
		if err != nil {
			return fmt.Errorf("%s: open: %w", s.Name(), err)
		}
	}

	db.SetMaxOpenConns(int(s.cfg.DatabaseMaxOpenConns))
	db.SetMaxIdleConns(s.cfg.DatabaseMaxIdleConns)
	db.SetConnMaxLifetime(s.cfg.DatabaseConnMaxLifetime)
	db.SetConnMaxIdleTime(s.cfg.DatabaseConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), defaultPingTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("%s: ping: %w", s.Name(), err)
	}

	s.db = db
	return nil
}

func (s *Service) initPgx() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPingTimeout)
	defer cancel()

	poolCfg, err := pgxpool.ParseConfig(s.cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("%s: parse pgx config: %w", s.Name(), err)
	}

	if s.cfg.DatabaseMaxOpenConns > 0 {
		poolCfg.MaxConns = s.cfg.DatabaseMaxOpenConns
	}
	if s.cfg.DatabaseConnMaxLifetime > 0 {
		poolCfg.MaxConnLifetime = s.cfg.DatabaseConnMaxLifetime
	}
	if s.cfg.DatabaseConnMaxIdleTime > 0 {
		poolCfg.MaxConnIdleTime = s.cfg.DatabaseConnMaxIdleTime
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("%s: pgxpool: %w", s.Name(), err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("%s: ping: %w", s.Name(), err)
	}

	s.pool = pool
	s.db = stdlib.OpenDBFromPool(pool)
	return nil
}

// Close closes the underlying database connections.
func (s *Service) Close() error {
	s.closed.Store(true)

	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return fmt.Errorf("%s: close: %w", s.Name(), err)
		}
	}

	if s.pool != nil {
		s.pool.Close()
	}

	return nil
}

// DB returns the underlying *sql.DB. This satisfies gas.DatabaseProvider
// and works in both ModeSQL and ModePgx (via stdlib adapter).
func (s *Service) DB() *sql.DB {
	return s.db
}

// Pool returns the native pgxpool.Pool. Returns nil when running in
// ModeSQL. Consuming services that want native pgx access can define a
// local interface (e.g., type PgxProvider interface { Pool() *pgxpool.Pool })
// and type-assert the DatabaseProvider.
func (s *Service) Pool() *pgxpool.Pool {
	return s.pool
}

// Ping verifies the database connection is still alive.
func (s *Service) Ping(ctx context.Context) error {
	if s.pool != nil {
		if err := s.pool.Ping(ctx); err != nil {
			return fmt.Errorf("%s: ping: %w", s.Name(), err)
		}
		return nil
	}
	if s.db == nil {
		return fmt.Errorf("%s: not initialized", s.Name())
	}
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("%s: ping: %w", s.Name(), err)
	}
	return nil
}
