package main

import (
	"fmt"
	"os"

	"github.com/kardianos/service"
	"github.com/urfave/cli/v2"

	svc "hooks.dx314.com/internal/service"
)

// serviceCommand returns the service subcommand.
func serviceCommand() *cli.Command {
	return &cli.Command{
		Name:  "service",
		Usage: "Manage hookly as a system service",
		Description: `Install and manage hookly as a background service.

By default, installs as a system service (requires sudo).
Use --user flag for a user-level service (no sudo, runs on login).

System services run at boot and require root privileges.
User services run when you log in and don't need sudo.`,
		Subcommands: []*cli.Command{
			{
				Name:   "list",
				Usage:  "List installed hookly services",
				Action: runServiceList,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "user",
						Usage: "List user services",
					},
				},
			},
			{
				Name:  "install",
				Usage: "Install hookly as a system service",
				Description: `Installs hookly to run automatically as a background service.

The --config flag is required and must point to your hookly.yaml.
Use --user to install as a user service (no sudo required).`,
				Action: runServiceInstall,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "config",
						Usage: "Path to hookly.yaml (required)",
					},
					&cli.BoolFlag{
						Name:  "user",
						Usage: "Install as user service (no sudo, runs on login)",
					},
					nameFlag(),
				},
			},
			{
				Name:        "uninstall",
				Usage:       "Remove hookly service",
				Description: "Stops the service if running and removes it from the system.",
				Action:      runServiceUninstall,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "user",
						Usage: "Uninstall user service",
					},
					nameFlag(),
				},
			},
			{
				Name:        "start",
				Usage:       "Start the hookly service",
				Description: "Starts the installed hookly service.",
				Action:      runServiceStart,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "user",
						Usage: "Start user service",
					},
					nameFlag(),
				},
			},
			{
				Name:        "stop",
				Usage:       "Stop the hookly service",
				Description: "Stops the running hookly service.",
				Action:      runServiceStop,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "user",
						Usage: "Stop user service",
					},
					nameFlag(),
				},
			},
			{
				Name:        "restart",
				Usage:       "Restart the hookly service",
				Description: "Stops and starts the hookly service.",
				Action:      runServiceRestart,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "user",
						Usage: "Restart user service",
					},
					nameFlag(),
				},
			},
			{
				Name:        "status",
				Usage:       "Check hookly service status",
				Description: "Shows whether the service is running, stopped, or not installed.",
				Action:      runServiceStatus,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "user",
						Usage: "Check user service status",
					},
					nameFlag(),
				},
			},
			{
				Name:  "logs",
				Usage: "View hookly service logs",
				Description: `View log output from the hookly service.

Use -f/--follow to stream logs in real-time (like tail -f).
Use -n/--lines to control how many lines to show.`,
				Action: runServiceLogs,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "follow",
						Aliases: []string{"f"},
						Usage:   "Follow log output (like tail -f)",
					},
					&cli.IntFlag{
						Name:    "lines",
						Aliases: []string{"n"},
						Usage:   "Number of lines to show",
						Value:   50,
					},
					&cli.BoolFlag{
						Name:  "user",
						Usage: "View user service logs",
					},
					nameFlag(),
				},
			},
		},
	}
}

// nameFlag selects a named service ("hookly-<name>"); without it the
// commands act on the unnamed "hookly" service.
func nameFlag() cli.Flag {
	return &cli.StringFlag{
		Name:  "name",
		Usage: "Service name, for running several relays on one machine (unit hookly-<name>)",
	}
}

// buildServiceConfig creates a ServiceConfig from CLI flags.
func buildServiceConfig(c *cli.Context) (*svc.ServiceConfig, error) {
	userService := c.Bool("user")
	cfg := svc.DefaultServiceConfig(userService)

	if name := c.String("name"); name != "" {
		if err := svc.ValidateName(name); err != nil {
			return nil, err
		}
		cfg.Name = name
	}

	// Override config path if specified
	if configPath := c.String("config"); configPath != "" {
		cfg.ConfigPath = configPath
	}

	return cfg, nil
}

