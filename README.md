<!-- prettier README for GitHub -->
# Sentinel Agent — v1.0

A lightweight, modular Windows endpoint agent (v1.0). Built in Go, this MVP focuses on offline-first telemetry collection, local persistence, and a safe policy enforcement loop.

Quick links

- Source: repository root
- Service entry: `cmd/agent/main.go`
- Core orchestrator: `internal/service/service.go`

Why this project

This agent is designed to be small, auditable, and easy to extend. It collects host telemetry, persists events locally, and enforces policies delivered as YAML. The architecture separates detection (modules) from persistence and delivery (service), enabling safe, auditable remediation workflows.

Highlights (what v1.0 does today)

- Run as a foreground process or Windows Service (`kardianos/service`).
- Collects host telemetry: `sysinfo` and `process` modules (`internal/modules`).
- Persists events to SQLite at `%PROGRAMDATA%/SentinelAgent/events.db`.
- Stores policies in a `policies` table and enforces `block_process` rules (detect-only by default).
- Periodic policy fetching from a YAML endpoint (configurable) and local YAML loader (`tools/load_policy`).
- Lightweight gateway client that POSTs events to a configured `GatewayURL`.

Quick start (Windows PowerShell)

1. Ensure Go (>=1.22) is installed and on your PATH.

2. From repo root, fetch deps and build:

```powershell
go mod tidy
go build ./...
```

3. (Optional) Load a local policy YAML to test enforcement (see sample below):

```powershell
go run tools/load_policy -f .\policies.yaml
```

4. Run the agent in foreground for debugging:

```powershell
.\cmd\agent\agent.exe -run
```

5. Inspect latest events:

```powershell
go run tools/query_events
```

Config notes

- Config path: `%PROGRAMDATA%/SentinelAgent/config.toml` (created on first run). Important fields:
	- `gateway_url` — where events are POSTed.
	- `poll_interval_seconds` — how often modules run (default 60s).
	- `policy_url` — (optional) YAML policy endpoint to poll.
	- `policy_poll_seconds` — how often to poll policies (default 300s).

Policies (format & flow)

- YAML schema (MVP): top-level `version` and `policies` array. Each policy has `id`, `name`, and `rules` (e.g. `block_process`).
- Loading options:
	- Local: `tools/load_policy` writes YAML policies into the DB (replacing by `id`).
	- Remote: set `policy_url` to enable periodic fetching; fetched policies are validated and upserted by `id`.
- Enforcement: `PolicyEnforcer` reads the active policy and emits `policy_violation` events. These are persisted and sent to the gateway for further scoring/triage.

Alerting & remediation (long-term flow)

1. Agent emits `policy_violation` events to DB and gateway.
2. Gateway (or an AI engine) scores events and may mark some `critical`.
3. For critical cases the gateway updates a policy with an explicit `action: kill` rule and publishes it at the policy endpoint.
4. Agent fetches the updated policy, and—if operator has enabled destructive actions and agent runs with admin privileges—applies remediation (and logs a `policy_remediation` event).

Security & safety (MVP principles)

- Destructive actions are disabled by default. Enable them only with explicit config and admin consent.
- Policies should be delivered over HTTPS and (later) signed/verified.
- All remediation actions must be auditable (events + gateway records).

Developer tools

- `tools/load_policy` — load YAML policies into DB (replaces existing IDs).
- `tools/query_events` — dump recent events as JSON.

Roadmap (near-term)

- [ ] Windows Event Log integration for `policy_violation` events (local alerting).
- [ ] Signed policy delivery and verification.
- [ ] `policy_enforce_actions` config gate + admin-only remediation path.
- [ ] Centralized DB manager (single connection) and better concurrency handling.
- [ ] Unit & integration tests for modules, policy parsing, and enforcement.
- [ ] Packaging: single self-contained exe and installer for Windows.

Files worth opening first

