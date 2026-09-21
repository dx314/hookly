package service

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
)

// LogsConfig holds options for viewing service logs.
type LogsConfig struct {
	Name        string // Service instance name ("" for the unnamed service)
	Follow      bool   // Tail/follow logs (-f)
	Lines       int    // Number of lines to show (-n)
	UserService bool   // User service vs system service
}

func (c *LogsConfig) service() *ServiceConfig {
	return &ServiceConfig{Name: c.Name, UserService: c.UserService}
}

// ViewLogs displays service logs using platform-appropriate commands.
func ViewLogs(cfg *LogsConfig) error {
	if runtime.GOOS == "darwin" {
		return viewLogsMacOS(cfg)
	}
	return viewLogsLinux(cfg)
}

// viewLogsLinux uses journalctl to view logs on Linux.
func viewLogsLinux(cfg *LogsConfig) error {
	args := []string{"-u", UnitName(cfg.Name)}

	if cfg.UserService {
		args = append(args, "--user")
	}

	if cfg.Follow {
		args = append(args, "-f")
	}

	if cfg.Lines > 0 {
		args = append(args, "-n", strconv.Itoa(cfg.Lines))
	} else if !cfg.Follow {
		// Default to 50 lines if not following
		args = append(args, "-n", "50")
	}

	cmd := exec.Command("journalctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// viewLogsMacOS uses tail to view logs on macOS.
func viewLogsMacOS(cfg *LogsConfig) error {
	logPath := cfg.service().LogPath()

	if logPath == "" {
		return fmt.Errorf("log path not configured")
	}

	// Check if log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("log file not found: %s\n\nThe service may not have started yet or logs are not configured", logPath)
	}

	args := []string{}

	if cfg.Follow {
		args = append(args, "-f")
	}

	if cfg.Lines > 0 {
		args = append(args, "-n", strconv.Itoa(cfg.Lines))
	} else if !cfg.Follow {
		// Default to 50 lines if not following
		args = append(args, "-n", "50")
	}

	args = append(args, logPath)

	cmd := exec.Command("tail", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// LogsHint is where to find the service's logs: a file on macOS, a
// journalctl command on Linux.
func LogsHint(cfg *ServiceConfig) string {
	if runtime.GOOS == "darwin" {
		return cfg.LogPath()
	}
	if cfg.UserService {
		return "journalctl --user -u " + cfg.UnitName()
	}
	return "journalctl -u " + cfg.UnitName()
}
