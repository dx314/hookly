package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/kardianos/service"
	"github.com/urfave/cli/v2"

	clicmd "hooks.dx314.com/internal/cli"
	"hooks.dx314.com/internal/config"
	svc "hooks.dx314.com/internal/service"
)

// installCommand installs the relay for ./hookly.yaml as a user service.
func installCommand() *cli.Command {
	return &cli.Command{
		Name:  "install",
		Usage: "Install hookly as a user service for the hookly.yaml in this directory",
		Description: "Installs and starts a user service (systemd --user, launchd agent) that runs the\n" +
			"    relay for ./hookly.yaml. No sudo: it runs as you and uses the login saved by\n" +
			"    'hookly login'. Run it again after moving hookly.yaml or upgrading hookly.",
		Action: runInstall,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config",
				Usage: "Path to hookly.yaml",
				Value: "hookly.yaml",
			},
		},
	}
}

// uninstallCommand removes the user service installed by `hookly install`.
func uninstallCommand() *cli.Command {
	return &cli.Command{
		Name:   "uninstall",
		Usage:  "Stop and remove the hookly user service",
		Action: runUninstall,
	}
}

func runInstall(c *cli.Context) error {
	// The hookly.yaml in the current directory (or --config)
	configPath, err := filepath.Abs(c.String("config"))
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	hooklyCfg, err := config.LoadHooklyYAML(configPath)
	if err != nil {
		return fmt.Errorf("load %s: %w\n\nRun 'hookly init' in this directory first", configPath, err)
	}

	// The service authenticates with this user's saved login
	credsMgr, err := clicmd.NewCredentialsManager()
	if err != nil {
		return fmt.Errorf("init credentials manager: %w", err)
	}
	creds, err := credsMgr.Load()
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}
	if creds == nil {
		return errors.New("not logged in\n\nRun 'hookly login' first - the service uses your login")
	}

	cfg := svc.DefaultServiceConfig(true)
	cfg.ConfigPath = configPath
	cfg.WorkingDir = filepath.Dir(configPath)
	if err := cfg.ValidateForInstall(); err != nil {
		return err
	}

	// Reinstall cleanly if it is already there (new path, new binary)
	if status, err := svc.GetServiceStatus(cfg); err == nil && status != service.StatusUnknown {
		fmt.Println("Existing hookly service found - replacing it")
		_ = svc.ControlService(cfg, "stop")
		if err := svc.ControlService(cfg, "uninstall"); err != nil && !isNotInstalledError(err) {
			return fmt.Errorf("remove existing service: %w", err)
		}
	}

	if err := svc.ControlService(cfg, "install"); err != nil {
		return fmt.Errorf("install service: %w", err)
	}
	if err := svc.ControlService(cfg, "start"); err != nil {
		return fmt.Errorf("service installed but failed to start: %w", err)
	}

	fmt.Printf("hookly is installed as a user service and running\n\n")
	fmt.Printf("  Config:    %s\n", configPath)
	fmt.Printf("  Runs as:   %s\n", creds.Username)
	fmt.Printf("  Edge:      %s\n", hooklyCfg.EdgeURL)
	fmt.Printf("  Endpoints: %d\n\n", len(hooklyCfg.Endpoints))

	if runtime.GOOS == "linux" {
		// Without lingering, systemd only runs user services while you are logged in
		if err := enableLinger(); err != nil {
			fmt.Printf("  To keep it running after you log out and start it at boot, run once:\n")
			fmt.Printf("    sudo loginctl enable-linger %s\n\n", currentUsername())
		} else {
			fmt.Printf("  Starts at boot and keeps running after you log out (lingering enabled).\n\n")
		}
		fmt.Printf("  Status:  systemctl --user status hookly\n")
		fmt.Printf("  Logs:    journalctl --user -u hookly -f\n")
	} else {
		fmt.Printf("  Starts when you log in and restarts if it stops.\n\n")
		fmt.Printf("  Status:  hookly service status --user\n")
		fmt.Printf("  Logs:    %s\n", svc.GetLogPath(true))
	}
	fmt.Printf("  Remove:  hookly uninstall\n")
	return nil
}

func runUninstall(c *cli.Context) error {
	cfg := svc.DefaultServiceConfig(true)
	_ = svc.ControlService(cfg, "stop")
	if err := svc.ControlService(cfg, "uninstall"); err != nil {
		if isNotInstalledError(err) {
			fmt.Println("hookly service is not installed")
			return nil
		}
		return fmt.Errorf("uninstall service: %w", err)
	}
	fmt.Println("hookly service stopped and removed")
	return nil
}

// enableLinger lets the user's systemd instance (and so the service) run
// without an open login session. Allowed for your own account on most systems.
func enableLinger() error {
	loginctl, err := exec.LookPath("loginctl")
	if err != nil {
		return err
	}
	return exec.Command(loginctl, "enable-linger", currentUsername()).Run()
}

func currentUsername() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv("USER")
}