func runServiceInstall(c *cli.Context) error {
	cfg, err := buildServiceConfig(c)
	if err != nil {
		return err
	}

	// Validate for installation
	if err := cfg.ValidateForInstall(); err != nil {
		return err
	}

	// Make config path absolute for service
	absConfigPath, err := makeAbsolute(cfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	cfg.ConfigPath = absConfigPath

	if err := svc.ControlService(cfg, "install"); err != nil {
		if isPermissionError(err) {
			return fmt.Errorf("permission denied\n\nTry one of:\n  sudo hookly service install --config %s\n  hookly service install --user --config %s", cfg.ConfigPath, cfg.ConfigPath)
		}
		return fmt.Errorf("install service: %w", err)
	}

	fmt.Printf("Service installed successfully\n")
	fmt.Printf("Config: %s\n", cfg.ConfigPath)
	fmt.Printf("Logs:   %s\n", svc.LogsHint(cfg))
	fmt.Printf("\nStart with: hookly service start%s\n", serviceFlags(cfg))

	return nil
}

func runServiceUninstall(c *cli.Context) error {
	cfg, err := buildServiceConfig(c)
	if err != nil {
		return err
	}

	// Check if service is running first
	status, err := svc.GetServiceStatus(cfg)
	if err == nil && status == service.StatusRunning {
		fmt.Println("Stopping service first...")
		if err := svc.ControlService(cfg, "stop"); err != nil {
			fmt.Printf("Warning: failed to stop service: %v\n", err)
		}
	}

	if err := svc.ControlService(cfg, "uninstall"); err != nil {
		if isPermissionError(err) {
			return fmt.Errorf("permission denied\n\nTry: sudo hookly service uninstall")
		}
		return fmt.Errorf("uninstall service: %w", err)
	}

	fmt.Println("Service uninstalled successfully")
	return nil
}

func runServiceStart(c *cli.Context) error {
	cfg, err := buildServiceConfig(c)
	if err != nil {
		return err
	}

	if err := svc.ControlService(cfg, "start"); err != nil {
		if isNotInstalledError(err) {
			return fmt.Errorf("service not installed\n\nInstall first with: hookly service install --config PATH")
		}
		if isPermissionError(err) {
			return fmt.Errorf("permission denied\n\nTry: sudo hookly service start")
		}
		return fmt.Errorf("start service: %w", err)
	}

	fmt.Println("Service started")
	return nil
}

func runServiceStop(c *cli.Context) error {
	cfg, err := buildServiceConfig(c)
	if err != nil {
		return err
	}

	if err := svc.ControlService(cfg, "stop"); err != nil {
		if isNotInstalledError(err) {
			return fmt.Errorf("service not installed")
		}
		if isPermissionError(err) {
			return fmt.Errorf("permission denied\n\nTry: sudo hookly service stop")
		}
		return fmt.Errorf("stop service: %w", err)
	}

	fmt.Println("Service stopped")
	return nil
}

func runServiceRestart(c *cli.Context) error {
	cfg, err := buildServiceConfig(c)
	if err != nil {
		return err
	}

	if err := svc.ControlService(cfg, "restart"); err != nil {
		if isNotInstalledError(err) {
			return fmt.Errorf("service not installed\n\nInstall first with: hookly service install --config PATH")
		}
		if isPermissionError(err) {
			return fmt.Errorf("permission denied\n\nTry: sudo hookly service restart")
		}
		return fmt.Errorf("restart service: %w", err)
	}

	fmt.Println("Service restarted")
	return nil
}

func runServiceStatus(c *cli.Context) error {
	cfg, err := buildServiceConfig(c)
	if err != nil {
		return err
	}

	status, err := svc.GetServiceStatus(cfg)
	if err != nil {
		if isNotInstalledError(err) {
			fmt.Println("Status: not installed")
			return nil
		}
		return fmt.Errorf("get status: %w", err)
	}

	fmt.Printf("Status: %s\n", svc.StatusString(status))
	fmt.Printf("Logs:   %s\n", svc.LogsHint(cfg))
	return nil
}

func runServiceLogs(c *cli.Context) error {
	name := c.String("name")
	if name != "" {
		if err := svc.ValidateName(name); err != nil {
			return err
		}
	}
	logsCfg := &svc.LogsConfig{
		Name:        name,
		Follow:      c.Bool("follow"),
		Lines:       c.Int("lines"),
		UserService: c.Bool("user"),
	}

	return svc.ViewLogs(logsCfg)
}

func runServiceList(c *cli.Context) error {
	userService := c.Bool("user")
	installed, err := svc.ListInstalled(userService)
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}
	if len(installed) == 0 {
		fmt.Println("No hookly services installed")
		return nil
	}
	for _, inst := range installed {
		cfg := &svc.ServiceConfig{Name: inst.Name, UserService: userService}
		state := "unknown"
		if status, err := svc.GetServiceStatus(cfg); err == nil {
			state = svc.StatusString(status)
		}
		fmt.Printf("%-24s %-8s %s\n", inst.Unit, state, inst.ConfigPath)
	}
	return nil
}

// serviceFlags repeats the flags that select cfg's service, for hints.
func serviceFlags(cfg *svc.ServiceConfig) string {
	flags := ""
	if cfg.UserService {
		flags += " --user"
	}
	if cfg.Name != "" {
		flags += " --name " + cfg.Name
	}
	return flags
}

// makeAbsolute converts a relative path to absolute.
func makeAbsolute(path string) (string, error) {
	if len(path) > 0 && path[0] == '/' {
		return path, nil
	}
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home + path[1:], nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return wd + "/" + path, nil
}

// isPermissionError checks if an error is a permission error.
func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "permission denied") ||
		contains(errStr, "access denied") ||
		contains(errStr, "operation not permitted")
}

// isNotInstalledError checks if an error indicates the service is not installed.
func isNotInstalledError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "not installed") ||
		contains(errStr, "does not exist") ||
		contains(errStr, "could not be found")
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
