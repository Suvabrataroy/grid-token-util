package service

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/kardianos/service"
	"github.com/rs/zerolog/log"

	"github.com/grid-computing/grid-worker/internal/config"
)

const (
	serviceName        = "grid-worker"
	serviceDisplayName = "Grid Worker Daemon"
	serviceDescription = "Grid Worker distributed AI coding agent daemon"
)

// serviceProgram is a placeholder program interface for installation purposes.
type serviceProgram struct{}

func (p *serviceProgram) Start(s service.Service) error { return nil }
func (p *serviceProgram) Stop(s service.Service) error  { return nil }

// buildServiceConfig returns a kardianos/service config for the grid-worker daemon.
func buildServiceConfig(execPath string) *service.Config {
	cfg := &service.Config{
		Name:        serviceName,
		DisplayName: serviceDisplayName,
		Description: serviceDescription,
		Executable:  execPath,
		Arguments:   []string{"run"},
		Option: service.KeyValue{
			// Restart on failure
			"OnFailure":      "restart",
			"OnFailureDelay": "5",
		},
	}

	switch runtime.GOOS {
	case "linux":
		cfg.Dependencies = []string{
			"After=network.target",
			"After=network-online.target",
			"Wants=network-online.target",
		}
		cfg.Option["Restart"] = "on-failure"
		cfg.Option["RestartSec"] = "5"
		cfg.Option["Nice"] = "19" // low priority

	case "darwin":
		cfg.Option["KeepAlive"] = true
		cfg.Option["RunAtLoad"] = true

	case "windows":
		cfg.Option["StartType"] = "automatic"
		cfg.Option["OnFailure"] = "restart"
		cfg.Option["OnFailureDelay"] = "5"
	}

	return cfg
}

// Install installs the grid-worker binary as a system service.
func Install(cfg *config.Config, execPath string) error {
	if execPath == "" {
		var err error
		execPath, err = exec.LookPath("grid-worker")
		if err != nil {
			return fmt.Errorf("grid-worker binary not found: %w", err)
		}
	}

	svcCfg := buildServiceConfig(execPath)
	prg := &serviceProgram{}

	svc, err := service.New(prg, svcCfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}

	if err := svc.Install(); err != nil {
		return fmt.Errorf("install service: %w", err)
	}

	log.Info().Str("service", serviceName).Msg("service installed successfully")
	return nil
}

// Uninstall removes the grid-worker system service.
func Uninstall() error {
	svcCfg := buildServiceConfig("")
	prg := &serviceProgram{}

	svc, err := service.New(prg, svcCfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}

	if err := svc.Uninstall(); err != nil {
		return fmt.Errorf("uninstall service: %w", err)
	}

	log.Info().Str("service", serviceName).Msg("service uninstalled successfully")
	return nil
}

// Start starts the installed system service.
func Start() error {
	svcCfg := buildServiceConfig("")
	prg := &serviceProgram{}

	svc, err := service.New(prg, svcCfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}

	if err := svc.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}

	log.Info().Str("service", serviceName).Msg("service started")
	return nil
}

// Stop stops the running system service.
func Stop() error {
	svcCfg := buildServiceConfig("")
	prg := &serviceProgram{}

	svc, err := service.New(prg, svcCfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}

	if err := svc.Stop(); err != nil {
		return fmt.Errorf("stop service: %w", err)
	}

	log.Info().Str("service", serviceName).Msg("service stopped")
	return nil
}
