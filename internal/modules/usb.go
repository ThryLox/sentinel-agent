package modules

import (
	"context"
	"encoding/json"
	"time"

	"github.com/shirou/gopsutil/disk"

	"sentinel-agent/internal/bus"
	"sentinel-agent/internal/config"
	"sentinel-agent/internal/events"
	"sentinel-agent/internal/gateway"
	"sentinel-agent/internal/logging"
)

type usbModule struct {
	knownDrives map[string]bool
}

func NewUSBModule() Module {
	return &usbModule{
		knownDrives: make(map[string]bool),
	}
}

func (m *usbModule) Name() string { return "usb_monitor" }

func (m *usbModule) Start(ctx context.Context, bus *bus.Bus, cfg *config.Config, gc gateway.GatewayClient, log *logging.Logger) error {
	run := func() {
		parts, err := disk.Partitions(true)
		if err != nil {
			log.Error("usb fetch failed", "err", err)
			return
		}

		current := make(map[string]bool)
		var newDrives []string

		for _, p := range parts {
			current[p.Device] = true
			if !m.knownDrives[p.Device] {
				// Found a new drive
				newDrives = append(newDrives, p.Device)
			}
		}

		// First run: just populate known drives, don't alert
		if len(m.knownDrives) == 0 {
			m.knownDrives = current
			return
		}

		// update known state
		m.knownDrives = current

		now := time.Now().UTC()
		for _, d := range newDrives {
			payload, _ := json.Marshal(map[string]string{
				"action": "connected",
				"device": d,
			})
			evt := events.Event{
				Timestamp: now,
				Type:      "device_change",
				Payload:   string(payload),
			}
			bus.Publish(evt)
		}
	}

	go func() {
		run()
		ticker := time.NewTicker(time.Duration(cfg.PollIntervalSeconds) * time.Second) // Could be faster for USB
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
	return nil
}
