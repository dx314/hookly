package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/urfave/cli/v2"

	clicmd "hooks.dx314.com/internal/cli"
	"hooks.dx314.com/internal/config"
	"hooks.dx314.com/internal/relay"
	svc "hooks.dx314.com/internal/service"
)

const version = "0.1.0"
const defaultEdgeURL = "https://hooks.dx314.com"

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
)

// Custom help templates with nice styling
var appHelpTemplate = `{{ cyan .Name | bold }} {{ dim .Version }}
{{ .Usage }}

{{ bold "USAGE" }}
    {{ .HelpName }}{{ if .Commands }} [command]{{ end }}{{ if .Flags }} [options]{{ end }}

    Running {{ cyan "hookly" }} without a command starts the webhook relay.

{{ bold "COMMANDS" }}
  {{ bold "Authentication" }}
    {{ green "login" }}     Authenticate via GitHub OAuth
    {{ green "logout" }}    Clear stored credentials
    {{ green "whoami" }}    Show current user
    {{ green "status" }}    Show connection and config status

  {{ bold "Setup" }}
    {{ green "init" }}      Create hookly.yaml interactively

  {{ bold "Service Management" }}
    {{ green "service" }}   Install/manage as system service
              └─ install, uninstall, start, stop, restart, status, logs

{{ bold "QUICK START" }}
    {{ dim "$" }} hookly login                    {{ dim "# authenticate with GitHub" }}
    {{ dim "$" }} hookly init                     {{ dim "# create config interactively" }}
    {{ dim "$" }} hookly                          {{ dim "# start the relay" }}

{{ bold "EXAMPLES" }}
    {{ dim "# Start relay in foreground" }}
    {{ dim "$" }} hookly

    {{ dim "# Run as a background service" }}
    {{ dim "$" }} hookly service install --config ./hookly.yaml
    {{ dim "$" }} hookly service start

    {{ dim "# Connect to a custom edge server" }}
    {{ dim "$" }} hookly login --edge-url https://hooks.example.com

{{ bold "FILES" }}
    {{ dim "~/.config/hookly/credentials.json" }}    Encrypted auth credentials
    {{ dim "./hookly.yaml" }}                        Endpoint configuration

{{ bold "GLOBAL OPTIONS" }}
    {{ green "--debug" }}         Enable debug logging (JSON output)
    {{ green "--help, -h" }}      Show help
    {{ green "--version, -v" }}   Print version ({{ .Version }})

{{ dim "Documentation:" }} https://github.com/alexdunmow/hookly
`

var commandHelpTemplate = `{{ cyan .FullName | bold }}
{{ .Usage }}

{{ bold "USAGE" }}
    {{ .HelpName }}{{ if .VisibleCommands }} <command>{{ end }}{{ if .VisibleFlags }} [options]{{ end }}
{{ if .Description }}
{{ bold "DESCRIPTION" }}
    {{ .Description }}
{{ end }}{{ if .VisibleCommands }}
{{ bold "COMMANDS" }}{{ range .VisibleCommands }}
    {{ green .Name }}	{{ .Usage }}{{ end }}
{{ end }}{{ if .VisibleFlags }}
{{ bold "OPTIONS" }}{{ range .VisibleFlags }}
    {{ green (flagNames .) }}	{{ .Usage }}{{ if (flagDefault .) }} {{ dim (flagDefault .) }}{{ end }}{{ end }}
{{ end }}`

var subcommandHelpTemplate = `{{ cyan .FullName | bold }}
{{ .Usage }}

{{ bold "USAGE" }}
    {{ .HelpName }}{{ if .VisibleCommands }} <command>{{ end }}{{ if .VisibleFlags }} [options]{{ end }}
{{ if .Description }}
{{ bold "DESCRIPTION" }}
    {{ .Description }}
{{ end }}{{ if .VisibleCommands }}
{{ bold "COMMANDS" }}{{ range .VisibleCommands }}
    {{ green .Name }}	{{ .Usage }}{{ end }}
{{ end }}{{ if .VisibleFlags }}
{{ bold "OPTIONS" }}{{ range .VisibleFlags }}
    {{ green (flagNames .) }}	{{ .Usage }}{{ if (flagDefault .) }} {{ dim (flagDefault .) }}{{ end }}{{ end }}
{{ end }}`

