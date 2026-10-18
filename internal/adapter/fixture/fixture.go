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
	"github.com/praline-labs/rewake/internal/core/brief"
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
		{Spellings: []string{"--connect"}, TakesValue: true},
		// rewake's own (worktree.go), read only with =<name>, so a switch
		// here: a typed one replaces an alias's.
		{Spellings: []string{worktreeFlag}},
	}
}

// ownFlags are the program's flags a caller may not pass: the adapter passes
// each one itself.
var ownFlags = []string{"--session", "--room", "--role", "--connect"}

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
	if request.Conversation != "" {
		return adapter.LaunchPlan{}, errors.New("the fixture takes no conversation id; a launch of it names none")
	}
	settings, notes := readConfig()
	description := []string{"--session", request.Name, "--room", room(request), "--role", request.Role.ID}
	server := append([]string{}, description...)
	socket := request.Socket
	if socket == "" {
		socket = filepath.Join(request.Dir, "fixture.sock")
	}
	server = append(server, "--connect", socket)
	terminal := append(append(append([]string{}, description...), "--"), request.Args...)
	env := request.Env
	cwd, _ := os.Getwd()
	run := newBackend(request.Program(ID), server, env, cwd, socket, request.Epoch)
	if request.Intro {
		if settings.brief {
			notes = append(notes, "not adding the rewake briefing: the fixture's configuration in "+settings.path+" sets one")
		} else {
			// Built once the exchange shows whether the tool is live, which
			// decides the transport sentence.
			context := request.BriefContext()
			run.brief = func(tool bool) string {
				context.Tool = tool
				return brief.Intro(context)
			}
		}
	}
	// The run serves its control directory itself, carrying each request to
	// the program while Control is live (control.go).
	run.controlDir = request.ControlDir
	return adapter.LaunchPlan{
		Service: run, Wake: run, TurnBoundary: run, Telemetry: run, Thread: run, Live: run,
		Socket:     socket,
		OwnsSocket: true,
		Command:    request.Program(ID),
		Args:       terminal,
		Env:        env,
		Notes:      notes,
	}, nil
}

// Offers: what the program can serve — every capability a test can withhold.
//
// Control: the fixture offers it while no product adapter does: Claude Code's
// served side left with its plugin module in S9, and stage 4 gives a real
// adapter Control again (docs/v2/design-api.md#control). The run serves its
// control directory itself (control.Serve) and carries each request to the
// program while Control is live on the connection, so rewake compact and
// rewake interrupt keep their workflow cases on the gate column. The program
// takes a focus with a compaction, and ends an interrupted turn with the
// interrupter's name as its text, which is what the stopped report carries.
//
// Spaces: none, said here rather than left to the zero value. Nothing in
// rewake hosts a space before S4-7 builds the host's neutral seam, and an
// offer no run could prove would be a capability the CLI admits and the run
// cannot serve; S4-7 gives the fixture its spaces through that seam, never
// through another adapter's mod. Conversation: the program takes no id, so a
// launch that names one is refused.
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
		Spaces: false,
		Conversation: adapter.ConversationOffer{
			Unnamed: "the fixture keeps one conversation per run and takes no id for it",
		},
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
