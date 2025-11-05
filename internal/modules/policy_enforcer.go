package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shirou/gopsutil/process"

	"sentinel-agent/internal/bus"
	"sentinel-agent/internal/config"
	"sentinel-agent/internal/events"
	"sentinel-agent/internal/gateway"
	"sentinel-agent/internal/logging"
	"sentinel-agent/internal/policy"
)

type policyEnforcer struct {
	pstore *policy.DBStore
	engine *policy.Engine
}

func NewPolicyEnforcer(ps *policy.DBStore) Module {
	return &policyEnforcer{
		pstore: ps,
		engine: policy.NewEngine(),
	}
}

func (m *policyEnforcer) Name() string { return "policy_enforcer" }

func (m *policyEnforcer) Start(ctx context.Context, bus *bus.Bus, cfg *config.Config, gc gateway.GatewayClient, log *logging.Logger) error {
	// Initial Load
	if policies, err := m.pstore.List(); err == nil {
		m.engine.Load(policies)
		log.Info("policy engine loaded", "count", len(policies))
	}

	ch := bus.Subscribe()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case e := <-ch:
				if e.Type == "process_started" {
					m.checkProcessStarted(e, bus, log)
				} else if e.Type == "device_change" {
					m.checkDeviceChange(e, bus, log)
				} else if e.Type == "policy_updated" {
					// Reload Trigger
					if policies, err := m.pstore.List(); err == nil {
						m.engine.Load(policies)
						log.Info("policy engine reloaded")
					}
				}
			}
		}
	}()
	return nil
}

func (m *policyEnforcer) checkProcessStarted(e events.Event, bus *bus.Bus, log *logging.Logger) {
	// Parse Payload
	// Payload: { "event": "started", "process": { "pid": 123, "name": "notepad.exe" } }
	var data struct {
		Process map[string]any `json:"process"`
	}
	if err := json.Unmarshal([]byte(e.Payload), &data); err != nil {
		log.Error("enforcer: failed to parse process started", "err", err)
		return
	}
	name, _ := data.Process["name"].(string)
	pid, _ := data.Process["pid"].(float64)

	// O(1) Check via Engine
	if blocked, ruleID, ruleType := m.engine.CheckProcess(name); blocked {
		now := time.Now().UTC()
		payload, _ := json.Marshal(map[string]any{
			"rule_id": ruleID,
			"rule_type": ruleType,
			"process": map[string]any{"name": name, "pid": int(pid)},
		})
		bus.Publish(events.Event{Timestamp: now, Type: "policy_violation", Payload: string(payload)})
		log.Info("POLICY VIOLATION DETECTED", "process", name, "rule", ruleID, "type", ruleType)

		go func(pid32 int32, rt string) {
			// Whitelist Self-Protection Safeguard
			if pid32 == int32(os.Getpid()) {
				return // Never suspend the active agent itself
			}
			lowerName := strings.ToLower(name)
			if strings.Contains(lowerName, "go.exe") || strings.Contains(lowerName, "agent.exe") || strings.Contains(lowerName, "gateway.exe") || strings.Contains(lowerName, "cmd.exe") || strings.Contains(lowerName, "powershell.exe") {
				return // Protect the active development terminal environments
			}

			proc, err := process.NewProcess(pid32)
			if err != nil {
				return
			}
			
			if rt == "suspend_process" || rt == "block_process" {
				if err := proc.Suspend(); err == nil {
					m.engine.TrackFrozen(name, pid32)
					log.Info("ACTIVE RESPONSE: Frozen (Suspended)", "pid", pid32)
				}
			} else if rt == "quarantine_process" {
				// Snapshot logic
				exePath, err := proc.Exe()
				if err == nil && exePath != "" {
					_ = os.MkdirAll("quarantine", 0755)
					dest := fmt.Sprintf("quarantine/malware_snapshot_%d.bin", pid32)
					
					input, err := os.ReadFile(exePath)
					if err == nil {
						_ = os.WriteFile(dest, input, 0644)
						log.Info("ACTIVE RESPONSE: Malware Snapshot Preserved", "path", dest)
					}
				}
				
				// Hard Kill
				if err := proc.Kill(); err == nil {
					log.Info("ACTIVE RESPONSE: Threat Eliminated (Killed)", "pid", pid32)
				}
			}
		}(int32(pid), ruleType)
	}
}

func (m *policyEnforcer) checkDeviceChange(e events.Event, bus *bus.Bus, log *logging.Logger) {
	// Simple Check via Engine
	if detected, ruleID := m.engine.CheckUSB(); detected {
		now := time.Now().UTC()
		payload, _ := json.Marshal(map[string]any{
			"rule_id":      ruleID,
			"description":  "Unauthorized Device Detected",
			"source_event": e.Payload,
		})
		bus.Publish(events.Event{Timestamp: now, Type: "policy_violation", Payload: string(payload)})
	}
}
