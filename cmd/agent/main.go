package main

import (
	"flag"
	"fmt"
	"os"

	ks "github.com/kardianos/service"

	"sentinel-agent/internal/config"
	"sentinel-agent/internal/logging"
	agentservice "sentinel-agent/internal/service"
)

type program struct{}

func (p *program) Start(s ks.Service) error {
	// Start should not block. agentservice.Run will run until stopped.
	go func() {
		cfg, err := config.Load()
		if err != nil {
			fmt.Println("failed to load config:", err)
			return
		}
		logger := logging.New(cfg)
		svc := agentservice.New(cfg, logger)
		svc.Run()
	}()
	return nil
}

func (p *program) Stop(s ks.Service) error {
	// let the service shutdown via context inside agentservice
	return nil
}

func main() {
	// 1. Auto-Elevation Check (Cross-Platform Interface)
	if !isAdmin() {
		runMeElevated()
		return // Exit original process
	}

	install := flag.Bool("install", false, "install service")
	uninstall := flag.Bool("uninstall", false, "uninstall service")
	runNow := flag.Bool("run", false, "run in foreground")
	useMemory := flag.Bool("memory", false, "use in-memory database (ephemeral mode)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Println("config load error:", err)
		os.Exit(1)
	}

	if *useMemory {
		cfg.ForceMemoryDB = true
		cfg.DBPath = ":memory:"
		fmt.Println("WARNING: Running in EPHEMERAL MODE (In-Memory DB). Data will be lost on exit.")
	}

	logger := logging.New(cfg)

	// Initialize Windows Event Log
	if err := logging.InitEventLog("SentinelAgent"); err != nil {
		logger.Error("failed to init event log", "err", err)
	}
	defer logging.CloseEventLog()

	svcConfig := &ks.Config{
		Name:        "SentinelAgent",
		DisplayName: "Sentinel Agent",
		Description: "Lightweight endpoint agent (MVP)",
	}

	prg := &program{}
	s, err := ks.New(prg, svcConfig)
	if err != nil {
		logger.Error("service.New failed", "err", err)
		os.Exit(1)
	}

	if *install {
		err = s.Install()
		if err != nil {
			logger.Error("install failed", "err", err)
		} else {
			logger.Info("service installed")
		}
		return
	}
	if *uninstall {
		err = s.Uninstall()
		if err != nil {
			logger.Error("uninstall failed", "err", err)
		} else {
			logger.Info("service uninstalled")
		}
		return
	}
	if *runNow {
		// Do NOT reload config here, or we lose the -memory override
		// cfg, _ := config.Load()

		// Re-init logger just in case (though we have one)
		// svc uses the cfg we modified above
		svc := agentservice.New(cfg, logger)
		svc.Run()
		return
	}

	err = s.Run()
	if err != nil {
		logger.Error("service run error", "err", err)
	}
}
