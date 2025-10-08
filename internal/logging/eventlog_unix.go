//go:build !windows

package logging

import (
	"log/syslog"
)

var writer *syslog.Writer

// InitEventLog connects to the local syslog daemon on Unix.
func InitEventLog(name string) error {
	var err error
	writer, err = syslog.New(syslog.LOG_WARNING|syslog.LOG_DAEMON, name)
	return err
}

// CloseEventLog closes the syslog writer.
func CloseEventLog() {
	if writer != nil {
		writer.Close()
	}
}

// LogEvent writes a warning/alert to syslog.
func LogEvent(msg string) {
	if writer == nil {
		return
	}
	// Default to warning for now, to match Windows signature
	_ = writer.Warning(msg)
}
