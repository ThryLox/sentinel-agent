package db

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"sentinel-agent/internal/events"
)

type VictoriaClient struct {
	URL string
}

func NewVictoriaClient(url string) *VictoriaClient {
	return &VictoriaClient{URL: url}
}

// WriteBatch sends a batch of events to VictoriaMetrics using the InfluxDB Line Protocol
func (vc *VictoriaClient) WriteBatch(agentIP string, evts []events.Event) error {
	if vc.URL == "" {
		return nil
	}
	
	// Fast builder
	var buf strings.Builder
	for _, e := range evts {
		// Escape spaces in tags
		safeType := strings.ReplaceAll(e.Type, " ", "_")
		safeAgent := strings.ReplaceAll(agentIP, " ", "_")
		if safeAgent == "" {
			safeAgent = "unknown_agent"
		}
		
		// Influx Line Protocol: measurement,tags fields timestamp_nanos
		// e.g. aura_event,agent=192.168.1.5,type=process_launch count=1i 1434055562000000000
		line := fmt.Sprintf("aura_event,agent=%s,type=%s count=1i %d\n", safeAgent, safeType, e.Timestamp.UnixNano())
		buf.WriteString(line)
	}

	req, err := http.NewRequest(http.MethodPost, vc.URL+"/write", bytes.NewBufferString(buf.String()))
	if err != nil {
		return err
	}
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		return fmt.Errorf("VictoriaMetrics HTTP %d", resp.StatusCode)
	}
	return nil
}
