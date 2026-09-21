package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"

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
			"    'hookly login'. Run it again after moving hookly.yaml or upgrading hookly.\n\n" +
			"    Each hookly.yaml gets its own service, named after its directory: running it\n" +
			"    in ~/homeboy installs hookly-homeboy. Install one per directory to run several\n" +
			"    relays on one machine, each for different endpoints.",
		Action: runInstall,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config",
				Usage: "Path to hookly.yaml",
				Value: "hookly.yaml",
			},
			&cli.StringFlag{
				Name:  "name",
				Usage: "Service name (default: the hookly.yaml's directory name)",
			},
		},
	}
}

// uninstallCommand removes a user service installed by `hookly install`.
func uninstallCommand() *cli.Command {
	return &cli.Command{
		Name:  "uninstall",
		Usage: "Stop and remove the hookly user service for this directory",
		Description: "Removes the service for ./hookly.yaml, or the one named by --name.\n" +
			"    When only one hookly service is installed, removes that one.",
		Action: runUninstall,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "name",
				Usage: "Service name (see 'hookly service list --user')",
			},
		},
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

	name := c.String("name")
	if name == "" {
		name = svc.DefaultName(configPath)
	}
	if err := svc.ValidateName(name); err != nil {
		return err
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
	cfg.Name = name
	cfg.ConfigPath = configPath
	cfg.WorkingDir = filepath.Dir(configPath)
	if err := cfg.ValidateForInstall(); err != nil {
		return err
	}

	installed, err := svc.ListInstalled(true)
	if err != nil {
		return fmt.Errorf("list installed services: %w", err)
	}
	if err := checkInstallConflicts(cfg, hooklyCfg, installed); err != nil {
		return err
	}

	// Replace this service if it is already there (new path, new binary), and
	// any other service for the same hookly.yaml - e.g. the unnamed "hookly"
	// service from older versions - so two relays never claim its endpoints.
	for _, inst := range installed {
		if inst.Unit != cfg.UnitName() && !sameFile(inst.ConfigPath, configPath) {
			continue
		}
		if inst.Unit == cfg.UnitName() {
			fmt.Printf("Existing %s service found - replacing it\n", inst.Unit)
		} else {
			fmt.Printf("Replacing %s, which runs the same hookly.yaml\n", inst.Unit)
		}
		old := &svc.ServiceConfig{Name: inst.Name, ConfigPath: inst.ConfigPath, UserService: true}
		_ = svc.ControlService(old, "stop")
		if err := svc.ControlService(old, "uninstall"); err != nil && !isNotInstalledError(err) {
			return fmt.Errorf("remove existing service %s: %w", inst.Unit, err)
		}
	}

	if err := svc.ControlService(cfg, "install"); err != nil {
		return fmt.Errorf("install service: %w", err)
	}
	if err := svc.ControlService(cfg, "start"); err != nil {
		return fmt.Errorf("service installed but failed to start: %w", err)
	}

	hubID := hooklyCfg.HubID
	if hubID == "" {
		hubID = config.HostHubID() + "-" + name
	}

	fmt.Printf("hookly is installed as a user service and running\n\n")
	fmt.Printf("  Service:   %s\n", cfg.UnitName())
	fmt.Printf("  Config:    %s\n", configPath)
	fmt.Printf("  Runs as:   %s\n", creds.Username)
	fmt.Printf("  Edge:      %s\n", hooklyCfg.EdgeURL)
	fmt.Printf("  Hub ID:    %s\n", hubID)
	fmt.Printf("  Endpoints: %d\n\n", len(hooklyCfg.Endpoints))

	if runtime.GOOS == "linux" {
		// Without lingering, systemd only runs user services while you are logged in
		if err := enableLinger(); err != nil {
			fmt.Printf("  To keep it running after you log out and start it at boot, run once:\n")
			fmt.Printf("    sudo loginctl enable-linger %s\n\n", currentUsername())
		} else {
			fmt.Printf("  Starts at boot and keeps running after you log out (lingering enabled).\n\n")
		}
		fmt.Printf("  Status:  systemctl --user status %s\n", cfg.UnitName())
	} else {
		fmt.Printf("  Starts when you log in and restarts if it stops.\n\n")
		fmt.Printf("  Status:  hookly service status --user --name %s\n", name)
	}
	fmt.Printf("  Logs:    %s\n", svc.LogsHint(cfg))
	fmt.Printf("  All:     hookly service list --user\n")
	fmt.Printf("  Remove:  hookly uninstall --name %s\n", name)
	return nil
}

