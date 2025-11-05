package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"sentinel-agent/internal/config"
)

type Logger struct {
	l *slog.Logger
}

func New(cfg *config.Config) *Logger {
	// create log directory next to DB (in ProgramData/SentinelAgent)
	var out io.Writer = os.Stdout
	if cfg != nil {
		if cfg.DBPath != "" {
			dir := filepath.Dir(cfg.DBPath)
			_ = os.MkdirAll(dir, 0o755)
			f, err := os.OpenFile(filepath.Join(dir, "agent.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				out = io.MultiWriter(os.Stdout, f)
			}
		}
	}
	handler := slog.NewJSONHandler(out, &slog.HandlerOptions{AddSource: false})
	l := slog.New(handler)
	return &Logger{l: l}
}

func (lg *Logger) Info(msg string, args ...any)  { lg.l.Info(msg, args...) }
func (lg *Logger) Error(msg string, args ...any) { lg.l.Error(msg, args...) }
func (lg *Logger) Debug(msg string, args ...any) { lg.l.Debug(msg, args...) }
