// Package service provides system service management for hookly.
package service

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/kardianos/service"

	"hooks.dx314.com/internal/config"
	"hooks.dx314.com/internal/relay"
)

const (
	serviceName        = "hookly"
	serviceDisplayName = "Hookly Webhook Relay"
	serviceDescription = "Webhook relay client for forwarding webhooks from edge to local services"
	shutdownTimeout    = 5 * time.Second
)

// Program implements service.Interface for the hookly relay.
type Program struct {
	cfg    *ServiceConfig
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Start is called when the service is started.
// Must not block - start work in a goroutine.
func (p *Program) Start(s service.Service) error {
	slog.Info("service starting", "config", p.cfg.ConfigPath)

	// Load hookly config
	hooklyCfg, err := config.LoadHooklyYAML(p.cfg.ConfigPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	// Create relay client
	client := relay.NewClient(hooklyCfg)

	// Start relay in goroutine (must not block)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if err := client.Run(ctx); err != nil && err != context.Canceled {
			slog.Error("relay error", "error", err)
		}
	}()

	slog.Info("service started",
		"edge_url", hooklyCfg.EdgeURL,
		"hub_id", hooklyCfg.HubID,
		"endpoints", len(hooklyCfg.Endpoints),
	)

	return nil
}

// Stop is called when the service is stopped.
func (p *Program) Stop(s service.Service) error {
	slog.Info("service stopping")

	if p.cancel != nil {
		p.cancel()
	}

	// Wait for graceful shutdown with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("service stopped gracefully")
	case <-time.After(shutdownTimeout):
		slog.Warn("service shutdown timed out")
	}

	return nil
}

// NewService creates a configured service.Service instance.
func NewService(cfg *ServiceConfig) (service.Service, error) {
	prg := &Program{cfg: cfg}

	options := make(service.KeyValue)
	options["KeepAlive"] = true
	options["RunAtLoad"] = true

	// For user services on macOS, set UserService option
	if cfg.UserService {
		options["UserService"] = true
	}

	svcConfig := &service.Config{
		Name:        serviceName,
		DisplayName: serviceDisplayName,
		Description: serviceDescription,
		Arguments:   []string{"--service-mode", "--config", cfg.ConfigPath},
		Option:      options,
	}

	// Set working directory if specified
	if cfg.WorkingDir != "" {
		svcConfig.WorkingDirectory = cfg.WorkingDir
	}

	return service.New(prg, svcConfig)
}

// RunServiceMode runs hookly in service mode (called by service manager).
func RunServiceMode(configPath string) error {
	// Setup logging for service mode
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := &ServiceConfig{
		ConfigPath: configPath,
	}

	svc, err := NewService(cfg)
	if err != nil {
		return err
	}

	return svc.Run()
}

// ControlService performs a control action on the service.
func ControlService(cfg *ServiceConfig, action string) error {
	svc, err := NewService(cfg)
	if err != nil {
		return err
	}

	switch action {
	case "install":
		return svc.Install()
	case "uninstall":
		return svc.Uninstall()
	case "start":
		return svc.Start()
	case "stop":
		return svc.Stop()
	case "restart":
		return svc.Restart()
	default:
		return service.ErrNoServiceSystemDetected
	}
}

// GetServiceStatus returns the current service status.
func GetServiceStatus(cfg *ServiceConfig) (service.Status, error) {
	svc, err := NewService(cfg)
	if err != nil {
		return service.StatusUnknown, err
	}

	return svc.Status()
}

// StatusString returns a human-readable status string.
func StatusString(status service.Status) string {
	switch status {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	default:
		return "unknown"
	}
}
