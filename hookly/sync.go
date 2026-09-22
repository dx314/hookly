package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/urfave/cli/v2"

	clicmd "hooks.dx314.com/internal/cli"
	"hooks.dx314.com/internal/config"
	"hooks.dx314.com/internal/provision"
)

// syncCommand makes the edge match hookly.yaml without starting the relay.
func syncCommand() *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "Create and update endpoints and destinations on the edge from hookly.yaml",
		Description: "Makes the edge match hookly.yaml: creates endpoints and destinations that don't\n" +
			"    exist, updates the ones that differ, and writes the ids and webhook URLs the edge\n" +
			"    assigns back into hookly.yaml. The relay (and its service) does this on start too.",
		Action: runSync,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config",
				Usage: "Path to hookly.yaml",
				Value: "hookly.yaml",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would change without changing anything",
			},
		},
	}
}

func runSync(c *cli.Context) error {
	configPath, err := filepath.Abs(c.String("config"))
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	cfg, err := config.LoadHooklyYAML(configPath)
	if err != nil {
		return fmt.Errorf("load %s: %w", configPath, err)
	}
	return syncConfig(c.Context, cfg, configPath, c.Bool("dry-run"))
}

// syncConfig syncs cfg once with the saved login and prints what happened.
func syncConfig(ctx context.Context, cfg *config.HooklyConfig, configPath string, dryRun bool) error {
	credsMgr, err := clicmd.NewCredentialsManager()
	if err != nil {
		return fmt.Errorf("init credentials manager: %w", err)
	}
	creds, err := credsMgr.Load()
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}
	if creds == nil {
		return errors.New("not logged in\n\nRun 'hookly login' first")
	}
	cfg.Token = creds.APIToken

	report, err := provision.Apply(ctx, cfg, configPath, provision.Options{DryRun: dryRun})
	for _, ch := range report.Changes {
		fmt.Printf("  %s\n", ch)
	}
	for _, f := range report.Fields {
		if !dryRun {
			fmt.Printf("  recorded %s of endpoint %d in %s\n", f.Key, f.Endpoint, filepath.Base(configPath))
		}
	}
	for _, n := range report.Notes {
		fmt.Printf("  note: %s\n", n)
	}
	if err != nil {
		return fmt.Errorf("sync %s: %w", filepath.Base(configPath), err)
	}
	if len(report.Changes) == 0 {
		fmt.Println("Edge matches hookly.yaml")
	}
	return nil
}
