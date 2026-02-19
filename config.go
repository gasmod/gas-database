package database

import (
	"errors"
	"fmt"
	"time"
)

// Mode selects the database backend.
const (
	// ModeSQL uses database/sql with any registered driver.
	ModeSQL = "sql"

	// ModePgx uses native pgxpool.Pool for PostgreSQL. DB() still works
	// via the pgx stdlib adapter. Pool() returns the native pool for
	// sqlc pgx mode.
	ModePgx = "pgx"
)

const (
	// DriverPostgres defines the constant for the PostgreSQL database driver.
	DriverPostgres = "postgres"

	// DriverPgx represents the const string identifier for the "pgx" database driver.
	DriverPgx = "pgx"

	// DriverSQLite represents the identifier for the SQLite database driver.
	DriverSQLite = "sqlite"
)

const (
	defaultMode            = ModeSQL
	defaultDriver          = "postgres"
	defaultMaxOpenConns    = 25
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 30 * time.Minute
	defaultConnMaxIdleTime = 5 * time.Minute
	defaultPingTimeout     = 5 * time.Second
)

// Config holds database connection settings.
type Config struct {
	// Mode selects the backend: ModeSQL (default) or ModePgx.
	DatabaseMode string

	// Driver is the database/sql driver name (e.g., "postgres", "pgx", "sqlite").
	// Only used in ModeSQL.
	DatabaseDriver string

	// DSN is the data source name (connection string).
	DatabaseDSN string

	// MaxOpenConns is the maximum number of open connections to the database.
	DatabaseMaxOpenConns int32

	// hasConnector is set internally when a driver.Connector is provided.
	// When true, DatabaseDriver and DatabaseDSN are not required in ModeSQL.
	hasConnector bool

	// MaxIdleConns is the maximum number of idle connections in the pool.
	// Only used in ModeSQL; pgx manages idle connections internally.
	DatabaseMaxIdleConns int

	// ConnMaxLifetime is the maximum amount of time a connection may be reused.
	DatabaseConnMaxLifetime time.Duration

	// ConnMaxIdleTime is the maximum amount of time a connection may be idle.
	DatabaseConnMaxIdleTime time.Duration
}

// DefaultConfig returns a Config with sensible defaults using database/sql.
func DefaultConfig() *Config {
	return &Config{
		DatabaseMode:            defaultMode,
		DatabaseDriver:          defaultDriver,
		DatabaseMaxOpenConns:    defaultMaxOpenConns,
		DatabaseMaxIdleConns:    defaultMaxIdleConns,
		DatabaseConnMaxLifetime: defaultConnMaxLifetime,
		DatabaseConnMaxIdleTime: defaultConnMaxIdleTime,
	}
}

var (
	validModes   = map[string]bool{ModeSQL: true, ModePgx: true}
	validDrivers = map[string]bool{DriverPostgres: true, DriverPgx: true, DriverSQLite: true}
)

const (
	minMaxOpenConns = 1
	maxMaxOpenConns = 1000
	minMaxIdleConns = 1
	maxMaxIdleConns = 1000

	minConnMaxLifetime = 1 * time.Second
	maxConnMaxLifetime = 2 * time.Hour
	minConnMaxIdleTime = 1 * time.Second
	maxConnMaxIdleTime = 1 * time.Hour
)

// Validate checks the Config struct for correctness and returns an error if any validation rule is violated.
// nolint:cyclop,gocyclo // intentionally complex
func (c *Config) Validate() error {
	if !validModes[c.DatabaseMode] {
		return fmt.Errorf("DatabaseMode must be one of [%s, %s], got %q", ModeSQL, ModePgx, c.DatabaseMode)
	}
	if c.DatabaseMode == ModeSQL && !c.hasConnector && !validDrivers[c.DatabaseDriver] {
		return fmt.Errorf("DatabaseDriver must be one of [%s, %s, %s], got %q", DriverPostgres, DriverPgx, DriverSQLite, c.DatabaseDriver)
	}
	if c.DatabaseDSN == "" && !c.hasConnector {
		return errors.New("DatabaseDSN must not be empty")
	}
	if c.DatabaseMaxOpenConns < minMaxOpenConns || c.DatabaseMaxOpenConns > maxMaxOpenConns {
		return fmt.Errorf("DatabaseMaxOpenConns must be between %d and %d, got %d", minMaxOpenConns, maxMaxOpenConns, c.DatabaseMaxOpenConns)
	}
	if c.DatabaseMode == ModeSQL {
		if c.DatabaseMaxIdleConns < minMaxIdleConns || c.DatabaseMaxIdleConns > maxMaxIdleConns {
			return fmt.Errorf("DatabaseMaxIdleConns must be between %d and %d, got %d", minMaxIdleConns, maxMaxIdleConns, c.DatabaseMaxIdleConns)
		}
		if c.DatabaseMaxIdleConns > int(c.DatabaseMaxOpenConns) {
			return fmt.Errorf("DatabaseMaxIdleConns (%d) must not exceed DatabaseMaxOpenConns (%d)", c.DatabaseMaxIdleConns, c.DatabaseMaxOpenConns)
		}
	}
	if c.DatabaseConnMaxLifetime < minConnMaxLifetime || c.DatabaseConnMaxLifetime > maxConnMaxLifetime {
		return fmt.Errorf("DatabaseConnMaxLifetime must be between %s and %s, got %s", minConnMaxLifetime, maxConnMaxLifetime, c.DatabaseConnMaxLifetime)
	}
	if c.DatabaseConnMaxIdleTime < minConnMaxIdleTime || c.DatabaseConnMaxIdleTime > maxConnMaxIdleTime {
		return fmt.Errorf("DatabaseConnMaxIdleTime must be between %s and %s, got %s", minConnMaxIdleTime, maxConnMaxIdleTime, c.DatabaseConnMaxIdleTime)
	}
	if c.DatabaseConnMaxIdleTime > c.DatabaseConnMaxLifetime {
		return fmt.Errorf("DatabaseConnMaxIdleTime (%s) must not exceed DatabaseConnMaxLifetime (%s)", c.DatabaseConnMaxIdleTime, c.DatabaseConnMaxLifetime)
	}
	return nil
}
