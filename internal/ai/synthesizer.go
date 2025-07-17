package ai

import (
	"encoding/json"
	"fmt"
	"time"
)

// SynthesizeSuspendPolicy creates a rule to freeze the payload in RAM
func SynthesizeSuspendPolicy(agentID string, targetPayload string) ([]byte, error) {
	ruleID := fmt.Sprintf("auto-suspend-%d", time.Now().Unix())
	policyMap := map[string]any{
		"version": 1,
		"policies": []map[string]any{
			{
				"id":   fmt.Sprintf("herd-immunity-%s", agentID),
				"name": "AI Auto-Generated Suspension",
				"rules": []map[string]any{
					{
						"id":     ruleID,
						"type":   "suspend_process",
						"match":  targetPayload, 
						"action": "enforce",
					},
				},
			},
		},
	}
	return json.Marshal(policyMap)
}

// SynthesizeQuarantinePolicy generates the explicit Snapshot and Kill rule
func SynthesizeQuarantinePolicy(agentID string, targetPayload string) ([]byte, error) {
	ruleID := fmt.Sprintf("auto-kill-%d", time.Now().Unix())
	policyMap := map[string]any{
		"version": 1,
		"policies": []map[string]any{
			{
				"id":   fmt.Sprintf("herd-immunity-%s", agentID),
				"name": "AI Auto-Generated Enforcement",
				"rules": []map[string]any{
					{
						"id":     ruleID,
						"type":   "quarantine_process",
						"match":  targetPayload, 
						"action": "enforce",
					},
				},
			},
		},
	}
	return json.Marshal(policyMap)
}

// SynthesizeResumePolicy creates a rule telling agents to unfreeze a falsely flagged payload
func SynthesizeResumePolicy(agentID string, targetPayload string) ([]byte, error) {
	ruleID := fmt.Sprintf("auto-resume-%d", time.Now().Unix())
	policyMap := map[string]any{
		"version": 1,
		"policies": []map[string]any{
			{
				"id":   fmt.Sprintf("herd-immunity-%s", agentID),
				"name": "AI False-Positive Reversal",
				"rules": []map[string]any{
					{
						"id":     ruleID,
						"type":   "resume_process",
						"match":  targetPayload, 
						"action": "enforce",
					},
				},
			},
		},
	}
	return json.Marshal(policyMap)
}