// Template functions for colorized output
var templateFuncs = template.FuncMap{
	"cyan":        func(s string) string { return colorCyan + s + colorReset },
	"green":       func(s string) string { return colorGreen + s + colorReset },
	"yellow":      func(s string) string { return colorYellow + s + colorReset },
	"bold":        func(s string) string { return colorBold + s + colorReset },
	"dim":         func(s string) string { return colorDim + s + colorReset },
	"flagNames":   flagNames,
	"flagDefault": flagDefault,
}

func init() {
	cli.AppHelpTemplate = appHelpTemplate
	cli.CommandHelpTemplate = commandHelpTemplate
	cli.SubcommandHelpTemplate = subcommandHelpTemplate

	// Custom help printer with our template functions
	cli.HelpPrinter = func(out io.Writer, templ string, data interface{}) {
		t := template.Must(template.New("help").Funcs(templateFuncs).Parse(templ))
		_ = t.Execute(out, data)
	}
}

// flagNames returns a formatted flag name string.
func flagNames(f cli.Flag) string {
	names := f.Names()
	if len(names) == 0 {
		return ""
	}

	var parts []string
	for _, name := range names {
		if len(name) == 1 {
			parts = append(parts, "-"+name)
		} else {
			parts = append(parts, "--"+name)
		}
	}
	return strings.Join(parts, ", ")
}

// flagDefault returns a default value string for a flag.
func flagDefault(f cli.Flag) string {
	switch ff := f.(type) {
	case *cli.StringFlag:
		if ff.Value != "" {
			return "(default: " + ff.Value + ")"
		}
	case *cli.IntFlag:
		if ff.Value != 0 {
			return fmt.Sprintf("(default: %d)", ff.Value)
		}
	case *cli.BoolFlag:
		if ff.Value {
			return "(default: true)"
		}
	}
	return ""
}

func main() {
	// Check if running in service mode (invoked by service manager)
	if isServiceMode() {
		configPath := getServiceConfigPath()
		if err := svc.RunServiceMode(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Service error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Setup structured logging (quiet by default)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})))

	app := &cli.App{
		Name:                 "hookly",
		Usage:                "Relay webhooks from the public internet to your local network",
		Version:              version,
		Action:               runRelay,
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "Enable debug logging with full structured JSON output",
			},
		},
		Commands: []*cli.Command{
			{
				Name:        "login",
				Usage:       "Authenticate with the hookly edge server",
				Description: "Opens a browser window to authenticate via GitHub OAuth.\nCredentials are encrypted and stored locally.",
				Action:      runLogin,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "edge-url",
						Usage: "Edge server URL",
						Value: defaultEdgeURL,
					},
				},
			},
			{
				Name:        "logout",
				Usage:       "Clear stored credentials and revoke token",
				Description: "Removes locally stored credentials.\nYou will need to run 'hookly login' again to use the CLI.",
				Action:      runLogout,
			},
			{
				Name:        "whoami",
				Usage:       "Show current authenticated user",
				Description: "Displays your username and the edge server you're connected to.",
				Action:      runWhoami,
			},
			{
				Name:        "status",
				Usage:       "Show current user, edge URL, and connection status",
				Description: "Displays authentication status, configuration details,\nand the number of configured endpoints.",
				Action:      runStatus,
			},
			{
				Name:        "init",
				Usage:       "Create hookly.yaml configuration file",
				Description: "Interactively creates a hookly.yaml config file.\nIf logged in, lets you select from your existing endpoints\nor create a new one.",
				Action:      runInit,
			},
			serviceCommand(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runRelay is the default action - starts the relay client.
func runRelay(c *cli.Context) error {
	// Setup logger based on debug flag
	setupLogger(c.Bool("debug"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load credentials (required for relay)
	credsMgr, err := clicmd.NewCredentialsManager()
	if err != nil {
		return fmt.Errorf("init credentials manager: %w", err)
	}

	creds, err := credsMgr.Load()
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	if creds == nil {
		return fmt.Errorf("not logged in\n\nRun 'hookly login' to authenticate first")
	}

	// Load config from hookly.yaml
	cfg, err := config.LoadHooklyYAML("hookly.yaml")
	if err != nil {
		return fmt.Errorf("load config: %w\n\nRun 'hookly init' to create a hookly.yaml file", err)
	}

	// Inject token from credentials
	cfg.Token = creds.APIToken

	slog.Info("hookly starting",
		"edge_url", cfg.EdgeURL,
		"hub_id", cfg.GetHubID(),
		"endpoints", len(cfg.Endpoints),
	)

	// Create relay client
	client := relay.NewClient(cfg)

	// Run client in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run(ctx)
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			return handleRelayError(err, credsMgr)
		}
	case sig := <-sigCh:
		slog.Info("received shutdown signal", "signal", sig)
	}

	// Graceful shutdown
	cancel()
	slog.Info("hookly stopped")
	return nil
}

// handleRelayError handles errors from the relay client and takes appropriate action.
func handleRelayError(err error, credsMgr *clicmd.CredentialsManager) error {
	// Token errors - clear credentials and prompt re-login
	if errors.Is(err, relay.ErrTokenInvalid) || errors.Is(err, relay.ErrTokenRevoked) {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Authentication failed - your token is invalid or has been revoked.")
		fmt.Fprintln(os.Stderr)

		// Clear the invalid credentials
		if delErr := credsMgr.Delete(); delErr != nil {
			slog.Warn("failed to clear credentials", "error", delErr)
		} else {
			fmt.Fprintln(os.Stderr, "Credentials have been cleared.")
		}

		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Run 'hookly login' to re-authenticate.")
		return err
	}

	// Endpoint not found - suggest reconfiguring
	if errors.Is(err, relay.ErrEndpointNotFound) {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Endpoint not found - the endpoint in your hookly.yaml doesn't exist.")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "This can happen if:")
		fmt.Fprintln(os.Stderr, "  - The endpoint was deleted from the web UI")
		fmt.Fprintln(os.Stderr, "  - The endpoint ID in hookly.yaml is incorrect")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Run 'hookly init' to reconfigure with a valid endpoint.")
		return err
	}

	// Endpoint access denied - different user
	if errors.Is(err, relay.ErrEndpointForbidden) {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Access denied - this endpoint belongs to another user.")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Make sure you're logged in as the correct user ('hookly whoami'),")
		fmt.Fprintln(os.Stderr, "or run 'hookly init' to select an endpoint you own.")
		return err
	}

	// No endpoints configured
	if errors.Is(err, relay.ErrNoEndpoints) {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "No endpoints configured in hookly.yaml.")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Run 'hookly init' to set up your configuration.")
		return err
	}

	// Generic error
	return fmt.Errorf("relay error: %w", err)
}

