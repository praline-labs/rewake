// Package codex owns a session-local server and connects its terminal client.
package codex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/brief"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// ID is the launch command and the harness field of a session record.
const ID = "codex"

// configFlag layers a configuration value over the user's file for one launch.
const configFlag = "-c"

// introKey carries the briefing. Codex has no key that appends to the user's
// instructions, so this one replaces them and the wrapper concatenates.
const introKey = "developer_instructions"

type codexHarness struct{}

// New returns the Codex harness.
func New() harness.Harness { return codexHarness{} }

func (codexHarness) ID() string    { return ID }
func (codexHarness) Title() string { return "Codex" }

func (codexHarness) Summary() string {
	return "Start Codex with a session-owned server for immediate delivery."
}

func (codexHarness) Examples() []string {
	return []string{
		"rewake codex",
		"rewake --name web codex --model gpt-5.6-terra",
	}
}

func (codexHarness) Notes() []string {
	return []string{
		"A private app-server starts or steers a turn when a notice arrives; no queue polling is needed.",
		"The server lives only for this session. Existing --remote, --profile, --worktree and --oss/--local-provider arguments require a separate checkout or explicit configuration instead.",
		"Arguments after the harness name are passed to codex untouched, with one exception: a --help written first asks rewake for this page instead of starting the harness.",
	}
}

func (codexHarness) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	args := append([]string{}, request.Args...)
	home := Home()
	var notes []string

	// A profile, or a setting the caller passed themselves, is a layer this
	// adapter cannot see into, and every value it passes replaces one.
	layered := hasProfile(args)

	if request.Intro && !hasConfigKey(args, introKey) {
		if layered {
			notes = append(notes, "not adding the rewake briefing: a profile is selected and its instructions cannot be seen from here")
		} else if mentioned, why := configMentions(home, introKey); mentioned {
			notes = append(notes, "not adding the rewake briefing: "+why+", and passing the briefing would replace the user's instructions. Run rewake guide in the session instead")
		} else {
			args = harness.AddFlags(args, configFlag, introKey+"="+quoteTOML(brief.Intro(request.BriefContext())))
		}
	}

	continuation, permissionOverride := continuationOptions(request.Args)
	if continuation && permissionOverride {
		notes = append(notes, "permission overrides cannot be used with resume/fork under rewake: the remote TUI rejects them; remove the permission flags or start a new thread; caller arguments are unchanged")
	}
	if continuation && request.Role.GitWrite {
		notes = append(notes, "resumed thread gets Git metadata access with each rewake task; turns you start yourself use the thread's stored roots")
	}
	if request.Role.GitWrite && !continuation {
		if flags, note := gitWriteFlags(args); note != "" {
			notes = append(notes, note)
		} else {
			args = harness.AddFlags(args, flags...)
		}
	}

	// The state directory uses the caller's existing temporary-directory
	// permissions. Granting Git writes changes neither tmp nor network policy.
	if note := tmpNote(home, request.Dir, args, layered); note != "" {
		notes = append(notes, note)
	}

	if harness.HasFlag(request.Args, "--remote") || hasProfile(request.Args) || harness.HasFlag(request.Args, "--worktree") || harness.HasFlag(request.Args, "--oss") || harness.HasFlag(request.Args, "--local-provider") {
		return harness.LaunchPlan{}, fmt.Errorf("session-owned app-server requires local arguments without --remote, --profile, --worktree or --oss/--local-provider; select a checkout and configuration explicitly before launching")
	}
	cwd, err := gitWorkingDirectory(request.Args)
	if err != nil {
		return harness.LaunchPlan{}, err
	}
	socket := request.Socket
	if socket == "" {
		socket = filepath.Join(request.Dir, "server.sock")
	}
	env := harness.SessionEnv(request, nil)
	serverArgs := serverConfigArgs(args)
	serverArgs = append(serverArgs, "app-server", "--listen", "unix://"+socket)
	args = harness.AddFlags(args, "--remote", "unix://"+socket)
	server := newServer(socket, serverArgs, append(append([]string{}, env...), "CODEX_INTERNAL_APP_SERVER_REMOTE_CONTROL_DISABLED=1"), cwd)
	server.gitWrite = request.Role.GitWrite
	return harness.LaunchPlan{
		Backend:    server,
		Socket:     socket,
		OwnsSocket: true,
		Command:    "codex",
		Args:       args,
		Env:        env,
		CodexHome:  home,
		Notes:      notes,
	}, nil
}

