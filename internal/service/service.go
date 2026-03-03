package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"

	"sentinel-agent/internal/assets"
	"sentinel-agent/internal/bus"
	"sentinel-agent/internal/config"
	"sentinel-agent/internal/events"
	"sentinel-agent/internal/gateway"
	"sentinel-agent/internal/logging"
	"sentinel-agent/internal/modules"
	"sentinel-agent/internal/policy"

	"gopkg.in/yaml.v3"
)

type Service struct {
	cfg    *config.Config
	log    *logging.Logger
	store  events.EventStore
	gc     gateway.GatewayClient
	mods   *modules.Registry
	pol    *policy.DBStore
	bus    *bus.Bus
	ctx    context.Context
	cancel context.CancelFunc
}

func New(cfg *config.Config, logger *logging.Logger) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{cfg: cfg, log: logger, ctx: ctx, cancel: cancel}
	if cfg.ForceMemoryDB {
		s.log.Info("initializing service in EPHEMERAL MODE (memory db)")
	}
	s.bus = bus.New() // Init v2 Bus

	s.mods = modules.NewRegistry() // Restore Module Registry Init

	// init policy store and register enforcer FIRST (so it catches initial events)
	if ps, err := policy.NewDBStore(cfg.DBPath); err == nil {
		s.pol = ps
		s.mods.Register(modules.NewPolicyEnforcer(ps))
		// if no policy exists, seed a default inert policy
		if s.pol.Get() == nil {
			defaultPolicy := &policy.Policy{
				ID:   "active",
				Name: "default",
				Raw:  `{"version":1,"rules":[{"id":"p1","type":"block_process","match":"cmd.exe","action":"alert"}]}`,
			}
			_ = s.pol.Set(defaultPolicy)
		}
	} else {
		// log but continue without policy enforcement
		s.log.Error("failed to open policy store", "err", err)
	}

	// register producers
	s.mods.Register(modules.NewSysInfoModule())
	s.mods.Register(modules.NewProcessModule())
	s.mods.Register(modules.NewUSBModule())
	return s
}

func (s *Service) Run() {
	s.log.Info("service starting")

	// Initial Fetch (Embedded or Local)
	s.fetchPolicyOnce()

	// start background policy fetcher if configured
	if (s.cfg.PolicyURL != "" || s.cfg.PolicyFile != "") && s.pol != nil {
		go func() {
			ticker := time.NewTicker(time.Duration(s.cfg.PolicyPollSeconds) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					s.fetchPolicyOnce()
				case <-s.ctx.Done():
					return
				}
			}
		}()
	}

	// initialize policy store (singleton)
	if s.pol == nil {
		if ps, err := policy.NewDBStore(s.cfg.DBPath); err == nil {
			s.pol = ps
		} else {
			s.log.Error("failed to open policy store", "err", err)
		}
	}

	// initialize event store
	store, err := events.NewSqliteStore(s.cfg.DBPath)
	if err != nil {
		s.log.Error("failed to open event store", "err", err)
		return
	}
	defer store.Close()
	s.store = store

	// initialize gateway client
	if s.cfg.GatewayURL != "" {
		s.gc = gateway.NewWebSocketClient(s.cfg.GatewayURL)
		
		// Setup WebSocket async policy sink
		s.gc.ReceivePolicies(func(rawJSON []byte) {
			s.log.Info("received live policy push from gateway over WS")
			var doc struct {
				Version  int              `yaml:"version" json:"version"`
				Policies []map[string]any `yaml:"policies" json:"policies"`
			}
			if err := json.Unmarshal(rawJSON, &doc); err != nil {
				// Try YAML fallback
				if yamlErr := yaml.Unmarshal(rawJSON, &doc); yamlErr != nil {
					s.log.Error("failed to parse websocket policy payload", "err", err)
					return
				}
			}
			count := 0
			for _, p := range doc.Policies {
				id, _ := p["id"].(string)
				name, _ := p["name"].(string)
				rawMap := map[string]any{"version": doc.Version, "id": id, "name": name, "rules": p["rules"]}
				jb, _ := json.Marshal(rawMap)
				pol := &policy.Policy{ID: id, Name: name, Raw: string(jb), Updated: time.Now().UTC()}
				if err := s.pol.Set(pol); err == nil {
					s.log.Info("ws policy stored dynamically", "id", id)
					count++
				}
			}
			if count > 0 {
				s.bus.Publish(events.Event{Timestamp: time.Now().UTC(), Type: "policy_updated"})
			}
		})
	}
	
	// Start v2 Persistence Sink (Skip in Ephemeral Mode)
	if !s.cfg.ForceMemoryDB {
		s.StartPersister(s.bus)
	} else {
		s.log.Info("persistence disabled locally (ephemeral mode)")
	}

	// Start Performance Profiler (10s interval for analysis)
	go s.monitorPerformance()

	// Start Gateway Shipper (Batched)
	s.StartGatewayShipper(s.bus)

	// Start all modules (Async)
	ctx := s.ctx
	mods := s.mods.List()
	for _, m := range mods {
		s.log.Info("starting module", "module", m.Name())
		if err := m.Start(ctx, s.bus, s.cfg, s.gc, s.log); err != nil {
			s.log.Error("module start error", "module", m.Name(), "err", err)
		}
	}

	// Block until stop
	<-s.ctx.Done()
	s.log.Info("service stopping")
}

