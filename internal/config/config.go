package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	GatewayURL          string `toml:"gateway_url"`
	PollIntervalSeconds int    `toml:"poll_interval_seconds"`
	LogLevel            string `toml:"log_level"`
	DBPath              string `toml:"db_path"`
	PolicyURL           string `toml:"policy_url"`
	PolicyPollSeconds   int    `toml:"policy_poll_seconds"`
}

func defaultConfig() *Config {
	progData := os.Getenv("ProgramData")
	if progData == "" {
		progData = `C:\ProgramData`
	}
	dbPath := filepath.Join(progData, "SentinelAgent", "events.db")
	return &Config{
		GatewayURL:          "https://example.com/api",
		PollIntervalSeconds: 60,
		LogLevel:            "info",
		DBPath:              dbPath,
		PolicyURL:           "",
		PolicyPollSeconds:   300,
	}
}

func configPath() (string, error) {
	progData := os.Getenv("ProgramData")
	if progData == "" {
		progData = `C:\ProgramData`
	}
	dir := filepath.Join(progData, "SentinelAgent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		cfg := defaultConfig()
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		enc := toml.NewEncoder(f)
		if err := enc.Encode(cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	// fill defaults where empty
	def := defaultConfig()
	if cfg.GatewayURL == "" {
		cfg.GatewayURL = def.GatewayURL
	}
	if cfg.PollIntervalSeconds == 0 {
		cfg.PollIntervalSeconds = def.PollIntervalSeconds
	}
	if cfg.DBPath == "" {
		cfg.DBPath = def.DBPath
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = def.LogLevel
	}
	return &cfg, nil
}
