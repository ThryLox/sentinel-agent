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
	"sentinel-agent/internal/policy"
)

type policyEnforcer struct {
	pstore *policy.DBStore
}

func NewPolicyEnforcer(ps *policy.DBStore) Module { return &policyEnforcer{pstore: ps} }

func (m *policyEnforcer) Name() string { return "policy_enforcer" }

func (m *policyEnforcer) Run(ctx context.Context, cfg *config.Config, store events.EventStore, gc gateway.GatewayClient, log *logging.Logger) ([]events.Event, error) {
	p := m.pstore.Get()
	if p == nil || p.Raw == "" {
		return nil, nil
	}
	var doc struct {
		Version int              `json:"version"`
		Rules   []map[string]any `json:"rules"`
	}
	if err := json.Unmarshal([]byte(p.Raw), &doc); err != nil {
		log.Error("policy parse failed", "err", err)
		return nil, nil
	}
	procs, err := proc.Processes()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	evts := []events.Event{}
	for _, r := range doc.Rules {
		if rt, ok := r["type"].(string); ok && rt == "block_process" {
			match, _ := r["match"].(string)
			rid, _ := r["id"].(string)
			for _, pr := range procs {
				name, _ := pr.Name()
				if name == match {
					payload, _ := json.Marshal(map[string]any{
						"policy_id": p.ID,
						"rule_id":   rid,
						"process":   map[string]any{"name": name, "pid": pr.Pid},
					})
					evts = append(evts, events.Event{Timestamp: now, Type: "policy_violation", Payload: string(payload)})
				}
			}
		}
	}
	return evts, nil
}
