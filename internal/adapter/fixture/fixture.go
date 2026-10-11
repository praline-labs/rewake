//go:build rewakefixture

// Package fixture is the harness the core is proven on without a real one: a
// program the workflow suite provides, every capability of which a test can
// withhold (docs/v2/stage3-fixture.md). It exists only in a build with the
// rewakefixture tag, which the suite uses and a release never does.
package fixture

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/brief"
)

// ID is the launch command, the program's name and the session record's
// harness.
const ID = "fixture"

// MinimumVersion is the oldest program the adapter starts.
const MinimumVersion = "1.0.0"

// configDir is where the program keeps its configuration, under the home.
const configDir = ".fixture"

type fixtureHarness struct{}

// New returns the fixture harness.
func New() adapter.Harness { return fixtureHarness{} }

func (fixtureHarness) ID() string    { return ID }
func (fixtureHarness) Title() string { return "Fixture harness (tests only)" }

func (fixtureHarness) Summary() string {
	return "Start the test fixture as a rewake session; built only for the workflow suite."
}

func (fixtureHarness) Examples() []string {
	return []string{"rewake fixture", "rewake --name api fixture"}
}

func (fixtureHarness) Notes() []string {
	return []string{
		"The fixture is a program the workflow suite provides: it proves the core without a real harness, and a test can withhold each thing it serves.",
		"Arguments after fixture reach its terminal as written, after a --; its own flags are rewake's to pass and are refused here.",
		"--worktree=<name> starts the session in a new checkout of HEAD on a new branch of that name under rewake's worktree directory, at the same place within the repository; --worktree alone names it.",
	}
}

// SingleUseFlags are the program's own flags, each of which it parses once.
func (fixtureHarness) SingleUseFlags() []adapter.Flag {
	return []adapter.Flag{
		{Spellings: []string{"--session"}, TakesValue: true},
		{Spellings: []string{"--room"}, TakesValue: true},
		{Spellings: []string{"--role"}, TakesValue: true},
		{Spellings: []string{"--brief"}, TakesValue: true},
		{Spellings: []string{"--connect"}, TakesValue: true},
		{Spellings: []string{"--control"}, TakesValue: true},
		// rewake's own (worktree.go), read only with =<name>, so a switch
		// here: a typed one replaces an alias's.
		{Spellings: []string{worktreeFlag}},
	}
}

// ProtectedDirs: the program keeps its configuration under the home and
// nothing inside a project.
func (fixtureHarness) ProtectedDirs() adapter.Protected {
	home, err := os.UserHomeDir()
	if err != nil {
		return adapter.Protected{}
	}
	return adapter.Protected{Dirs: []string{filepath.Join(home, configDir)}}
}

// ownFlags are the program's flags a caller may not pass: the adapter passes
// each one itself.
var ownFlags = []string{"--session", "--room", "--role", "--brief", "--connect", "--control"}

func (fixtureHarness) Launch(request adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	for index, arg := range request.Args {
		if arg == "--" {
			break
		}
		name, _, _ := strings.Cut(arg, "=")
		for _, own := range ownFlags {
			if name == own {
				// By position, never by value: the argument may carry what
				// its writer would not have printed (L2).
				return adapter.LaunchPlan{}, fmt.Errorf("argument %d after fixture is one of the flags rewake passes the fixture itself; remove it", index+1)
			}
		}
	}
	settings, notes := readConfig()
	description := []string{"--session", request.Name, "--room", room(request), "--role", request.Role.ID}
	server := append([]string{}, description...)
	if request.Intro {
		if settings.brief {
			notes = append(notes, "not adding the rewake briefing: the fixture's configuration in "+settings.path+" sets one")
		} else {
			server = append(server, "--brief", brief.Intro(request.BriefContext()))
		}
	}
	socket := request.Socket
	if socket == "" {
		socket = filepath.Join(request.Dir, "fixture.sock")
	}
	server = append(server, "--connect", socket)
	if request.ControlDir != "" {
		server = append(server, "--control", request.ControlDir)
	}
	terminal := append(append(append([]string{}, description...), "--"), request.Args...)
	env := request.Env
	cwd, _ := os.Getwd()
	run := newBackend(request.Program(ID), server, env, cwd, socket, request.Epoch)
	return adapter.LaunchPlan{
		Service: run, Wake: run, TurnBoundary: run, Telemetry: run, Thread: run,
		Socket:     socket,
		OwnsSocket: true,
		Command:    request.Program(ID),
		Args:       terminal,
		Env:        env,
		Notes:      notes,
	}, nil
}

// Offers: what the program can serve — every capability a test can withhold
// — and the core's part of a directory grant.
//
// Permissions.Dir: a directory granted with a task reaches the program with
// the letter that carries it, once the wrapper has checked it again and main's
// wrapper has confirmed it, and the program says it was offered one. Applying
// it to what the program may write, and taking it back, is the Permissions
// capability stage 6 designs (docs/v2/design-api.md#permissions); here the
// fixture holds the core's part of a grant on the gate column. ReachesWrapper:
// the program runs its commands without a sandbox of its own, so a fixture
// main's send reaches its wrapper and can grant.
//
// Control: the fixture offers it while no product adapter does: Claude Code's
// served side left with its plugin module in S9, and stage 4 gives a real
// adapter Control again (docs/v2/design-api.md#control). Until then the
// program serves the run's control directory itself, as the module did, so
// rewake compact and rewake interrupt keep their workflow cases on the gate
// column; S11 moves the served side behind the fixture's live Control
// (control.Serve). It takes a focus with a compaction, and ends an
// interrupted turn with the interrupter's name as its text, which is what the
// stopped report carries.
func (fixtureHarness) Offers() adapter.Offer {
	return adapter.Offer{
		Wake:          true,
		ToolTransport: Transport,
		TurnBoundary:  adapter.TurnBoundaryOffer{Reports: true, HoldsEnd: true},
		Telemetry:     true,
		Control: adapter.ControlOffer{
			Compact: true, Interrupt: true, CompactFocus: true,
			InterruptTrace: "the fixture ends the turn saying who interrupted it",
		},
		Permissions: adapter.PermissionsOffer{Dir: true, ReachesWrapper: true},
	}
}

func room(request adapter.LaunchRequest) string {
	if request.Room == "" {
		return "default"
	}
	return request.Room
}

// Version: the program answers --version with its name and its version,
// "fixture 1.0.0".
func (fixtureHarness) Version() adapter.VersionGate {
	return adapter.VersionGate{Minimum: MinimumVersion, Args: []string{"--version"}}
}

// EnvNotInherited: the program exports nothing to its children.
func (fixtureHarness) EnvNotInherited() []string { return nil }

// settings is what the adapter reads of the program's configuration.
type settings struct {
	path  string
	brief bool
}

// readConfig reads <home>/.fixture/config, one "key = value" a line. Its
// notes name the file and the line, never what the line holds (L2).
func readConfig() (settings, []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return settings{}, nil
	}
	found := settings{path: filepath.Join(home, configDir, "config")}
	raw, err := os.ReadFile(found.path)
	if errors.Is(err, os.ErrNotExist) {
		return found, nil
	}
	if err != nil {
		return found, []string{"the fixture's configuration in " + found.path + " could not be read; launching without it"}
	}
	var notes []string
	for index, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, _, ok := strings.Cut(line, "=")
		switch {
		case ok && strings.TrimSpace(key) == "brief":
			found.brief = true
		default:
			notes = append(notes, fmt.Sprintf("line %d of %s is not a setting the fixture reads; ignored", index+1, found.path))
		}
	}
	return found, notes
}
