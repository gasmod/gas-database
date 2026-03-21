---
name: gas-database
description: >
  Reference documentation for the gas-database Go package
  (github.com/gasmod/gas-database) — the database connection service for the
  Gas ecosystem. Use this skill when writing, reviewing, or debugging Go code
  that uses gas-database for database access, transactions, connection pooling,
  sqlc integration, or PostgreSQL/SQLite connectivity. Covers ModeSQL and
  ModePgx backends, DI wiring via gas.DatabaseProvider, transaction helpers
  (BeginTx, WithTx), pgxpool native access, connector injection, DBTX
  interface, connection retry with exponential backoff, and configuration
  binding. Make sure to use this skill whenever working with database
  connections in the Gas ecosystem, even if the user doesn't explicitly
  mention gas-database.
---

# Gas Database Package Reference

Database connection service for the Gas ecosystem. Wraps `database/sql` and
native `pgxpool` to provide connection management, transaction helpers, and
sqlc compatibility.

```
import database "github.com/gasmod/gas-database"
```

## Choosing a Mode

| Mode                                  | Backend        | Use case                                                          |
|---------------------------------------|----------------|-------------------------------------------------------------------|
| `database.ModeSQL` (`"sql"`, default) | `database/sql` | Any driver: PostgreSQL, SQLite, MySQL                             |
| `database.ModePgx` (`"pgx"`)         | `pgxpool.Pool` | Native pgx for PostgreSQL (better perf, pgx types, batch queries) |

In both modes, `DB()` returns `*sql.DB` so sqlc `database/sql` mode always
works. In pgx mode, `Pool()` additionally returns `*pgxpool.Pool` for sqlc pgx
mode.

## Constructor

```go
func New(opts ...Option) func(gas.ConfigProvider, gas.Logger) *Service
```

`New` captures options and returns a DI-injectable constructor. The returned
func receives `gas.ConfigProvider` and `gas.Logger` from the DI container.

### Options

| Option                              | Description                                                                 |
|-------------------------------------|-----------------------------------------------------------------------------|
| `WithConfig(cfg *Config)`           | Set database configuration explicitly (skips config binding from DI)        |
| `WithConnector(c driver.Connector)` | Provide a `driver.Connector` for ModeSQL; uses `sql.OpenDB(connector)`      |

- `WithConnector` — `Database.Driver` and `Database.DSN` are not required when
  a connector is set.
- If `WithConfig` is not provided, the service automatically binds
  configuration from the `gas.ConfigProvider` injected via DI.

## Service

`Service` implements both `gas.Service` and `gas.DatabaseProvider`.

### Lifecycle (gas.Service)

| Method    | Signature        | Description                              |
|-----------|------------------|------------------------------------------|
| `Name`    | `() string`      | Returns `"gas-database"`                 |
| `Init`    | `() error`       | Opens connection, configures pool, pings |
| `Close`   | `() error`       | Closes underlying connections            |

### Database Access

| Method   | Signature                                                    | Description                                 |
|----------|--------------------------------------------------------------|---------------------------------------------|
| `DB`     | `() *sql.DB`                                                 | Always works in both modes                  |
| `Pool`   | `() *pgxpool.Pool`                                           | Native pool; nil in ModeSQL                 |
| `Query`  | `(ctx, query string, args ...any) (gas.Rows, error)`        | Implements `gas.DatabaseProvider`           |
| `Exec`   | `(ctx, query string, args ...any) (gas.Result, error)`      | Implements `gas.DatabaseProvider`           |
| `Ping`   | `(ctx context.Context) error`                                | Verify connectivity                         |
| `Driver` | `() string`                                                  | Returns driver name based on mode/settings  |

`Query` and `Exec` return an error if the service has been closed.

### Transactions

```go
// Manual — caller commits/rolls back
func (s *Service) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)

// Automatic — commits if fn returns nil, rolls back on error or panic
func (s *Service) WithTx(ctx context.Context, opts *sql.TxOptions, fn func(*sql.Tx) error) error
```

## DBTX Interface

Matches what sqlc generates. Both `*sql.DB` and `*sql.Tx` satisfy it:

```go
type DBTX interface {
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
    PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
    QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
```

## Config

