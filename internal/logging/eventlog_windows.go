//go:build windows

package logging

import (
	"fmt"

	"golang.org/x/sys/windows/svc/eventlog"
)

var eventLog *eventlog.Log

// InitEventLog initializes the Windows Event Log for the service.
func InitEventLog(name string) error {
	const (
		// Simple bitmask for supported types
		Error   = 0x0001
		Warning = 0x0002
		Info    = 0x0004
	)
	// We point to the current executable as the message file (generic),
	// or standard windows libraries if we don't have a message resource.
	// For MVP, passing the executable path is common, but 'name' works if it's just a key.
	// We use Install(src, msgFile, useExpandEnv, supportedTypes).
	err := eventlog.Install(name, name, false, Error|Warning|Info)
	if err != nil {
		// key might already exist, which is fine
		// checking specific error is hard across versions, so we try to open anyway
	}

	l, err := eventlog.Open(name)
	if err != nil {
		return err
	}
	eventLog = l
	return nil
}

// LogEvent writes a warning/alert to the Windows Event Log.
// We use Warning for policy violations.
func LogEvent(msg string) {
	if eventLog != nil {
		fmt.Printf("DEBUG: Writing to EventLog: %s\n", msg)
		// Allow duplicate events, simply log it.
		// Event ID 1 is generic.
		_ = eventLog.Warning(1, msg)
	} else {
		fmt.Println("DEBUG: EventLog not initialized, skipping write")
	}
}

// CloseEventLog closes the handle.
func CloseEventLog() {
	if eventLog != nil {
		_ = eventLog.Close()
	}
}
