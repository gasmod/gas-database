# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] - 2026-07-02

First open source release. Versions prior to 0.3.0 were developed in a private
repository; this entry summarizes the module as published.

### Added

- **Dual backend support** — `database.ModeSQL` (default) wraps `database/sql`
  for any driver (PostgreSQL, SQLite, MySQL, etc.), while `database.ModePgx`
  creates a native `pgxpool.Pool` for PostgreSQL. `DB()` always returns a
  `*sql.DB` in both modes (via the pgx stdlib adapter in `ModePgx`), so sqlc's
  `database/sql` mode always works; `Pool()` additionally exposes the native
  `*pgxpool.Pool` for sqlc's pgx mode.
- **`gas.DatabaseProvider` implementation** for DI-based injection into
  consuming services, plus `gas.HealthReporter` / `gas.ReadyReporter` for
  health and readiness probes — `CheckHealth` reports liveness without
  pinging the database (both backends auto-reconnect), while `CheckReady`
  pings the database to signal when traffic should drain.
- **Transaction helpers** — `BeginTx` for manual transaction management and
  `WithTx` for automatic commit/rollback, including rollback on panic.
- **`DBTX` interface** matching the shape sqlc generates, satisfied by both
  `*sql.DB` and `*sql.Tx`.
- **Connector support** — `WithConnector` accepts a `driver.Connector`
  directly (`sql.OpenDB`) for custom connection setup such as TLS or auth
  tokens, bypassing the `Driver`/`DSN` config fields.
- **Connection retry with exponential backoff** — `ConnRetries` and
  `ConnRetryInterval` config fields control retry attempts on initial
  connection failure.
- **Configuration binding** — automatic binding from the injected
  `gas.ConfigProvider` when `WithConfig` is not supplied, with validated
  settings for mode, driver, DSN, pool sizing (`MaxOpenConns`,
  `MaxIdleConns`), connection lifetime (`ConnMaxLifetime`,
  `ConnMaxIdleTime`), and retry behavior.
- **`Driver()`** accessor reporting the effective database driver name based
  on configured mode and settings.

[Unreleased]: https://github.com/gasmod/gas-database/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/gasmod/gas-database/releases/tag/v0.3.0

