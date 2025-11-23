package events

import "time"

type Event struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Payload   string    `json:"payload"`
}

type EventStore interface {
	Save(e Event) error
	List(limit int) ([]Event, error)
	Close() error
}
