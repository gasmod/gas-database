# gas-database

Database connection module for the [Gas](https://github.com/gasmod/gas) ecosystem. Wraps `database/sql` and native
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
	dbMod := database.New(database.WithConfig(&database.Config{
		DatabaseDSN:    "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
		DatabaseDriver: "pgx",
	}))

	app := gas.NewApp(
		gas.WithModule(dbMod),
		// ...
	)

	app.Run()
}
```

### Native pgx mode

```go
dbMod := database.New(database.WithConfig(&database.Config{
	DatabaseMode: database.ModePgx,
	DatabaseDSN:  "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
}))

// After Init(), both are available:
// dbMod.DB()   -> *sql.DB (via stdlib adapter)
// dbMod.Pool() -> *pgxpool.Pool
```

### Using a connector (sql.OpenDB)

When you need full control over connection setup (e.g., custom TLS, auth tokens), pass a `driver.Connector` directly:

```go
import "github.com/jackc/pgx/v5/stdlib"

connConfig, _ := pgx.ParseConfig("postgres://user:pass@localhost:5432/mydb")
connector := stdlib.GetConnector(*connConfig)

dbMod := database.New(
	database.WithConnector(connector),
)
```

When a connector is provided, `DatabaseDriver` and `DatabaseDSN` are not required.

### SQLite

```go
import _ "modernc.org/sqlite"

dbMod := database.New(database.WithConfig(database.Config{
	DatabaseDriver: "sqlite",
	DatabaseDSN:    "./app.db",
}))
```

### Passing to other modules

Modules receive the database through `gas.DatabaseProvider`:

```go
// In main.go
authMod := auth.New(
	auth.WithDatabaseProvider(dbMod), // Module implements gas.DatabaseProvider
)
```

Inside a consuming module, use `DB()` for sqlc-generated queries:

```go
// gas-auth/module.go
func (m *Module) Init() error {
	m.queries = authdb.New(m.db.DB()) // sqlc database/sql mode
	return nil
}
```

For modules that want native pgx access, define a local interface and type-assert:

```go
// gas-auth/providers.go
type PgxProvider interface {
	Pool() *pgxpool.Pool
}

// gas-auth/module.go
func (m *Module) Init() error {
	if pp, ok := m.db.(PgxProvider); ok && pp.Pool() != nil {
		m.queries = authdb.New(pp.Pool()) // sqlc pgx mode
	} else {
		m.queries = authdb.New(m.db.DB()) // fallback to database/sql
	}
	return nil
}
```

### Transactions

Manual transaction management:

```go
tx, err := dbMod.BeginTx(ctx, nil)
if err != nil {
	return err
}
// use tx with sqlc: queries.WithTx(tx)
err = tx.Commit()
```

Automatic commit/rollback with `WithTx`:

```go
err := dbMod.WithTx(ctx, nil, func(tx *sql.Tx) error {
	qtx := queries.WithTx(tx)
	if err := qtx.CreateUser(ctx, params); err != nil {
		return err // triggers rollback
	}
	return qtx.CreateProfile(ctx, params) // commits if nil
})
```

`WithTx` also rolls back on panic.

## Config

| Field                     | Default      | Description                                               |
|---------------------------|--------------|-----------------------------------------------------------|
| `DatabaseMode`            | `"sql"`      | Backend mode: `"sql"` or `"pgx"`                          |
| `DatabaseDriver`          | `"postgres"` | `database/sql` driver name (ModeSQL only)                 |
| `DatabaseDSN`             |              | Connection string (required unless using `WithConnector`) |
| `DatabaseMaxOpenConns`    | `25`         | Max open connections                                      |
| `DatabaseMaxIdleConns`    | `5`          | Max idle connections (ModeSQL only)                       |
| `DatabaseConnMaxLifetime` | `30m`        | Max connection reuse time                                 |
| `DatabaseConnMaxIdleTime` | `5m`         | Max connection idle time                                  |

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