// checkInstallConflicts refuses installs the edge could not serve: taking a
// name another hookly.yaml already uses, or a second service relaying an
// endpoint that another installed service already relays (the edge sends
// each endpoint to one relay only).
func checkInstallConflicts(cfg *svc.ServiceConfig, hooklyCfg *config.HooklyConfig, installed []svc.Installed) error {
	mine := make(map[string]bool)
	for _, id := range hooklyCfg.EndpointIDs() {
		mine[id] = true
	}
	for _, inst := range installed {
		if sameFile(inst.ConfigPath, cfg.ConfigPath) {
			continue // replaced below
		}
		if inst.Unit == cfg.UnitName() {
			return fmt.Errorf("service %s already runs %s\n\nPick another name with --name, or remove it first: hookly uninstall --name %s",
				inst.Unit, inst.ConfigPath, inst.Name)
		}
		other, err := config.LoadHooklyYAML(inst.ConfigPath)
		if err != nil {
			continue // its relay can't start either, so it claims nothing
		}
		for _, id := range other.EndpointIDs() {
			if mine[id] {
				return fmt.Errorf("endpoint %s is already relayed by service %s (%s)\n\n"+
					"The edge sends each endpoint to one relay. To deliver it to more services,\n"+
					"add them as destinations of that endpoint and list them in that hookly.yaml.",
					id, inst.Unit, inst.ConfigPath)
			}
		}
	}
	return nil
}

func runUninstall(c *cli.Context) error {
	target, err := uninstallTarget(c.String("name"))
	if err != nil {
		return err
	}
	if target == nil {
		fmt.Println("No hookly service is installed")
		return nil
	}

	cfg := &svc.ServiceConfig{Name: target.Name, ConfigPath: target.ConfigPath, UserService: true}
	_ = svc.ControlService(cfg, "stop")
	if err := svc.ControlService(cfg, "uninstall"); err != nil {
		if isNotInstalledError(err) {
			fmt.Printf("%s is not installed\n", cfg.UnitName())
			return nil
		}
		return fmt.Errorf("uninstall service: %w", err)
	}
	fmt.Printf("%s stopped and removed\n", cfg.UnitName())
	return nil
}

// uninstallTarget picks the service `hookly uninstall` removes: the one
// named, else the one for ./hookly.yaml, else the only one installed.
func uninstallTarget(name string) (*svc.Installed, error) {
	installed, err := svc.ListInstalled(true)
	if err != nil {
		return nil, fmt.Errorf("list installed services: %w", err)
	}

	if name != "" {
		if err := svc.ValidateName(name); err != nil {
			return nil, err
		}
		for i := range installed {
			if installed[i].Name == name {
				return &installed[i], nil
			}
		}
		return nil, fmt.Errorf("no hookly service named %s\n\nSee: hookly service list --user", name)
	}

	if here, err := filepath.Abs("hookly.yaml"); err == nil {
		for i := range installed {
			if sameFile(installed[i].ConfigPath, here) {
				return &installed[i], nil
			}
		}
	}

	switch len(installed) {
	case 0:
		return nil, nil
	case 1:
		return &installed[0], nil
	}
	msg := "several hookly services are installed - choose one with --name:\n"
	for _, inst := range installed {
		label := inst.Name
		if label == "" {
			label = `""  (the unnamed service; use 'hookly service uninstall --user')`
		}
		msg += fmt.Sprintf("\n  %-20s %s", label, inst.ConfigPath)
	}
	return nil, errors.New(msg)
}

// sameFile reports whether a and b name the same file, following symlinks.
func sameFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	ai, err1 := os.Stat(a)
	bi, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(ai, bi)
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