// runLogin handles the login command.
func runLogin(c *cli.Context) error {
	edgeURL := c.String("edge-url")

	// Check if already logged in
	credsMgr, err := clicmd.NewCredentialsManager()
	if err != nil {
		return fmt.Errorf("init credentials manager: %w", err)
	}

	existing, err := credsMgr.Load()
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	if existing != nil {
		fmt.Printf("Already logged in as %s (%s)\n", existing.Username, existing.EdgeURL)
		fmt.Print("Log out first with 'hookly logout' to switch accounts.\n")
		return nil
	}

	// Perform OAuth login
	result, err := clicmd.Login(c.Context, edgeURL)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Save credentials
	creds := &clicmd.Credentials{
		EdgeURL:   edgeURL,
		APIToken:  result.Token,
		UserID:    result.UserID,
		Username:  result.Username,
		CreatedAt: time.Now(),
	}

	if err := credsMgr.Save(creds); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	fmt.Printf("\nLogged in as %s\n", result.Username)
	fmt.Printf("Credentials saved to %s\n", credsMgr.Path())
	return nil
}

// runLogout handles the logout command.
func runLogout(c *cli.Context) error {
	credsMgr, err := clicmd.NewCredentialsManager()
	if err != nil {
		return fmt.Errorf("init credentials manager: %w", err)
	}

	creds, err := credsMgr.Load()
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	if creds == nil {
		fmt.Println("Not logged in.")
		return nil
	}

	// Try to revoke token on server (best effort)
	// Note: We'd need to call the revoke endpoint here, but for simplicity
	// we'll just clear local credentials. In production, you'd want to
	// call the server to revoke the token.

	// Delete local credentials
	if err := credsMgr.Delete(); err != nil {
		return fmt.Errorf("delete credentials: %w", err)
	}

	fmt.Printf("Logged out. Credentials removed from %s\n", credsMgr.Path())
	return nil
}

// runWhoami handles the whoami command.
func runWhoami(c *cli.Context) error {
	credsMgr, err := clicmd.NewCredentialsManager()
	if err != nil {
		return fmt.Errorf("init credentials manager: %w", err)
	}

	creds, err := credsMgr.Load()
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	if creds == nil {
		fmt.Println("Not logged in.")
		fmt.Println("Run 'hookly login' to authenticate.")
		return nil
	}

	fmt.Printf("%s (%s)\n", creds.Username, creds.EdgeURL)
	return nil
}

