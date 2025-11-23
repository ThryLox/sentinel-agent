package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"sentinel-agent/internal/events"
)

type GatewayClient interface {
	SendEvents(ctx context.Context, evts []events.Event) error
}

type httpClient struct {
	url string
	c   *http.Client
}

func NewHTTPClient(url string) GatewayClient {
	return &httpClient{url: url, c: &http.Client{Timeout: 15 * time.Second}}
}

func (h *httpClient) SendEvents(ctx context.Context, evts []events.Event) error {
	if h.url == "" {
		// no-op for MVP
		return nil
	}
	payload, err := json.Marshal(evts)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Body = http.NoBody
	// Use a simple POST with payload via Do with Buffer
	// Construct request manually to include body
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("gateway returned status %d", resp.StatusCode)
	}
	return nil
}
