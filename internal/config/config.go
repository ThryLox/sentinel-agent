package config

import (
	"errors"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	GatewayURL           string `toml:"gateway_url"`
	PollIntervalSeconds  int    `toml:"poll_interval_seconds"`
	LogLevel             string `toml:"log_level"`
	DBPath               string `toml:"db_path"`
	PolicyURL            string `toml:"policy_url"`
	PolicyPollSeconds    int    `toml:"policy_poll_seconds"`
	ProcessLimit         int    `toml:"process_limit"`
	PolicyFile           string `toml:"policy_file"`
	GatewayFlushInterval int    `toml:"gateway_flush_interval"`
	GatewayBatchSize     int    `toml:"gateway_batch_size"`
	ForceMemoryDB        bool   `toml:"-"` // runtime override
}

func defaultConfig() *Config {
	// Portable Mode: Default to local directory
	dbPath := "events.db"
	return &Config{
		GatewayURL:           "http://localhost:8080/api/v1/events",
		PollIntervalSeconds:  5,
		LogLevel:             "info",
		DBPath:               dbPath,
		PolicyURL:            "",
		PolicyPollSeconds:    5, // Instant Hot-Reload for Portable Mode
		ProcessLimit:         1000,
		PolicyFile:           "policies.yaml",
		GatewayFlushInterval: 60,
		GatewayBatchSize:     50,
	}
}

func configPath() (string, error) {
	// Portable Mode: Always look in current directory
	return "config.toml", nil
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
	if cfg.ProcessLimit == 0 {
		cfg.ProcessLimit = def.ProcessLimit
	}
	if cfg.PolicyFile == "" {
		cfg.PolicyFile = def.PolicyFile
	}
	if cfg.PolicyPollSeconds == 0 {
		cfg.PolicyPollSeconds = def.PolicyPollSeconds
	}
	if cfg.GatewayFlushInterval == 0 {
		cfg.GatewayFlushInterval = def.GatewayFlushInterval
	}
	if cfg.GatewayBatchSize == 0 {
		cfg.GatewayBatchSize = def.GatewayBatchSize
	}
	return &cfg, nil
}
