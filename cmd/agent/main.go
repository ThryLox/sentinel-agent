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
	install := flag.Bool("install", false, "install service")
	uninstall := flag.Bool("uninstall", false, "uninstall service")
	runNow := flag.Bool("run", false, "run in foreground")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Println("config load error:", err)
		os.Exit(1)
	}
	logger := logging.New(cfg)

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
		cfg, _ := config.Load()
		logger := logging.New(cfg)
		svc := agentservice.New(cfg, logger)
		svc.Run()
		return
	}

	err = s.Run()
	if err != nil {
		logger.Error("service run error", "err", err)
	}
}
