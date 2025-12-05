package modules

import (
	"context"
	"encoding/json"
	"time"

	proc "github.com/shirou/gopsutil/process"

	"sentinel-agent/internal/bus"
	"sentinel-agent/internal/config"
	"sentinel-agent/internal/events"
	"sentinel-agent/internal/gateway"
	"sentinel-agent/internal/logging"
)

type ProcessFetcher interface {
	Processes() ([]*proc.Process, error)
}

type realFetcher struct{}

func (r *realFetcher) Processes() ([]*proc.Process, error) {
	return proc.Processes()
}

type processModule struct {
	fetcher ProcessFetcher
}

func NewProcessModule() Module {
	return &processModule{fetcher: &realFetcher{}}
}

// NewProcessModuleWithFetcher allows injecting a mock fetcher for testing
func NewProcessModuleWithFetcher(f ProcessFetcher) Module {
	return &processModule{fetcher: f}
}

func (m *processModule) Name() string { return "process" }

func (m *processModule) Start(ctx context.Context, bus *bus.Bus, cfg *config.Config, gc gateway.GatewayClient, log *logging.Logger) error {
	// State for diffing
	known := make(map[int32]string) // pid -> name

	run := func() {
		procs, err := m.fetcher.Processes()
		// fmt.Printf("DEBUG: Fetcher returned %d processes\n", len(procs))
		if err != nil {
			log.Error("process fetch failed", "err", err)
			return
		}

		current := make(map[int32]string)
		var newProcs []*proc.Process

		for _, p := range procs {
			pid := p.Pid
			name, _ := p.Name() // ignore err, best effort
			current[pid] = name

			if _, exists := known[pid]; !exists {
				newProcs = append(newProcs, p)
			}
		}

		// First run: treat EVERYTHING as new to enforce policies on already running apps
		if len(known) == 0 {
			// Deep copy current to known is handled after loop,
			// but we need to flag these as newProcs to trigger alerts.
			if len(newProcs) == 0 {
				// if loop populated newProcs, we are good.
				// The logic above `if !exists` handles it because `known` is empty.
				// So newProcs ALREADY contains everything.
				// The previous code had `return` here, skipping the event generation loop.
				// We simply REMOVE the early return.
			}
		}

		// Detect attributes for new processes and fire events
		// We do this AFTER the loop to minimize blocking within the fetch loop
		now := time.Now().UTC()
		for _, p := range newProcs {
			name, _ := p.Name()
			exe, _ := p.Exe()
			cmd, _ := p.Cmdline()

			payload, _ := json.Marshal(map[string]any{
				"event": "started",
				"process": map[string]any{
					"pid":     p.Pid,
					"name":    name,
					"exe":     exe,
					"cmdline": cmd,
				},
			})
			bus.Publish(events.Event{
				Timestamp: now,
				Type:      "process_started",
				Payload:   string(payload),
			})
		}

		// Update state
		known = current
	}

	go func() {
		// Initial populate
		run()

		interval := time.Duration(cfg.PollIntervalSeconds) * time.Second
		if cfg.PollIntervalSeconds == 0 {
			interval = 1 * time.Second
		}
		// Hack for testing: if negative, use millis
		if cfg.PollIntervalSeconds < 0 {
			interval = time.Duration(-cfg.PollIntervalSeconds) * time.Millisecond
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
	return nil
}