- `cmd/agent/main.go` — service entry & flags
- `internal/service/service.go` — orchestrator, fetcher, run loop
- `internal/modules/` — module implementations
- `internal/events/` and `internal/policy/` — SQLite-backed stores
- `tools/load_policy` and `tools/query_events` — helper CLIs

Support

If you'd like I can:

- Add a sample `policies.yaml` and load it for a demo.
- Implement Windows Event Log logging for violations (low-risk).
- Create end-to-end demo scripts (local policy server → enforcement → remediation simulation).

License

MIT

---
Small, focused, and auditable — v1.0 aims to be a clean foundation for more advanced security automation.

Long-term workflow (how critical remediation will work)

1. Detection: the agent's `PolicyEnforcer` emits `policy_violation` events and the Service persists them to the local DB and posts them to the configured `GatewayURL`.
2. Cloud/AI scoring: the gateway receives the event stream and an AI engine/classifier evaluates each violation and assigns a severity (e.g., `informational`, `warning`, `critical`).
3. Escalation: for violations marked `critical` the gateway prepares an updated policy that includes a rule with `action: kill` for the offending process (or an equivalent remediation instruction). The updated policy is published at the agreed policy endpoint.
4. Policy distribution: the agent periodically polls `policy_url` (or receives push notifications) and upserts policies by `id` into the local policy DB (the agent uses `policy.Policy.ID` as the unique key). If a policy ID already exists the agent replaces it atomically.
5. Safe enforcement: on the next enforcement cycle the `PolicyEnforcer` will read the updated policy. If an explicit, permitted `kill` action appears and the agent is configured to allow destructive actions (explicit config flag + running with admin privileges), the module may perform the remediation (and will emit a final `policy_remediation` event documenting the action). All remediation must be auditable and reversible where possible.

Design notes and safety gates

- Remediation (kill) is gated: by default v1.0 only emits detections. Enabling destructive actions requires:
	- Operator opt-in in `config.toml` (e.g., `policy_enforce_actions = true`).
	- Agent running with appropriate privileges (admin) and under operational controls.
	- Signed policies or authenticated policy delivery so the agent only applies trusted updates.
- Audit trail: every remediation must produce an event saved to the DB and sent to the gateway so the system has a complete audit log.
- Staged rollout: recommend a staged rollout (detect-only → block in sandbox → selective remediation) and monitoring of false positives before enabling kill actions.

Suggested OS notification for v1.0

For immediate local visibility in v1.0 we recommend writing `policy_violation` events to the Windows Event Log. This is low-risk and requires no UI privileges. Advantages:

- Administrators can use Event Viewer, SIEMs, or Windows Event subscriptions to get alerted.
- Works when the agent runs as a background service.

Implementation notes (how to):

- Use the Windows Event Log API via `golang.org/x/sys/windows/svc/eventlog` or a small wrapper. On policy violation, write an event with a clear message and structured JSON in the event detail.
- Optionally, for interactive desktops you can add a separate non-service notifier using `github.com/go-toast/toast` to show a Windows toast, but this should be separate from the service to avoid session/permission issues.

To-do (near-term and next milestones)

- [ ] Add Windows Event Log logging for `policy_violation` events (low-risk local alerting).
- [ ] Implement signed policy payloads and verification before applying updates.
- [ ] Add `policy_enforce_actions` config flag and safe path to enable destructive actions only with admin consent and audit logging.
- [ ] Centralize DB access behind a single connection manager to avoid independent SQLite connections.
- [ ] Add unit and integration tests for policy parsing, enforcement logic, and the load/replace-by-id behavior.
- [ ] Add a demo mode (local policy server) and scripts to run end-to-end tests (detection → gateway scoring → policy push → remediation).

If you'd like I can start by implementing the Windows Event Log write for `policy_violation` events (non-destructive) as the next small change. Reply with 'yes' and I'll add the code, tests, and a short README entry describing how to view events in Event Viewer.
