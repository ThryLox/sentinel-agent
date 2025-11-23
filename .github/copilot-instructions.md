## Goal

Lightweight Windows endpoint agent (MVP): run as a Windows Service, ship as a single self-contained exe, store a TOML config under `%PROGRAMDATA%\SentinelAgent`, cache events/policy in SQLite, collect basic telemetry (sysinfo + process list), and send events to a remote gateway (MVP: simple REST). The codebase emphasizes a modular, offline-first architecture.

## Quick orientation

This repo is a small Go-based Windows endpoint agent (MVP). Key responsibilities:

- `cmd/agent/main.go` — service entrypoint; installs/runs the Windows service (kardianos/service) and supports `-run`, `-install`, `-uninstall` for local testing.
- `internal/config/config.go` — loads/writes a TOML config in `%ProgramData%/SentinelAgent/config.toml`. Defaults are applied automatically.
- `internal/service/service.go` — core orchestration: initializes the event store, creates a gateway client, runs registered modules on a ticker, persists events, and sends them to the gateway.
- `internal/modules/` — module registry and modules implementing `Module` (see `NewSysInfoModule`, `NewProcessModule`). Modules produce `events.Event` values — they should not persist or send events themselves; the service handles that.
- `internal/events/` — the `Event` struct and `EventStore` interface (`Save`, `List`, `Close`).
- `internal/gateway/` — `GatewayClient` (`SendEvents`) and an HTTP implementation used by the service.
- `internal/logging/logger.go` — centralized logger. Writes to stdout and `ProgramData/SentinelAgent/agent.log` when available.

## Tech stack (useful references)

- Go >=1.22
- Service framework: `github.com/kardianos/service`
- Config/TOML: `github.com/BurntSushi/toml`
- Telemetry: `github.com/shirou/gopsutil/v4`
- SQLite: `modernc.org/sqlite` (pure-Go driver)
- Logging: `log/slog`

## Project structure (open these files first)

- `cmd/agent/main.go` — entrypoint + service wiring
- `internal/config/config.go` — config load/create + default paths
- `internal/service/service.go` — orchestration, run loop, persistence & sending
- `internal/modules/modules.go` and `internal/modules/*.go` — module registry and implementations
- `internal/events/events.go` — event types and store interface
- `internal/gateway/gateway.go` — HTTP gateway behavior
- `internal/logging/logger.go` — slog wrapper writing to ProgramData

## Big-picture architecture & flow

1. Agent starts in `cmd/agent/main.go` and loads config via `config.Load()`.
2. `service.New(cfg, logger)` builds a `Service` with a `modules.Registry` (built-in modules are registered in `New`).
3. `Service.Run()` opens an `events.EventStore` (SQLite), creates a `gateway.GatewayClient`, runs `runOnce()` immediately and then repeatedly on a ticker.
4. Each module's `Run(ctx, cfg, store, gc, log)` returns zero or more `events.Event`. `Service` persists those via `EventStore.Save` and then calls `GatewayClient.SendEvents` to deliver them.

Why this matters for code changes:
- Modules only produce events; do not duplicate persistence/sending logic.
- To add behavior, implement a `Module` and register it in `internal/service.New` (or via the `modules.Registry`).

## Conventions and patterns (project-specific)

- Module contract: implement the `Module` interface from `internal/modules/modules.go`:
  - `Name() string`
  - `Run(ctx context.Context, cfg *config.Config, store events.EventStore, gc gateway.GatewayClient, log *logging.Logger) ([]events.Event, error)`
  - Keep `Run` short-lived and cancellable via the passed `ctx`. Avoid long blocking calls without respecting `ctx.Done()`.
- Event ownership: modules return `events.Event` values (with `Type`, `Timestamp`, `Payload`). The `Service` is responsible for `Save` and `SendEvents`.
- Config and file locations:
  - Config: `%ProgramData%/SentinelAgent/config.toml` (created with defaults on first load).
  - DB: default at `%ProgramData%/SentinelAgent/events.db` (configured via `Config.DBPath`).
  - Logs: `agent.log` in the same ProgramData folder.
- MVP error-handling: many functions log errors and continue. When adding features, prefer returning clear errors and adding targeted retries in `gateway` or higher-level orchestration.

## Developer workflows (how to build/run/debug)

- Build locally (PowerShell):

  go mod tidy
  go build ./...

- Run in foreground (useful for debugging):

  .\cmd\agent\agent.exe -run

- Install/uninstall as Windows service (kardianos/service handles the mechanics):

  .\cmd\agent\agent.exe -install
  .\cmd\agent\agent.exe -uninstall

Notes: The config file path and DB path are created under ProgramData when `config.Load()` runs, so run as a user with permission to create `%ProgramData%/SentinelAgent` when testing.

## Integration points & external dependencies

- HTTP gateway: `internal/gateway.NewHTTPClient(url)` implements `GatewayClient` and does a simple JSON POST. See `internal/gateway/gateway.go`.
- SQLite store: `internal/events/sqlite_store.go` (uses `modernc.org/sqlite`) — the `EventStore` interface is intentionally small.
- System info and processes: modules depend on `github.com/shirou/gopsutil/v4`.

## How to add a new module (concrete checklist)

1. Create `internal/modules/<yourmod>.go` with a type that implements `Module`.
2. Fill `Run` to return `[]events.Event` (marshal complex payloads to JSON in `Payload`). Keep payload sizes reasonable (see `process` module's 200 item cap).
3. Register the module in `internal/service/service.go` (or add to registry initialization): `s.mods.Register(modules.NewYourModule())`.
4. Unit-test the module logic in isolation. Tests should exercise `Run` and validate returned `events.Event` values.

## Quick references & examples

- Process module caps its reported list to 200 items to avoid large payloads: `internal/modules/process.go`.
- Config defaults and auto-creation occur in `internal/config/config.go` (creates `%ProgramData%/SentinelAgent/config.toml` on first `Load`).

If anything above is unclear or you'd like a shorter or longer version (examples for new module, a test skeleton, or contributor checklist), tell me which section to expand and I will iterate.