func (codexHarness) Deliver(_ context.Context, _ registry.Session, _ inbox.Message) inbox.Result {
	return inbox.Result{State: inbox.Failed, Detail: "this session needs an owned app-server; restart it through rewake"}
}

// Home is where Codex keeps its state, and where delivery has to look.
func Home() string {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".codex"
	}
	return filepath.Join(home, ".codex")
}

// rootsKey is the sandbox setting that lists what the agent may write to.
const rootsKey = sandboxSection + ".writable_roots"

// excludeTmpKey says whether /tmp has been taken out of the sandbox's reach.
const excludeTmpKey = "exclude_slash_tmp"

// tmpNote says how to let the agent write its messages when /tmp may be out of
// the sandbox's reach, and nothing when it is not.
func tmpNote(home, dir string, args []string, layered bool) string {
	advice := "; if the agent cannot send messages, add " + dir + " to " + rootsKey
	switch {
	case hasConfigKey(args, rootsKey) || hasConfigKey(args, sandboxSection+"."+excludeTmpKey):
		// The caller set the sandbox up themselves and knows what it allows.
		return ""
	case layered:
		return "a profile is selected, so whether /tmp is writable cannot be seen from here" + advice
	}
	if mentioned, why := configMentions(home, excludeTmpKey); mentioned {
		return why + ", so /tmp may be out of the sandbox's reach" + advice
	}
	return ""
}

// hasConfigKey reports whether the caller already set a configuration key, in
// either spelling of the flag. Anything after "--" is input for the harness,
// not a flag.
func hasConfigKey(args []string, key string) bool {
	visible := harness.BeforeTerminator(args)
	for index, arg := range visible {
		// Both spellings, and both shapes: "-c key=value" and "--config=key=value"
		// are the same instruction, and missing one of them means overriding a
		// setting the caller had already made.
		switch {
		case arg == configFlag || arg == "--config":
			if index+1 < len(visible) && strings.HasPrefix(visible[index+1], key+"=") {
				return true
			}
		case strings.HasPrefix(arg, "--config="):
			if strings.HasPrefix(strings.TrimPrefix(arg, "--config="), key+"=") {
				return true
			}
		case strings.HasPrefix(arg, "-c") && len(arg) > 2:
			// Both "-ckey=value" and "-c=key=value": the CLI takes either, and
			// missing one of them means overriding a setting the caller made.
			if strings.HasPrefix(strings.TrimPrefix(arg[2:], "="), key+"=") {
				return true
			}
		}
	}
	return false
}

// hasProfile reports whether the caller selected a configuration profile. Its
// values are a layer this adapter cannot read, so overriding on top of one would
// replace something it never saw.
func hasProfile(args []string) bool {
	for _, arg := range harness.BeforeTerminator(args) {
		// Including the joined short form: "-pwork" selects a profile as surely
		// as "-p work" does.
		if arg == "-p" || arg == "--profile" || strings.HasPrefix(arg, "--profile=") ||
			(strings.HasPrefix(arg, "-p") && len(arg) > 2) {
			return true
		}
	}
	return false
}

// quoteTOML renders a string as a TOML basic string.
//
// Not strconv.Quote: Go escapes a control character as \xNN, which TOML does not
// define. Codex then fails to parse the value and falls back to taking the
// argument as a raw string, so the agent receives quotes and escapes instead of
// its instructions.
func quoteTOML(value string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, symbol := range value {
		switch symbol {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if symbol < 0x20 || symbol == 0x7f {
				_, _ = fmt.Fprintf(&out, `\u%04X`, symbol)
				continue
			}
			out.WriteRune(symbol)
		}
	}
	out.WriteByte('"')
	return out.String()
}
