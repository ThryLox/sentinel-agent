package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"sentinel-agent/internal/ai"
	"sentinel-agent/internal/db"
	"sentinel-agent/internal/events"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all for MVP LAN
	},
}

var activeAgents = struct {
	mu    sync.Mutex
	conns map[*websocket.Conn]bool
}{
	conns: make(map[*websocket.Conn]bool),
}

type Store struct {
	mu     sync.RWMutex
	Events []events.Event
	Stats  Stats
}

type Stats struct {
	EventCount     int       `json:"event_count"`
	ViolationCount int       `json:"violation_count"`
	AgentLastSeen  time.Time `json:"agent_last_seen"`
}

var store = &Store{}
var aiEngine = ai.NewEngine()
var tsdb = db.NewVictoriaClient("http://localhost:8428")

func main() {
	fs := http.FileServer(http.Dir("./web"))
	http.Handle("/", fs)

	// Legacy API Endpoints
	http.HandleFunc("/api/v1/events", handleEvents)
	
	http.HandleFunc("/api/v1/enforce_kill", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			return
		}
		
		agentID := r.URL.Query().Get("agent")
		payloadBase64 := r.URL.Query().Get("payload")
		if agentID == "" || payloadBase64 == "" {
			http.Error(w, "missing arguments", http.StatusBadRequest)
			return
		}
		
		decodedPayload, err := base64.StdEncoding.DecodeString(payloadBase64)
		if err != nil {
			http.Error(w, "invalid base64", http.StatusBadRequest)
			return
		}
		
		// Push the final Quarantine & Kill rule
		rawPolicy, err := ai.SynthesizeQuarantinePolicy(agentID, string(decodedPayload))
		if err == nil {
			BroadcastPolicy(rawPolicy)
			log.Printf("HITL: Administrator enforced Quarantine/Kill on %s!", string(decodedPayload))
		}
		w.WriteHeader(http.StatusOK)
	})
	
	http.HandleFunc("/api/v1/resume_process", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			return
		}
		
		agentID := r.URL.Query().Get("agent")
		payloadBase64 := r.URL.Query().Get("payload")
		if agentID == "" || payloadBase64 == "" {
			http.Error(w, "missing arguments", http.StatusBadRequest)
			return
		}
		
		decodedPayload, err := base64.StdEncoding.DecodeString(payloadBase64)
		if err != nil {
			http.Error(w, "invalid base64", http.StatusBadRequest)
			return
		}
		
		// Push the Resume (thaw) Rule
		rawPolicy, err := ai.SynthesizeResumePolicy(agentID, string(decodedPayload))
		if err == nil {
			BroadcastPolicy(rawPolicy)
			log.Printf("HITL: Administrator declared False-Positive and resumed %s!", string(decodedPayload))
		}
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/api/v1/metrics", handleMetrics)
	
	// New V3.0 WebSocket Endpoint
	http.HandleFunc("/api/v1/events/stream", handleStream)
	
	// Simulator API to trigger the Gateway AI and Dashboard UI seamlessly
	http.HandleFunc("/api/v1/mock_anomaly", func(w http.ResponseWriter, r *http.Request) {
		// Mock an agent connection context
		agentID := "local-test-agent"
		offendingPayload := "notepad.exe" // standard test dummy

		// 1. Auto-Freeze broadcast
		suspendRule, _ := ai.SynthesizeSuspendPolicy(agentID, offendingPayload)
		BroadcastPolicy(suspendRule)
		
		// 2. Inject Alert into Dashboard
		alertPayload, _ := json.Marshal(map[string]any{
			"rule_id": "suspend_process",
			"agent_id": agentID,
			"process": map[string]any{"name": offendingPayload, "pid": "0"},
		})
		
		store.mu.Lock()
		store.Events = append(store.Events, events.Event{
			Timestamp: time.Now().UTC(),
			Type: "ai_alert",
			Payload: string(alertPayload),
		})
		store.mu.Unlock()
		
		w.WriteHeader(http.StatusOK)
	})

	fmt.Println("AURA Gateway v3.0 listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

// WebSocket connection handler
func handleStream(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade err:", err)
		return
	}
	log.Printf("Agent connected via WebSocket: %s", conn.RemoteAddr())
	
	activeAgents.mu.Lock()
	activeAgents.conns[conn] = true
	activeAgents.mu.Unlock()

	defer func() {
		activeAgents.mu.Lock()
		delete(activeAgents.conns, conn)
		activeAgents.mu.Unlock()
		conn.Close()
		log.Printf("Agent disconnected: %s", conn.RemoteAddr())
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var batch []events.Event
		if err := json.Unmarshal(message, &batch); err != nil {
			log.Printf("Failed to map websocket events: %v", err)
			continue
		}

		store.mu.Lock()
		store.Events = append(store.Events, batch...)
		
		// Write to VictoriaMetrics Time-Series Database
		go func(ip string, evts []events.Event) {
			if err := tsdb.WriteBatch(ip, evts); err != nil {
				log.Printf("DB Write Error: %v", err)
			}
		}(conn.RemoteAddr().String(), batch)
		
		const MaxEvents = 10000
		if len(store.Events) > MaxEvents {
			store.Events = store.Events[len(store.Events)-MaxEvents:]
		}

		store.Stats.EventCount += len(batch)
		store.Stats.AgentLastSeen = time.Now()

		// 1. Process telemetry through AI Engine
		ip := conn.RemoteAddr().String()
		
		// Map Event struct to anonymous struct expected by AI
		aiEvts := make([]struct{ Type, Payload string }, len(batch))
		for i, e := range batch {
			aiEvts[i].Type = e.Type
			aiEvts[i].Payload = e.Payload
			
			if e.Type == "policy_violation" {
				store.Stats.ViolationCount++
				fmt.Printf("ALERT (via WS): %s\n", e.Payload)
			}
		}

		score := aiEngine.ProcessTelemetry(ip, aiEvts)
		
		// 2. Anomaly Threshold Check & HITL Event Alerting
		if score > 0.85 {
			offendingPayload := aiEngine.GetLastPayload(ip)
			
			// 1. Immediately Auto-Freeze
			suspendRule, err := ai.SynthesizeSuspendPolicy(ip, offendingPayload)
			if err == nil {
				BroadcastPolicy(suspendRule)
				log.Printf("AI Autonomously Suspended payload: %s", offendingPayload)
			}
			
			// 2. Push HITL Alert to Dashboard Feed
			alertPayload, _ := json.Marshal(map[string]any{
				"rule_id": "suspend_process",
				"agent_id": ip,
				"process": map[string]any{"name": offendingPayload, "pid": "0"},
			})
			store.Events = append(store.Events, events.Event{
				Timestamp: time.Now().UTC(),
				Type: "ai_alert",
				Payload: string(alertPayload),
			})
		}
		
		store.mu.Unlock()
	}
}

