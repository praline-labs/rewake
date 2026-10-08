//go:build rewakefixture

// Package fixture is the harness the core is proven on without a real one: a
// program the workflow suite provides, every capability of which a test can
// withhold (docs/v2/stage3-fixture.md). It exists only in a build with the
// rewakefixture tag, which the suite uses and a release never does.
package fixture

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/brief"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
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
func New() harness.Harness { return fixtureHarness{} }

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
func (fixtureHarness) SingleUseFlags() []harness.Flag {
	return []harness.Flag{
		{Spellings: []string{"--session"}, TakesValue: true},
		{Spellings: []string{"--room"}, TakesValue: true},
		{Spellings: []string{"--role"}, TakesValue: true},
		{Spellings: []string{"--brief"}, TakesValue: true},
		{Spellings: []string{"--connect"}, TakesValue: true},
		// rewake's own (worktree.go), read only with =<name>, so a switch
		// here: a typed one replaces an alias's.
		{Spellings: []string{worktreeFlag}},
	}
}

func (fixtureHarness) ProtectedDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(home, configDir)}
}

// ownFlags are the program's flags a caller may not pass: the adapter passes
// each one itself.
var ownFlags = []string{"--session", "--room", "--role", "--brief", "--connect"}

func (fixtureHarness) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	for index, arg := range request.Args {
		if arg == "--" {
			break
		}
		name, _, _ := strings.Cut(arg, "=")
		for _, own := range ownFlags {
			if name == own {
				// By position, never by value: the argument may carry what
				// its writer would not have printed (L2).
				return harness.LaunchPlan{}, fmt.Errorf("argument %d after fixture is one of the flags rewake passes the fixture itself; remove it", index+1)
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
	terminal := append(append(append([]string{}, description...), "--"), request.Args...)
	env := harness.SessionEnv(request, nil)
	cwd, _ := os.Getwd()
	return harness.LaunchPlan{
		Backend:    newBackend(request.Program(ID), server, env, cwd, socket, request.Epoch),
		Socket:     socket,
		OwnsSocket: true,
		Command:    request.Program(ID),
		Args:       terminal,
		Env:        env,
		Notes:      notes,
	}, nil
}

// SupportsDirGrant: a directory granted with a task reaches the program with
// the letter that carries it, once the wrapper has checked it again and main's
// wrapper has confirmed it, and the program says it was offered one. Applying
// it to what the program may write, and taking it back, is the Permissions
// capability stage 6 designs (docs/v2/design-api.md#permissions); here the
// fixture holds the core's part of a grant on the gate column.
func (fixtureHarness) SupportsDirGrant() bool { return true }

// ReachesWrapper: the program runs its commands without a sandbox of its own,
// so a fixture main's send reaches its wrapper and can grant.
func (fixtureHarness) ReachesWrapper() bool { return true }

func room(request harness.LaunchRequest) string {
	if request.Room == "" {
		return "default"
	}
	return request.Room
}

// Deliver is the path for a session without the fixture's backend, which a
// fixture session never is.
func (fixtureHarness) Deliver(context.Context, registry.Session, inbox.Message) inbox.Result {
	return inbox.Result{State: inbox.Failed, Detail: "this session needs the fixture's backend; restart it through rewake"}
}

// ReadLaunchVersion reads the program's version before the claim and refuses
// a launch below the minimum or without a version.
func (fixtureHarness) ReadLaunchVersion(program string, env []string, dir string) (harness.Version, error) {
	version, err := harness.ReadVersion(program, env, dir)
	if err != nil {
		return version, err
	}
	return version, harness.RequireVersion("the fixture", version, MinimumVersion)
}

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