```go
type Config struct {
    Database Settings
}

type Settings struct {
    Mode              string        // "sql" (default) or "pgx"
    Driver            string        // database/sql driver name, default "postgres" (ModeSQL only)
    DSN               string        // connection string (required unless WithConnector)
    MaxOpenConns      int32         // default 25
    MaxIdleConns      int           // default 5 (ModeSQL only, pgx manages internally)
    ConnMaxLifetime   time.Duration // default 30m
    ConnMaxIdleTime   time.Duration // default 5m
    ConnRetries       int           // default 0 (no retries); number of retry attempts on connect failure
    ConnRetryInterval time.Duration // default 2s; base interval, doubles each attempt (exponential backoff)
}

func DefaultConfig() *Config
func (c *Config) Validate() error
```

### Connection Retry

When `ConnRetries > 0`, the service retries the initial connection with
exponential backoff. The interval starts at `ConnRetryInterval` (default 2s)
and doubles after each failed attempt. If all retries are exhausted, `Init`
returns an error.

## Driver Constants

```go
const (
    DriverPostgres = "postgres"
    DriverPgx      = "pgx"
    DriverSQLite   = "sqlite"
)
```

## DI Wiring Patterns

### Basic registration

```go
app := gas.NewApp(
    gas.WithService[*database.Service](
        database.New(database.WithConfig(&database.Config{
            Database: database.Settings{
                DSN:    "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
                Driver: "pgx",
            },
        })),
        gas.ServiceLifetimeSingleton,
    ),
)
```

### Native pgx mode

```go
database.New(database.WithConfig(&database.Config{
    Database: database.Settings{
        Mode: database.ModePgx,
        DSN:  "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
    },
}))
// After Init(): svc.DB() -> *sql.DB, svc.Pool() -> *pgxpool.Pool
```

### SQLite

```go
import _ "modernc.org/sqlite"

database.New(database.WithConfig(&database.Config{
    Database: database.Settings{
        Driver: "sqlite",
        DSN:    "./app.db",
    },
}))
```

### Custom connector

```go
import "github.com/jackc/pgx/v5/stdlib"

connConfig, _ := pgx.ParseConfig("postgres://user:pass@localhost:5432/mydb")
connector := stdlib.GetConnector(*connConfig)
database.New(database.WithConnector(connector))
```

### With connection retry

```go
database.New(database.WithConfig(&database.Config{
    Database: database.Settings{
        DSN:               "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
        ConnRetries:       3,                // retry up to 3 times
        ConnRetryInterval: 2 * time.Second,  // 2s, 4s, 8s backoff
    },
}))
```

### Consuming via gas.DatabaseProvider

Services receive the database through the provider interface, never importing
gas-database directly:

```go
type Service struct {
    db gas.DatabaseProvider
}

func New(db gas.DatabaseProvider) *Service {
    return &Service{db: db}
}

func (s *Service) Init() error {
    s.queries = authdb.New(s.db.DB()) // sqlc database/sql mode
    return nil
}
```

### Native pgx access via local interface

```go
// Define locally where consumed
type PgxProvider interface {
    Pool() *pgxpool.Pool
}

func (s *Service) Init() error {
    if pp, ok := s.db.(PgxProvider); ok && pp.Pool() != nil {
        s.queries = authdb.New(pp.Pool()) // sqlc pgx mode
    } else {
        s.queries = authdb.New(s.db.DB()) // fallback
    }
    return nil
}
```

### Transaction patterns with sqlc

```go
// Manual
tx, err := dbSvc.BeginTx(ctx, nil)
qtx := queries.WithTx(tx)
// ... use qtx ...
tx.Commit()

// Automatic
dbSvc.WithTx(ctx, nil, func(tx *sql.Tx) error {
    qtx := queries.WithTx(tx)
    if err := qtx.CreateUser(ctx, params); err != nil {
        return err // rollback
    }
    return qtx.CreateProfile(ctx, params) // commit if nil
})
```

## Complete Example

```go
package main

import (
    "time"

    _ "github.com/jackc/pgx/v5/stdlib"

    "github.com/gasmod/gas"
    database "github.com/gasmod/gas-database"
)

func main() {
    app := gas.NewApp(
        // Register database as a singleton service
        gas.WithService[*database.Service](
            database.New(database.WithConfig(&database.Config{
                Database: database.Settings{
                    DSN:               "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
                    Driver:            "pgx",
                    MaxOpenConns:      25,
                    MaxIdleConns:      5,
                    ConnMaxLifetime:   30 * time.Minute,
                    ConnMaxIdleTime:   5 * time.Minute,
                    ConnRetries:       3,               // retry on startup failure
                    ConnRetryInterval: 2 * time.Second, // exponential backoff: 2s, 4s, 8s
                },
            })),
            gas.ServiceLifetimeSingleton, // shared across all services
        ),
        // Other services receive *database.Service as gas.DatabaseProvider via DI
    )

    app.Run()
}
```
