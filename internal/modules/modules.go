package modules

import (
	"sync"

	"context"
	"sentinel-agent/internal/bus"
	"sentinel-agent/internal/config"
	"sentinel-agent/internal/gateway"
	"sentinel-agent/internal/logging"
)

type Module interface {
	Name() string
	Start(ctx context.Context, bus *bus.Bus, cfg *config.Config, gc gateway.GatewayClient, log *logging.Logger) error
}

type Registry struct {
	mu   sync.RWMutex
	mods []Module
}

func NewRegistry() *Registry { return &Registry{} }

func (r *Registry) Register(m Module) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mods = append(r.mods, m)
}

func (r *Registry) List() []Module {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Module, len(r.mods))
	copy(out, r.mods)
	return out
}
