package service

import (
	"html"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// Installed is a hookly service found in the service manager's config.
type Installed struct {
	Name       string // Instance name ("" for the unnamed "hookly" service)
	Unit       string // Service manager name: "hookly" or "hookly-<name>"
	ConfigPath string // hookly.yaml the service runs, from its --config argument
}

// unitDir is where the service manager keeps hookly's unit files, and the
// extension they use.
func unitDir(userService bool) (dir, ext string) {
	switch runtime.GOOS {
	case "darwin":
		if !userService {
			return "/Library/LaunchDaemons", ".plist"
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", ""
		}
		return filepath.Join(home, "Library", "LaunchAgents"), ".plist"
	case "linux":
		if !userService {
			return "/etc/systemd/system", ".service"
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", ""
		}
		return filepath.Join(home, ".config", "systemd", "user"), ".service"
	}
	return "", ""
}

// ListInstalled returns the hookly services installed for this user (or
// system-wide), sorted by unit name. Services are found by their unit files:
// "hookly" and every "hookly-<name>".
func ListInstalled(userService bool) ([]Installed, error) {
	dir, ext := unitDir(userService)
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var found []Installed
	for _, e := range entries {
		unit, ok := strings.CutSuffix(e.Name(), ext)
		if !ok || e.IsDir() {
			continue
		}
		name, named := strings.CutPrefix(unit, serviceName+"-")
		if unit != serviceName && (!named || ValidateName(name) != nil) {
			continue
		}
		if !named {
			name = ""
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		found = append(found, Installed{
			Name:       name,
			Unit:       unit,
			ConfigPath: argAfter(unitArgs(string(data), ext), "--config"),
		})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Unit < found[j].Unit })
	return found, nil
}

var (
	plistString = regexp.MustCompile(`<string>([^<]*)</string>`)
	quotedArg   = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
)

// unitArgs extracts the command-line arguments kardianos/service wrote into a
// unit file: ProgramArguments strings in a plist, quoted ExecStart arguments
// in a systemd unit.
func unitArgs(unit, ext string) []string {
	var args []string
	if ext == ".plist" {
		for _, m := range plistString.FindAllStringSubmatch(unit, -1) {
			args = append(args, html.UnescapeString(m[1]))
		}
		return args
	}
	for _, line := range strings.Split(unit, "\n") {
		if rest, ok := strings.CutPrefix(line, "ExecStart="); ok {
			for _, m := range quotedArg.FindAllStringSubmatch(rest, -1) {
				args = append(args, strings.ReplaceAll(m[1], `\"`, `"`))
			}
		}
	}
	return args
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