func (s *Service) fetchPolicyOnce() {
	if s.pol == nil {
		return
	}
	var b []byte
	var err error

	// Priority 1: Local File
	if s.cfg.PolicyFile != "" {
		b, err = os.ReadFile(s.cfg.PolicyFile)
		if err != nil {
			// Fallback: Embed
			if os.IsNotExist(err) {
				s.log.Info("local policy file not found, using embedded defaults")
				b = assets.DefaultPolicies
			} else {
				s.log.Error("failed to read local policy file", "file", s.cfg.PolicyFile, "err", err)
				return
			}
		} else {
			s.log.Info("loaded local policy file")
		}
	} else if s.cfg.PolicyURL != "" {
		// Priority 2: Remote URL
		req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, s.cfg.PolicyURL, nil)
		if err != nil {
			s.log.Error("policy fetch request failed", "err", err)
			return
		}
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			s.log.Error("policy fetch failed", "err", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			s.log.Error("policy fetch returned status", "status", resp.StatusCode)
			return
		}
		b, err = io.ReadAll(resp.Body)
		if err != nil {
			s.log.Error("policy read failed", "err", err)
			return
		}
	} else {
		// No source configured
		return
	}

	// parse YAML to ensure it's valid and split into policies
	var doc struct {
		Version  int              `yaml:"version"`
		Policies []map[string]any `yaml:"policies"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		s.log.Error("policy yaml parse failed", "err", err)
		return
	}
	// store each policy by id
	count := 0
	for _, p := range doc.Policies {
		id, _ := p["id"].(string)
		name, _ := p["name"].(string)
		// canonicalize to JSON for storage
		rawMap := map[string]any{"version": doc.Version, "id": id, "name": name, "rules": p["rules"]}
		jb, _ := json.Marshal(rawMap)
		pol := &policy.Policy{ID: id, Name: name, Raw: string(jb), Updated: time.Now().UTC()}
		if err := s.pol.Set(pol); err != nil {
			s.log.Error("failed to set policy", "id", id, "err", err)
		} else {
			s.log.Info("policy stored", "id", id)
			count++
		}
	}
	if count > 0 {
		// Calculate simple checksum or just notify
		s.bus.Publish(events.Event{Timestamp: time.Now().UTC(), Type: "policy_updated"})
	}
}

func (s *Service) StartGatewayShipper(b *bus.Bus) {
	if s.gc == nil {
		return
	}
	ch := b.Subscribe()
	go func() {
		var batch []events.Event

		flushInterval := time.Duration(s.cfg.GatewayFlushInterval) * time.Second
		if flushInterval <= 0 {
			flushInterval = 60 * time.Second
		}
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()

		backoffDuration := time.Duration(0)
		maxBackoff := 5 * time.Minute

		flush := func(force bool) {
			if len(batch) == 0 {
				return
			}
			// Check Backoff
			if backoffDuration > 0 {
				s.log.Info("gateway backoff active", "retry_in", backoffDuration)
				time.Sleep(backoffDuration)
			}

			if err := s.gc.SendEvents(s.ctx, batch); err != nil {
				s.log.Error("gateway send failed", "err", err)

				// Smart Backoff Logic
				errMsg := err.Error()
				if isAuthError(errMsg) { // 403 Forbidden
					if backoffDuration == 0 {
						backoffDuration = 1 * time.Minute
					} else {
						backoffDuration *= 2
						if backoffDuration > maxBackoff {
							backoffDuration = maxBackoff
						}
					}
					s.log.Info("gateway 403 forbidden: backing off", "duration", backoffDuration)
				} else {
					// Other errors (500, network): shorter backoff
					backoffDuration = 5 * time.Second
				}
				// Keep batch to retry? Or drop? For now, we drop to avoid memory leak if permanent failure
				// But real agent might persist to disk. Here we clear.
			} else {
				// Success
				backoffDuration = 0
			}
			batch = nil // clear buffer
		}

		for {
			select {
			case <-s.ctx.Done():
				return
			case e := <-ch:
				batch = append(batch, e)
				// Critical Alert = Immediate Flush
				if e.Type == "policy_violation" {
					flush(true)
				} else if len(batch) >= s.cfg.GatewayBatchSize {
					flush(false)
				}
			case <-ticker.C:
				flush(false)
			}
		}
	}()
}

func isAuthError(err string) bool {
	// Crude string check since our gateway client returns simple errors
	// In production, type assertion on custom error struct is better
	return len(err) > 0 && (contains(err, "403") || contains(err, "401"))
}

func contains(s, substr string) bool {
	// naive contains for dependency-free
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func (s *Service) monitorPerformance() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			// Convert to MB
			allocMB := float64(m.Alloc) / 1024 / 1024
			sysMB := float64(m.Sys) / 1024 / 1024

			// Payload
			stats := map[string]any{
				"alloc_mb":   allocMB,
				"sys_mb":     sysMB,
				"goroutines": runtime.NumGoroutine(),
			}
			payload, _ := json.Marshal(stats)

			// Log locally for debug
			s.log.Info("performance stats", "alloc_mb", allocMB, "goroutines", runtime.NumGoroutine())

			// Publish to Bus (sends to Gateway)
			s.bus.Publish(events.Event{
				Timestamp: time.Now().UTC(),
				Type:      "agent_health",
				Payload:   string(payload),
			})
		}
	}
}

func (s *Service) Stop() {
	s.cancel()
}
