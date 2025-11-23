package service

import (
	"time"
)

// RunEvery starts a goroutine that runs fn every interval until stop channel is closed.
func RunEvery(interval time.Duration, stop <-chan struct{}, fn func()) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fn()
			case <-stop:
				return
			}
		}
	}()
}
