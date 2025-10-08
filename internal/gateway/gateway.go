package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"sentinel-agent/internal/events"
)

type GatewayClient interface {
	SendEvents(ctx context.Context, evts []events.Event) error
	ReceivePolicies(callback func(rawJSON []byte))
	Close() error
}

type wsClient struct {
	url       string
	conn      *websocket.Conn
	connected bool
	mu        sync.Mutex
	callback  func(rawJSON []byte)
}

func NewWebSocketClient(url string) GatewayClient {
	wsUrl := strings.Replace(url, "http://", "ws://", 1)
	wsUrl = strings.Replace(wsUrl, "https://", "wss://", 1)
	if !strings.HasSuffix(wsUrl, "/stream") {
		wsUrl = strings.TrimSuffix(wsUrl, "/") + "/stream"
	}
	
	c := &wsClient{url: wsUrl}
	go c.connectLoop()
	return c
}

func (w *wsClient) connectLoop() {
	if w.url == "" {
		return
	}
	for {
		log.Printf("Connecting to gateway: %s", w.url)
		conn, _, err := websocket.DefaultDialer.Dial(w.url, nil)
		if err != nil {
			log.Printf("Failed to connect to gateway: %v", err)
			w.mu.Lock()
			w.connected = false
			w.mu.Unlock()
			time.Sleep(5 * time.Second)
			continue
		}
		log.Printf("Connected to gateway via WebSocket")
		w.mu.Lock()
		w.conn = conn
		w.connected = true
		w.mu.Unlock()

		// Read loop
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				w.mu.Lock()
				w.connected = false
				w.conn = nil
				w.mu.Unlock()
				conn.Close()
				break
			}
			w.mu.Lock()
			cb := w.callback
			w.mu.Unlock()
			
			if cb != nil {
				cb(msg)
			}
		}
		time.Sleep(5 * time.Second)
	}
}

func (w *wsClient) SendEvents(ctx context.Context, evts []events.Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if !w.connected || w.conn == nil {
		return fmt.Errorf("not connected to gateway")
	}
	payload, err := json.Marshal(evts)
	if err != nil {
		return err
	}
	return w.conn.WriteMessage(websocket.TextMessage, payload)
}

func (w *wsClient) ReceivePolicies(callback func(rawJSON []byte)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callback = callback
}

func (w *wsClient) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.conn != nil {
		w.connected = false
		return w.conn.Close()
	}
	return nil
}
