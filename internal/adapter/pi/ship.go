package pi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter"
)

// PinnedVersion is the pi installed with rewake (scripts/pack.sh reads it):
// the one version this build was checked with. NodeMinimum is that pi's own
// engines field, which it does not check itself.
const (
	PinnedVersion = "1.4.0"
	NodeMinimum   = "22.19.0"
	// Package is the npm package of pi, a dependency of rewake's own.
	Package = "@earendil-works/pi-coding-agent"
	// shippedEnv names a pi package directory or entry script in place of
	// the installed one, for a build of rewake that npm did not install.
	shippedEnv = "REWAKE_PI"
)

// Locate finds the pi shipped with rewake and the node that runs it. rewake
// starts the entry script with node itself rather than through a pi on PATH,
// which may be the person's own of another version; node runs it in its own
// process, which the extension's peer check needs.
func (harness) Locate() (adapter.Start, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return adapter.Start{}, fmt.Errorf("pi runs on Node.js %s or later, and no node is on PATH", NodeMinimum)
	}
	entry, err := shippedEntry()
	if err != nil {
		return adapter.Start{}, err
	}
	return adapter.Start{
		Command: node, Args: []string{entry},
		RuntimeTitle: "Node.js", Runtime: adapter.VersionGate{Minimum: NodeMinimum, Args: []string{"--version"}},
	}, nil
}

func shippedEntry() (string, error) {
	if named := os.Getenv(shippedEnv); named != "" {
		entry, err := entryOf(named)
		if err != nil {
			return "", fmt.Errorf("%s names no pi: %w", shippedEnv, err)
		}
		return entry, nil
	}
	executable, err := os.Executable()
	if err == nil {
		executable, err = filepath.EvalSymlinks(executable)
	}
	if err != nil {
		return "", fmt.Errorf("could not find rewake's own executable, beside which pi is installed: %w", err)
	}
	if entry, ok := findShipped(filepath.Dir(executable)); ok {
		return entry, nil
	}
	return "", fmt.Errorf("the pi installed with rewake is not in any node_modules above %s; reinstall rewake from npm, set %s to a pi %s package, or name a pi with --command",
		executable, shippedEnv, PinnedVersion)
}

// findShipped looks for pi's package in node_modules from dir up to the root,
// as node resolves a dependency: npm puts it beside the platform package that
// holds rewake's executable, or in the entry package's own node_modules.
func findShipped(dir string) (string, bool) {
	for {
		if entry, err := entryOf(filepath.Join(dir, "node_modules", Package)); err == nil {
			return entry, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// entryOf is the script pi starts from, given its package's directory, whose
// manifest names it, or the script itself.
func entryOf(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return path, nil
	}
	data, err := os.ReadFile(filepath.Join(path, "package.json"))
	if err != nil {
		return "", err
	}
	var manifest struct {
		Bin json.RawMessage `json:"bin"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("%s: %w", filepath.Join(path, "package.json"), err)
	}
	var script string
	var scripts map[string]string
	switch {
	case json.Unmarshal(manifest.Bin, &script) == nil:
	case json.Unmarshal(manifest.Bin, &scripts) == nil:
		script = scripts["pi"]
	}
	if script == "" {
		return "", errors.New(filepath.Join(path, "package.json") + " names no pi command")
	}
	entry := filepath.Join(path, script)
	if _, err := os.Stat(entry); err != nil {
		return "", err
	}
	return entry, nil
}

// agentDirPath is the directory pi keeps its login, settings, extensions,
// packages and sessions in for every pi rewake starts: rewake's own, beside
// its alias file, so the person's global pi is neither read nor changed. It
// is the person's, not a room's: it outlives the state directory, which may
// be cleared with the system's temporary files.
func agentDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("pi's directory is in the home directory: %w", err)
	}
	return filepath.Join(home, ".config", "rewake", "pi", "agent"), nil
}

// agentDir makes that directory for a launch, private since it holds the
// login: one made before, by the person or by pi, is made private too. It
// also gets the two folders a person adds to, extensions/ and skills/, so
// the layout is visible, and the workers' sessions folder, private before pi
// would make it; what is already there is left as it is.
func agentDir() (string, error) {
	dir, err := agentDirPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("could not make pi's directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("could not make pi's directory private: %w", err)
	}
	for _, name := range []string{"extensions", "skills", workerSessions} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
			return "", fmt.Errorf("could not make pi's %s directory: %w", name, err)
		}
	}
	return dir, nil
}
