package database

import "time"

const defaultPingTimeout = 5 * time.Second

// Mode selects the database backend.
const (
	// ModeSQL uses database/sql with any registered driver.
	ModeSQL = "sql"

	// ModePgx uses native pgxpool.Pool for PostgreSQL. DB() still works
	// via the pgx stdlib adapter. Pool() returns the native pool for
	// sqlc pgx mode.
	ModePgx = "pgx"
)

// Config holds database connection settings.
type Config struct {
	// Mode selects the backend: ModeSQL (default) or ModePgx.
	Mode string

	// Driver is the database/sql driver name (e.g., "postgres", "pgx", "sqlite").
	// Only used in ModeSQL.
	Driver string

	// DSN is the data source name (connection string).
	DSN string

	// MaxOpenConns is the maximum number of open connections to the database.
	MaxOpenConns int32

	// MaxIdleConns is the maximum number of idle connections in the pool.
	// Only used in ModeSQL; pgx manages idle connections internally.
	MaxIdleConns int

	// ConnMaxLifetime is the maximum amount of time a connection may be reused.
	ConnMaxLifetime time.Duration

	// ConnMaxIdleTime is the maximum amount of time a connection may be idle.
	ConnMaxIdleTime time.Duration
}

// DefaultConfig returns a Config with sensible defaults using database/sql.
func DefaultConfig() *Config {
	return &Config{
		Mode:            ModeSQL,
		Driver:          "postgres",
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}
}
