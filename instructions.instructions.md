---
applyTo: '**'
---
# Sentinel Agent – MVP Architecture

Goal: Build a lightweight Windows endpoint security agent in Go that:

- Runs as a **Windows Service** (later also Linux service).
- Ships as a **single self-contained .exe**.
- Uses a simple **TOML config** stored in `%PROGRAMDATA%\SentinelAgent\config.toml`.
- Collects basic **telemetry**:
  - System info (OS, hostname, CPU/RAM)
  - Process list (name, PID, path)
- Caches **policy** and **events** locally in SQLite.
- Communicates with a **remote gateway** over HTTPS (MVP: stubbed or simple REST).
- Is **modular** so features can be added/removed later without rewriting the core.
- Is **offline-first**: keeps working and buffering logs if the gateway is unreachable.

MVP focus: stable architecture + basic telemetry + service lifecycle.
No kernel drivers, no firewall modification, no USB hooks yet.


## Tech Stack

- Language: Go (1.22+)
- Target OS (MVP): Windows 10/11 (amd64)
- Service framework: `github.com/kardianos/service`
- Config format: TOML via `github.com/BurntSushi/toml`
- Telemetry: `github.com/shirou/gopsutil/v4`
- Local storage:
  - SQLite via `modernc.org/sqlite` (pure Go, no external DLLs)
- HTTP client: Go `net/http` (REST-style; later can be swapped to gRPC)
- Logging: Go `log/slog`

## Project Structure

sentinel-agent/
  cmd/
    agent/
      main.go          # entrypoint: CLI flags, service wiring

  internal/
    config/
      config.go        # load/create TOML config, default paths
    logging/
      logger.go        # slog setup (file + level)
    service/
      service.go       # integrates with kardianos/service, main loop
      scheduler.go     # (optional) simple periodic task scheduler

    policy/
      policy.go        # Policy struct + in-memory/cache API

    events/
      events.go        # Event type + interface for event storage
      sqlite_store.go  # SQLite-backed implementation of event store

    modules/
      modules.go       # module interfaces + registry
      sysinfo.go       # sysinfo module: basic system info
      process.go       # process module: list processes + simple checks

    gateway/
      gateway.go       # GatewayClient interface + HTTP implementation (stub ok for MVP)
