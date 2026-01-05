package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/process"
)

type RuleData struct {
	ID   string
	Type string
}

// Engine holds compiled rules in memory for fast evaluation.
type Engine struct {
	mu           sync.RWMutex
	blockedProcs map[string]RuleData // process_name -> rule data
	usbRules     []map[string]string // list of usb rules (id, etc)
	isolated     bool                // Is the agent under Active Response Soft Isolation?
	frozenPIDs   map[string][]int32  // name -> []PIDs (Track for hitl resume)
}

func NewEngine() *Engine {
	return &Engine{
		blockedProcs: make(map[string]RuleData),
		frozenPIDs:   make(map[string][]int32),
	}
}

// TrackFrozen safely logs PIDs that have been soft-isolated so they can be resumed.
func (e *Engine) TrackFrozen(name string, pid int32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := strings.ToLower(name)
	e.frozenPIDs[n] = append(e.frozenPIDs[n], pid)
}

// Load compiles a list of raw policies into memory.
func (e *Engine) Load(policies []*Policy) error {
	blocked := make(map[string]RuleData)
	var usb []map[string]string

	for _, p := range policies {
		if p.Raw == "" {
			continue
		}
		var doc struct {
			Rules []map[string]any `json:"rules"`
		}
		if err := json.Unmarshal([]byte(p.Raw), &doc); err != nil {
			continue
		}

		for _, r := range doc.Rules {
			rt, _ := r["type"].(string)
			rid, _ := r["id"].(string)

			if rt == "suspend_process" || rt == "block_process" {
				match, _ := r["match"].(string)
				if match != "" {
					blocked[strings.ToLower(match)] = RuleData{ID: rid, Type: rt}
				}
			} else if rt == "quarantine_process" {
				match, _ := r["match"].(string)
				if match != "" {
					key := strings.ToLower(match)
					blocked[key] = RuleData{ID: rid, Type: rt}
					
					// Snapshot and Kill existing actively frozen apps
					for _, pid := range e.frozenPIDs[key] {
						if pr, err := process.NewProcess(pid); err == nil {
							exePath, _ := pr.Exe()
							if exePath != "" {
								_ = os.MkdirAll("quarantine", 0755)
								dest := fmt.Sprintf("quarantine/malware_snapshot_%d.bin", pid)
								if input, err := os.ReadFile(exePath); err == nil {
									_ = os.WriteFile(dest, input, 0644)
								}
							}
							_ = pr.Kill()
						}
					}
					delete(e.frozenPIDs, key)
				}
			} else if rt == "resume_process" {
				match, _ := r["match"].(string)
				if match != "" {
					key := strings.ToLower(match)
					delete(blocked, key)
					
					// Thaw frozen app safely
					for _, pid := range e.frozenPIDs[key] {
						if pr, err := process.NewProcess(pid); err == nil {
							_ = pr.Resume()
						}
					}
					delete(e.frozenPIDs, key)
				}
			} else if rt == "detect_usb" {
				usb = append(usb, map[string]string{"id": rid, "policy_id": p.ID})
			} else if rt == "isolate_network" {
				e.isolated = true
			}
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.blockedProcs = blocked
	e.usbRules = usb
	return nil
}

// IsIsolated safely dictates if the host is under lockdown
func (e *Engine) IsIsolated() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.isolated
}

// CheckProcess returns true, rule_id, and rule_type if the process matches a policy.
func (e *Engine) CheckProcess(name string) (bool, string, string) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	if e.isolated {
		return true, "global-isolation", "suspend_process"
	}

	meta, exists := e.blockedProcs[strings.ToLower(name)]
	if exists {
		return true, meta.ID, meta.Type
	}
	return false, "", ""
}

// CheckUSB returns true and the first matching rule_id if USB detection is enabled.
func (e *Engine) CheckUSB() (bool, string) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.usbRules) > 0 {
		return true, e.usbRules[0]["id"]
	}
	return false, ""
}