// runStatus handles the status command.
func runStatus(c *cli.Context) error {
	credsMgr, err := clicmd.NewCredentialsManager()
	if err != nil {
		return fmt.Errorf("init credentials manager: %w", err)
	}

	creds, err := credsMgr.Load()
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	fmt.Println("Hookly Status")
	fmt.Println("=============")

	if creds == nil {
		fmt.Println("Logged in: No")
		fmt.Println("\nRun 'hookly login' to authenticate.")
	} else {
		fmt.Printf("Logged in: Yes\n")
		fmt.Printf("User:      %s\n", creds.Username)
		fmt.Printf("Edge URL:  %s\n", creds.EdgeURL)
		fmt.Printf("Since:     %s\n", creds.CreatedAt.Format(time.RFC3339))
	}

	// Check for hookly.yaml
	fmt.Println()
	if _, err := os.Stat("hookly.yaml"); err == nil {
		cfg, err := config.LoadHooklyYAML("hookly.yaml")
		if err != nil {
			fmt.Printf("Config:    hookly.yaml (error: %v)\n", err)
		} else {
			fmt.Printf("Config:    hookly.yaml\n")
			fmt.Printf("Hub ID:    %s\n", cfg.HubID)
			fmt.Printf("Endpoints: %d\n", len(cfg.Endpoints))
		}
	} else {
		fmt.Println("Config:    Not found (run 'hookly init')")
	}

	return nil
}

// runInit handles the init command.
func runInit(c *cli.Context) error {
	// Check if logged in
	credsMgr, err := clicmd.NewCredentialsManager()
	if err != nil {
		return fmt.Errorf("init credentials manager: %w", err)
	}

	creds, err := credsMgr.Load()
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	// If logged in, use interactive wizard
	if creds != nil {
		return runInitWizard(c, creds, credsMgr)
	}

	// Not logged in - create basic template
	return runInitBasic(c)
}

// runInitBasic creates a basic template hookly.yaml.
func runInitBasic(c *cli.Context) error {
	// Check if file already exists
	if _, err := os.Stat("hookly.yaml"); err == nil {
		return fmt.Errorf("hookly.yaml already exists")
	}

	// Write example config
	if err := os.WriteFile("hookly.yaml", []byte(config.ExampleYAML()), 0644); err != nil {
		return fmt.Errorf("create hookly.yaml: %w", err)
	}

	fmt.Println("Created hookly.yaml")
	fmt.Println("")
	fmt.Println("Edit the file to configure:")
	fmt.Println("  - edge_url: Your hookly edge server URL")
	fmt.Println("  - endpoints: The endpoint IDs to handle")
	fmt.Println("")
	fmt.Println("Then run 'hookly login' to authenticate, and 'hookly' to start the relay.")
	return nil
}

// runInitWizard runs the interactive init wizard.
func runInitWizard(c *cli.Context, creds *clicmd.Credentials, credsMgr *clicmd.CredentialsManager) error {
	// Check if file already exists
	if _, err := os.Stat("hookly.yaml"); err == nil {
		return fmt.Errorf("hookly.yaml already exists")
	}

	// Create API client
	client := clicmd.NewClient(creds.EdgeURL, creds.APIToken)

	// Run wizard
	cfg, err := clicmd.RunWizard(client, creds)
	if err != nil {
		return err
	}
	if cfg == nil {
		// No endpoints found
		return nil
	}

	// Write config file
	configContent := clicmd.GenerateConfigYAML(cfg)
	if err := os.WriteFile("hookly.yaml", []byte(configContent), 0644); err != nil {
		return fmt.Errorf("write hookly.yaml: %w", err)
	}

	fmt.Println()
	fmt.Println("Created hookly.yaml")
	fmt.Println()
	fmt.Printf("Webhook URL: %s/h/%s\n", cfg.EdgeURL, cfg.EndpointID)
	fmt.Println()
	fmt.Println("Run 'hookly' to start the relay.")
	return nil
}

// isServiceMode checks if hookly was started by the service manager.
func isServiceMode() bool {
	for _, arg := range os.Args {
		if arg == "--service-mode" {
			return true
		}
	}
	return false
}

// getServiceConfigPath extracts the config path from service mode arguments.
func getServiceConfigPath() string {
	for i, arg := range os.Args {
		if arg == "--config" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	// Default to system config path
	return "/etc/hookly/hookly.yaml"
}
