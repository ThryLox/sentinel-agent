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

## v1.1 — Next minor release (TODOs & expected changes)

This section lists concise, actionable items for the v1.1 milestone and the changes you can expect. Keep each item small and testable; mark items done as you complete them.

### v1.1 — network policy discovery & client/server workflow (next steps)

Short summary: detect a local policy server on the LAN, fetch signed policies, persist them into the policy store, and enforce them on the client with robust auditing and safe rollbacks.

Discovery (how the agent finds a local policy server)
- UDP broadcast or multicast (simple MVP): send a small discovery packet on a configurable port; server replies with its base URL and capability hints (supports /policies, /health, signing metadata).
- mDNS/Avahi (optional): advertise/resolve a service name (e.g., _sentinel-policy._tcp.local) for environments that support mDNS.
- Static fallback: `Config.PolicyServerURL` if discovery fails.

Policy transport & verification
- REST endpoints: `/policies/latest` (returns metadata + checksum), `/policies/{id}` (full payload), `/policies/signature/{id}` (signature blob) — simple JSON over HTTPS.
- Signing: require signed policies. Verify signatures on receipt (cosign/fulcio keyless or GPG detached signatures as configured). Store the signature and verification result with the policy record.
- Integrity: verify checksum in metadata before applying.

Client workflow (high-level)
1. Discovery: periodically or at startup discover a local policy server (fast retry/backoff).
2. Fetch: GET `/policies/latest`; if new, fetch the policy and signature and verify.
3. Persist: upsert policy into `internal/policy/sqlite_store.go` (upsert-by-id), mark provenance and verification status.
4. Enforce: `PolicyEnforcer` module loads active policies from the store and applies them safely (audit events for allowed/blocked decisions). Enforcer should support a dry-run mode and an apply mode.
5. Audit & telemetry: persist `policy_applied`, `policy_rejected`, and `policy_verification_failed` events via the `events.EventStore` so operators can inspect decisions.
6. Rollback & staging: support `staged` policies that require operator approval (or a grace period) before becoming `active`. Provide a `tools/` helper to promote/demote policies.

Server workflow (minimal spec)
- Provide discovery reply (URL + metadata).
- Expose endpoints: `/policies/latest`, `/policies/{id}`, `/policies/signature/{id}`, and `/health`.
- Optional admin GUI or CLI: upload policy YAML, sign with cosign/GPG, and publish (server computes checksum + stores signed blob and signature).

Testing & verification
- Unit tests for policy parsing, upsert, and verification logic.
- Integration test: a small test server that responds to discovery and serves signed policy blobs; agent integration test runs the discovery + fetch + enforce path in an isolated environment.

Expected changes & impact
- New configs: `PolicyDiscoveryPort`, `PolicyServerURL`, `PolicyVerificationMethod` (cosign|gpg|none), `PolicyStaging` (bool), and polling intervals.
- New modules: lightweight discovery client, policy fetcher, and improved `PolicyEnforcer` with staging/dry-run.
- DB: extensions to policy schema to store signature, checksum, provenance, staged/active state.
- Events: new event types for policy lifecycle (fetched, verified, applied, rejected, rollback).

If you'd like, I can implement the discovery client + policy fetcher first (small PR), then wire the enforcer to use persisted policies and add unit tests for verification and upsert behavior.
High-level priorities for v1.1

- Improve release hygiene: remove tracked binaries and workflows from the default branch; add a stable, minimal release workflow or rely on manual releases. (Status: cleanup in progress)
- Policy & enforcement: complete DB-backed policy CRUD and add unit tests + tool integration for safe policy rollouts.
- Packaging & signing: provide a documented manual release process for Windows binaries and optionally add a lightweight, well-tested CI workflow for builds and uploads (deferred until signing is stable).

Concrete TODOs (short tasks)

1. Policy store tests
  - Add unit tests for `internal/policy/sqlite_store.go`: test upsert, get, list, and DB migration behavior.
  - Expected change: fewer runtime surprises when loading policies; small increase in test coverage.

2. Policy enforcer improvements
  - Harden `internal/modules/policy_enforcer.go` to handle edge cases (empty policy, malformed rules). Add logging for rejected/ignored policies.
  - Expected change: clearer runtime logs and fewer silent failures.

3. CLI tools polish
  - Add help text and input validation to `tools/load_policy` and `tools/query_events`. Add a `--dry-run` option to `load_policy` to validate YAML without applying.
  - Expected change: safer operator workflows and reduced accidental policy pushes.

4. Minimal release docs
  - Add `RELEASE.md` with step-by-step manual release instructions (build, zip, create GitHub release, upload asset, attach checksums/signatures). Link to cosign/keyless docs as optional guidance.
  - Expected change: reproducible manual releases and fewer CI surprises.

5. CI/workflow options (deferred, small spike)
  - If reintroducing automation, add a gated workflow that only runs for tagged releases and uses OIDC-based keyless signing (Sigstore) with clear fallbacks. Start with a small matrix (windows/linux) and one well-documented runner step for signing.
  - Expected change: automated releases when you want them; plan to iterate after a successful manual release.

Quick wins (can be done now)

- Re-add `.gitignore` entries for typical artifacts (exes, tmp files). Commit and push. Expected change: fewer accidental binary commits.
- Remove lingering temporary files (release_notes.txt, tmp_notes.txt) from the repo (already done). Expected change: cleaner repository.
- Add a small README section with developer tips for running locally on Windows (run flags, permissions). Expected change: easier onboarding.

What to expect from this release (brief)

- Slightly improved stability around policy handling and safer CLI helpers.
- Documentation for manual releases so you can ship platform binaries consistently.
- More unit tests around the policy subsystem and small logging improvements.

If you want, I can open PRs for each TODO or implement the highest-priority items (policy tests + RELEASE.md) next — tell me which task to start with.
