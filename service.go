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
	logger               gas.Logger
	customConfigProvided bool
	closed               atomic.Bool
}

var _ gas.Service = (*Service)(nil)
var _ gas.DatabaseProvider = (*Service)(nil)

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
func New(opts ...Option) func(gas.ConfigProvider, gas.Logger) *Service {
	return func(cfgProvider gas.ConfigProvider, logger gas.Logger) *Service {
		s := &Service{
			cfg:         DefaultConfig(),
			cfgProvider: cfgProvider,
			logger:      logger.With().Str("service", serviceName).Logger(),
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
		s.logger.Error("invalid database configuration").Err("error", err).Send()
		return err
	}

	switch s.cfg.Database.Mode {
	case ModePgx:
		if err := s.initPgx(); err != nil {
			s.logger.Error("failed to initialize pgx pool").Err("error", err).Send()
			return err
		}
	case ModeSQL, "":
		if err := s.initSQL(); err != nil {
			s.logger.Error("failed to initialize SQL pool").Err("error", err).Send()
			return err
		}
	default:
		s.logger.Error("unknown database mode").Str("mode", s.cfg.Database.Mode).Send()
		return fmt.Errorf("%s: unknown mode %q", s.Name(), s.cfg.Database.Mode)
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
		db, err = sql.Open(s.cfg.Database.Driver, s.cfg.Database.DSN)
		if err != nil {
			s.logger.Error("failed to open database connection").Err("error", err).Send()
			return fmt.Errorf("%s: open: %w", s.Name(), err)
		}
	}

	db.SetMaxOpenConns(int(s.cfg.Database.MaxOpenConns))
	db.SetMaxIdleConns(s.cfg.Database.MaxIdleConns)
	db.SetConnMaxLifetime(s.cfg.Database.ConnMaxLifetime)
	db.SetConnMaxIdleTime(s.cfg.Database.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), defaultPingTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		s.logger.Error("failed to ping database connection").Err("error", err).Send()
		return fmt.Errorf("%s: ping: %w", s.Name(), err)
	}

	s.logger.Info("SQL database connection initialized").Send()

	s.db = db
	return nil
}

func (s *Service) initPgx() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPingTimeout)
	defer cancel()

	poolCfg, err := pgxpool.ParseConfig(s.cfg.Database.DSN)
	if err != nil {
		s.logger.Error("failed to parse pgx config").Err("error", err).Send()
		return fmt.Errorf("%s: parse pgx config: %w", s.Name(), err)
	}

	if s.cfg.Database.MaxOpenConns > 0 {
		poolCfg.MaxConns = s.cfg.Database.MaxOpenConns
	}
	if s.cfg.Database.ConnMaxLifetime > 0 {
		poolCfg.MaxConnLifetime = s.cfg.Database.ConnMaxLifetime
	}
	if s.cfg.Database.ConnMaxIdleTime > 0 {
		poolCfg.MaxConnIdleTime = s.cfg.Database.ConnMaxIdleTime
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		s.logger.Error("failed to create pgx pool").Err("error", err).Send()
		return fmt.Errorf("%s: pgxpool: %w", s.Name(), err)
	}

	if pErr := pool.Ping(ctx); pErr != nil {
		pool.Close()
		s.logger.Error("failed to ping pgx pool").Err("error", pErr).Send()
		return fmt.Errorf("%s: ping: %w", s.Name(), pErr)
	}

	s.logger.Info("PGX database connection initialized").Send()

	s.pool = pool
	s.db = stdlib.OpenDBFromPool(pool)
	return nil
}

// Close closes the underlying database connections.
func (s *Service) Close() error {
	s.closed.Store(true)

	if s.db != nil {
		if err := s.db.Close(); err != nil {
			s.logger.Error("failed to close database connection").Err("error", err).Send()
			return fmt.Errorf("%s: close: %w", s.Name(), err)
		}

		s.logger.Info("database connection closed").Send()
	}

	if s.pool != nil {
		s.pool.Close()

		s.logger.Info("pgx pool closed").Send()
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

// Driver returns the database driver name based on the configured mode and settings.
func (s *Service) Driver() string {
	if s.cfg.Database.Mode == ModePgx {
		return DriverPgx
	}
	return s.cfg.Database.Driver
}
