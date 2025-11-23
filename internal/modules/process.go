package modules

import (
	"context"
	"encoding/json"
	"time"

	proc "github.com/shirou/gopsutil/process"

	"sentinel-agent/internal/config"
	"sentinel-agent/internal/events"
	"sentinel-agent/internal/gateway"
	"sentinel-agent/internal/logging"
)

type processModule struct{}

func NewProcessModule() Module { return &processModule{} }

func (m *processModule) Name() string { return "process" }

func (m *processModule) Run(ctx context.Context, cfg *config.Config, store events.EventStore, gc gateway.GatewayClient, log *logging.Logger) ([]events.Event, error) {
	procs, err := proc.Processes()
	if err != nil {
		return nil, err
	}
	list := make([]map[string]any, 0, len(procs))
	for _, p := range procs {
		name, _ := p.Name()
		pid := p.Pid
		exe, _ := p.Exe()
		list = append(list, map[string]any{"name": name, "pid": pid, "exe": exe})
		if len(list) >= 200 { // cap to avoid huge payloads
			break
		}
	}
	b, _ := json.Marshal(list)
	evt := events.Event{
		Timestamp: time.Now().UTC(),
		Type:      "process_list",
		Payload:   string(b),
	}
	return []events.Event{evt}, nil
}
