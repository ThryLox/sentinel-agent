package ai

import (
	"log"
	"math"
	"sync"
)

// AgentFeatures strict numerical data structure for Anomaly Detection
type AgentFeatures struct {
	ProcessSpawns int
	NetworkCalls  int
	FileMods      int
	LastPayload   string // Tracks context for rule generation
}

// Engine maintains the baseline matrix for all active agents
type Engine struct {
	mu        sync.RWMutex
	baselines map[string]*AgentFeatures // agentID -> rolling features
}

func NewEngine() *Engine {
	return &Engine{
		baselines: make(map[string]*AgentFeatures),
	}
}

// ProcessTelemetry digests raw events and returns an anomaly score [0.0 - 1.0]
func (e *Engine) ProcessTelemetry(agentID string, evts []struct{ Type, Payload string }) float64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	features, exists := e.baselines[agentID]
	if !exists {
		features = &AgentFeatures{}
		e.baselines[agentID] = features
	}

	var newProcs, newNets, newFiles int

	for _, evt := range evts {
		switch evt.Type {
		case "process_launch":
			newProcs++
			features.LastPayload = evt.Payload // e.g. "unknown_crypto.exe"
		case "network_connect":
			newNets++
		case "file_read", "file_write":
			newFiles++
		// If the agent sends raw "process" we map it loosely
		case "process":
			newProcs++
			features.LastPayload = evt.Payload
		}
	}

	features.ProcessSpawns += newProcs
	features.NetworkCalls += newNets
	features.FileMods += newFiles
	
	// Active Anomaly Scoring (Edge-Optimized logic substituting full IF model)
	score := e.calculateAnomalyScore(newProcs, newNets, newFiles)
	
	if score > 0.85 {
		log.Printf("[AI ENGINE] HIGH ANOMALY on %s -> Score: %.2f | Context Trigger: %s", agentID, score, features.LastPayload)
	}

	return score
}

func (e *Engine) calculateAnomalyScore(procs, nets, files int) float64 {
	score := 0.0
	// Heuristic: E.g., Ransomware scripts opening 50 processes simultaneously
	if procs > 50 {
		score += 0.4
	}
	// Heuristic: Massive, rapid file modifications (encryption phase)
	if files > 100 {
		score += 0.5
	}
	// Heuristic: Sudden network scan burst
	if nets > 20 {
		score += 0.3
	}
	return math.Min(score, 1.0)
}

func (e *Engine) GetLastPayload(agentID string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if feat, exists := e.baselines[agentID]; exists {
		if feat.LastPayload != "" {
			return feat.LastPayload
		}
	}
	return "unknown.exe"
}