func BroadcastPolicy(rawPolicy []byte) {
	activeAgents.mu.Lock()
	defer activeAgents.mu.Unlock()
	log.Printf("Broadcasting policy to %d active agents", len(activeAgents.conns))
	for conn := range activeAgents.conns {
		if err := conn.WriteMessage(websocket.TextMessage, rawPolicy); err != nil {
			log.Printf("Failed to broadcast to agent: %v", err)
		}
	}
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Read failed", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var batch []events.Event
	if err := json.Unmarshal(body, &batch); err != nil {
		log.Printf("Failed to map events: %v", err)
		http.Error(w, "Bad JSON", http.StatusBadRequest)
		return
	}

	store.mu.Lock()
	store.Events = append(store.Events, batch...)
	const MaxEvents = 10000
	if len(store.Events) > MaxEvents {
		store.Events = store.Events[len(store.Events)-MaxEvents:]
	}

	store.Stats.EventCount += len(batch)
	store.Stats.AgentLastSeen = time.Now()

	for _, e := range batch {
		if e.Type == "policy_violation" {
			store.Stats.ViolationCount++
			fmt.Printf("ALERT: %s\n", e.Payload)
		}
	}
	store.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	start := 0
	if len(store.Events) > 50 {
		start = len(store.Events) - 50
	}

	resp := map[string]any{
		"stats":  store.Stats,
		"events": store.Events[start:],
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
