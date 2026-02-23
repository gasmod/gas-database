# gas-database

Database connection service for the [Gas](https://github.com/gasmod/gas) ecosystem. Wraps `database/sql` and native
`pgxpool` to provide connection management, transaction helpers, and sqlc compatibility.

## Install

```bash
go get github.com/gasmod/gas-database
```

## Modes

| Mode                         | Backend        | Use case                                                                 |
|------------------------------|----------------|--------------------------------------------------------------------------|
| `database.ModeSQL` (default) | `database/sql` | Any driver: PostgreSQL, SQLite, MySQL, etc.                              |
| `database.ModePgx`           | `pgxpool.Pool` | Native pgx for PostgreSQL (better performance, pgx types, batch queries) |

In both modes, `DB()` returns a `*sql.DB` so sqlc `database/sql` mode always works. In pgx mode, `Pool()` additionally
returns the native `*pgxpool.Pool` for sqlc pgx mode.

## Usage

### Basic setup (database/sql)

```go
package main

import (
	_ "github.com/jackc/pgx/v5/stdlib" // register pgx as database/sql driver

	"github.com/gasmod/gas"
	database "github.com/gasmod/gas-database"
)

func main() {
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
		// ...
	)

	app.Run()
}
```

### Native pgx mode

```go
database.New(database.WithConfig(&database.Config{
	Database: database.Settings{
		Mode: database.ModePgx,
		DSN:  "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
	},
}))

// After Init(), both are available:
// svc.DB()   -> *sql.DB (via stdlib adapter)
// svc.Pool() -> *pgxpool.Pool
```

### Using a connector (sql.OpenDB)

When you need full control over connection setup (e.g., custom TLS, auth tokens), pass a `driver.Connector` directly:

```go
import "github.com/jackc/pgx/v5/stdlib"

connConfig, _ := pgx.ParseConfig("postgres://user:pass@localhost:5432/mydb")
connector := stdlib.GetConnector(*connConfig)

database.New(database.WithConnector(connector))
```

When a connector is provided, `Database.Driver` and `Database.DSN` are not required.

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

### Dependency injection

Services receive the database through `gas.DatabaseProvider` via constructor injection:

```go
// gas-auth/service.go
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

For services that want native pgx access, define a local interface and type-assert:

```go
// gas-auth/providers.go
type PgxProvider interface {
	Pool() *pgxpool.Pool
}

// gas-auth/service.go
func (s *Service) Init() error {
	if pp, ok := s.db.(PgxProvider); ok && pp.Pool() != nil {
		s.queries = authdb.New(pp.Pool()) // sqlc pgx mode
	} else {
		s.queries = authdb.New(s.db.DB()) // fallback to database/sql
	}
	return nil
}
```

### Transactions

Manual transaction management:

```go
tx, err := dbSvc.BeginTx(ctx, nil)
if err != nil {
	return err
}
// use tx with sqlc: queries.WithTx(tx)
err = tx.Commit()
```

Automatic commit/rollback with `WithTx`:

```go
err := dbSvc.WithTx(ctx, nil, func(tx *sql.Tx) error {
	qtx := queries.WithTx(tx)
	if err := qtx.CreateUser(ctx, params); err != nil {
		return err // triggers rollback
	}
	return qtx.CreateProfile(ctx, params) // commits if nil
})
```

`WithTx` also rolls back on panic.

## Config

If `WithConfig` is not provided, the service automatically binds configuration from the `gas.ConfigProvider` injected
via DI. This lets you drive database settings from environment variables or a config file without any explicit wiring.

| Field                        | Default      | Description                                               |
|------------------------------|--------------|-----------------------------------------------------------|
| `Database.Mode`              | `"sql"`      | Backend mode: `"sql"` or `"pgx"`                          |
| `Database.Driver`            | `"postgres"` | `database/sql` driver name (ModeSQL only)                 |
| `Database.DSN`               |              | Connection string (required unless using `WithConnector`) |
| `Database.MaxOpenConns`      | `25`         | Max open connections                                      |
| `Database.MaxIdleConns`      | `5`          | Max idle connections (ModeSQL only)                       |
| `Database.ConnMaxLifetime`   | `30m`        | Max connection reuse time                                 |
| `Database.ConnMaxIdleTime`   | `5m`         | Max connection idle time                                  |

## DBTX Interface

The package exports a `DBTX` interface matching what sqlc generates. Both `*sql.DB` and `*sql.Tx` satisfy it:

```go
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
```
