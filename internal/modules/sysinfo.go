package modules

import (
	"context"
	"encoding/json"
	"time"

	cpu "github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"

	"sentinel-agent/internal/config"
	"sentinel-agent/internal/events"
	"sentinel-agent/internal/gateway"
	"sentinel-agent/internal/logging"
)

type sysinfoModule struct{}

func NewSysInfoModule() Module { return &sysinfoModule{} }

func (m *sysinfoModule) Name() string { return "sysinfo" }

func (m *sysinfoModule) Run(ctx context.Context, cfg *config.Config, store events.EventStore, gc gateway.GatewayClient, log *logging.Logger) ([]events.Event, error) {
	h, _ := host.Info()
	c, _ := cpu.Counts(true)
	mems, _ := mem.VirtualMemory()
	data := map[string]any{
		"host":      h.Hostname,
		"os":        h.Platform + " " + h.PlatformVersion,
		"uptime":    h.Uptime,
		"cpu_cores": c,
		"total_mem": mems.Total,
	}
	b, _ := json.Marshal(data)
	evt := events.Event{
		Timestamp: time.Now().UTC(),
		Type:      "sysinfo",
		Payload:   string(b),
	}
	return []events.Event{evt}, nil
}
