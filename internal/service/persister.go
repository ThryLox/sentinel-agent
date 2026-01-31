package service

import (
	"fmt"
	"sentinel-agent/internal/bus"
	"sentinel-agent/internal/logging"
)

// StartPersister starts a background goroutine that drains the event channel
// and writes events to SQLite. This ensures only ONE goroutine touches the DB for writes.
func (s *Service) StartPersister(b *bus.Bus) {
	ch := b.Subscribe()
	go func() {
		s.log.Info("persister started")
		for {
			select {
			case e := <-ch:
				if err := s.store.Save(e); err != nil {
					s.log.Error("persister failed to save", "err", err)
				}
				// Also handle Windows Event Log here (centralized alerting)
				if e.Type == "policy_violation" {
					logging.LogEvent(fmt.Sprintf("Sentinel Agent Alert: %s", e.Payload))
				}
			case <-s.ctx.Done():
				s.log.Info("persister stopping")
				return
			}
		}
	}()
}
