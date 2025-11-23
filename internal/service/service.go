package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"gopkg.in/yaml.v3"

	"sentinel-agent/internal/config"
	"sentinel-agent/internal/events"
	"sentinel-agent/internal/gateway"
	"sentinel-agent/internal/logging"
	"sentinel-agent/internal/modules"
	"sentinel-agent/internal/policy"
)

type Service struct {
	cfg    *config.Config
	log    *logging.Logger
	store  events.EventStore
	gc     gateway.GatewayClient
	mods   *modules.Registry
	pol    *policy.DBStore
	ctx    context.Context
	cancel context.CancelFunc
}

func New(cfg *config.Config, logger *logging.Logger) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{cfg: cfg, log: logger, ctx: ctx, cancel: cancel}
	s.mods = modules.NewRegistry()
	// register built-in modules
	s.mods.Register(modules.NewSysInfoModule())
	s.mods.Register(modules.NewProcessModule())
	// init policy store and register enforcer
	if ps, err := policy.NewDBStore(cfg.DBPath); err == nil {
		s.pol = ps
		s.mods.Register(modules.NewPolicyEnforcer(ps))
		// if no policy exists, seed a default inert policy (detect-only)
		if s.pol.Get() == nil {
			defaultPolicy := &policy.Policy{
				ID:   "active",
				Name: "default",
				Raw:  `{"version":1,"rules":[{"id":"p1","type":"block_process","match":"cmd.exe","action":"alert"},{"id":"p2","type":"block_process","match":"notepad.exe","action":"alert"}]}`,
			}
			_ = s.pol.Set(defaultPolicy)
		}
	} else {
		// log but continue without policy enforcement
		s.log.Error("failed to open policy store", "err", err)
	}
	return s
}

func (s *Service) Run() {
	s.log.Info("service starting")

	// start background policy fetcher if configured
	if s.cfg.PolicyURL != "" && s.pol != nil {
		go func() {
			// fetch once immediately, then on ticker
			s.fetchPolicyOnce()
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

	// initialize store
	store, err := events.NewSqliteStore(s.cfg.DBPath)
	if err != nil {
		s.log.Error("failed to open event store", "err", err)
		return
	}
	defer store.Close()
	s.store = store

	// initialize gateway client
	s.gc = gateway.NewHTTPClient(s.cfg.GatewayURL)

	// initial run
	s.runOnce()

	ticker := time.NewTicker(time.Duration(s.cfg.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.runOnce()
		case <-s.ctx.Done():
			s.log.Info("service stopping")
			return
		}
	}
}

func (s *Service) fetchPolicyOnce() {
	if s.cfg.PolicyURL == "" || s.pol == nil {
		return
	}
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
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		s.log.Error("policy read failed", "err", err)
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
		}
	}
}

func (s *Service) runOnce() {
	s.log.Info("running modules")
	ctx := s.ctx
	mods := s.mods.List()
	for _, m := range mods {
		s.log.Info("running module", "module", m.Name())
		if evts, err := m.Run(ctx, s.cfg, s.store, s.gc, s.log); err != nil {
			s.log.Error("module run error", "module", m.Name(), "err", err)
		} else if len(evts) > 0 {
			// persist events
			for _, e := range evts {
				if err := s.store.Save(e); err != nil {
					s.log.Error("failed to save event", "err", err)
				}
			}
			// attempt to send
			if s.gc != nil {
				if err := s.gc.SendEvents(ctx, evts); err != nil {
					s.log.Error("gateway send failed", "err", err)
				}
			}
		}
	}
}

func (s *Service) Stop() {
	s.cancel()
}
