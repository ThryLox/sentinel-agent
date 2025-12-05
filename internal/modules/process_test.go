package modules

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"sentinel-agent/internal/bus"
	"sentinel-agent/internal/config"
	"sentinel-agent/internal/logging"

	// We need to import proc to use the type in the interface,
	// but we don't call real system functions in the mock if we can help it.
	proc "github.com/shirou/gopsutil/process"
)

// dynamicMockFetcher
type dynamicMockFetcher struct {
	procs []*proc.Process
}

func (m *dynamicMockFetcher) Processes() ([]*proc.Process, error) {
	return m.procs, nil
}

func TestProcessModule_DiffEngine(t *testing.T) {
	// Setup
	fetcher := &dynamicMockFetcher{
		procs: []*proc.Process{{Pid: 100}},
	}
	mod := NewProcessModuleWithFetcher(fetcher)
	b := bus.New()
	ch := b.Subscribe()

	// Use negative value for millis hack: -100 = 100ms
	cfg := &config.Config{PollIntervalSeconds: -100}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start (Initial run populates cache, sends NOTHING because known is populated from current)
	if err := mod.Start(ctx, b, cfg, nil, logging.New(nil)); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for initial run to settle (50ms < 100ms ticker)
	time.Sleep(50 * time.Millisecond)

	// Update Mock: Add new process
	fetcher.procs = append(fetcher.procs, &proc.Process{Pid: 101})

	// Wait for NEXT ticker (approx 500ms > 100ms) - Increased wait time
	time.Sleep(500 * time.Millisecond)

	// Check Bus
	select {
	case e := <-ch:
		if e.Type != "process_started" {
			t.Errorf("expected process_started, got %s", e.Type)
		}
		// Validate payload
		var data struct {
			Event   string         `json:"event"`
			Process map[string]any `json:"process"`
		}
		if err := json.Unmarshal([]byte(e.Payload), &data); err != nil {
			t.Fatalf("bad payload: %v", err)
		}
		if pid, ok := data.Process["pid"].(float64); !ok || int(pid) != 101 {
			t.Errorf("expected pid 101, got %v", data.Process["pid"])
		}
	default:
		t.Fatal("expected event, got none")
	}
}
