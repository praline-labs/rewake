// Package codex owns a session-local server and connects its terminal client.
package codex

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

// ID is the launch command and the harness field of a session record.
const ID = "codex"

// configFlag layers a configuration value over the user's file for one launch.
const configFlag = "-c"

// introKey carries the briefing. Codex has no key that appends to the user's
// instructions, so this one replaces them and the wrapper concatenates.
const introKey = "developer_instructions"

type codexHarness struct{}

var _ harness.Steerable = codexHarness{}

// New returns the Codex harness.
func New() harness.Harness { return codexHarness{} }

func (codexHarness) ID() string    { return ID }
func (codexHarness) Title() string { return "Codex" }

func (codexHarness) Summary() string {
	return "Start Codex as a rewake session, with its own server for delivery."
}

func (codexHarness) Examples() []string {
	return []string{
		"rewake codex",
		"rewake --name web codex --search",
	}
}

// codexDefaults are the launch settings rewake may take from the environment.
//
// Every way a person can state either setting for one launch, checked against
// the installed 0.155.1 and its argument parser rather than from memory:
//
//   - model: --model or -m, written apart, with '=', or joined ("-mname");
//     and the configuration key `model` through -c/--config in any of its
//     spellings, with whitespace around the key allowed.
//   - reasoning effort: the configuration key `model_reasoning_effort` only.
//     There is no flag for it; saying so is more useful than inventing one.
//
// Deliberately not consulted, each for a reason:
//
//   - ~/.codex/config.toml, and anything it includes. rewake does not read or
//     edit a person's configuration (AGENTS.md), and a flag it adds would win
//     over that file. Somebody who sets a model there and also sets the
//     environment variable gets the variable; the variable is the thing they
//     set for rewake specifically.
//   - a profile (-p/--profile): a launch with one is refused outright before
//     any of this, because its values are a layer this adapter cannot read.
//   - --oss and --local-provider: refused outright for the same reason.
//
// The reasoning effort is a configuration key rather than a flag here, which
// is why it is applied with -c.
func codexDefaults() []harness.Default {
	return []harness.Default{
		{
			Env:  "REWAKE_CODEX_MODEL",
			What: "model",
			Present: func(args []string) bool {
				// Every way a person can choose a model for this launch:
				// the flag in any spelling, including the joined short form
				// "-mmodel" — Codex takes it, and a second --model then makes
				// it refuse the arguments, so the session never starts — and
				// the configuration key, which is the same choice said
				// another way.
				return len(harness.FlagValues(args, "--model", "-m")) > 0 || hasConfigKey(args, modelKey)
			},
			Apply: func(args []string, value string) []string {
				return harness.AddFlags(args, "--model", value)
			},
		},
		{
			Env:  "REWAKE_CODEX_EFFORT",
			What: "reasoning effort",
			Present: func(args []string) bool {
				return hasConfigKey(args, reasoningKey)
			},
			Apply: func(args []string, value string) []string {
				return harness.AddFlags(args, "-c", reasoningKey+"="+quoteTOML(value))
			},
		},
	}
}

// The configuration keys Codex reads these settings from. Either can also be
// written as a flag, except the reasoning effort, which has no flag.
const (
	reasoningKey = "model_reasoning_effort"
	modelKey     = "model"
)

func (codexHarness) Notes() []string {
	return []string{
		"A private app-server, living as long as the session, starts or steers a turn when a notice arrives.",
		"--worktree is rewake's here, since the terminal refuses its own beside the server: it starts the session in a new checkout of HEAD on a new branch under rewake's worktree directory, at the same place within the repository; --worktree=<name> names both. rewake worktree --help says how to land and remove it. It starts a new conversation only: resume and fork are refused beside it, since they continue in the conversation's own directory.",
		"Arguments after codex reach it as written, beside the --remote rewake adds for its server, except: a --help first asks rewake for this page; beside --worktree, -C or --cd only says where the checkout is made from; --remote, --profile or -p, --oss and --local-provider are refused, since the session's own server needs local arguments; and a typed flag replaces an alias's copy of it.",
	}
}

func (codexHarness) SupportsGitGrant() bool { return true }
func (codexHarness) SupportsDirGrant() bool { return true }

// CompactFocus: thread/compact/start has no field for one, and the only key
// that shapes the summary replaces its whole prompt for the whole
// conversation, so a focus is refused before anything is sent (owner decision,
// docs/remote-control.md).
func (codexHarness) CompactFocus() bool { return false }

// InterruptTrace: Codex records the interrupt in the conversation itself, so
// rewake adds no line (owner decision, docs/remote-control.md).
func (codexHarness) InterruptTrace() string {
	return "Codex records the interrupt in its model's history"
}

// SingleUseFlags are the flags Codex takes at most once, each naming one
// parameter by every spelling it has.
//
// Read from `codex --help` on 0.155.1 and checked by running it: a repeated
// flag from this list ends the launch with a parse error rather than taking
// the last value. That is what makes replacement necessary here rather than
// merely tidy.
//
// Deliberately absent, and they must stay absent: --image, --add-dir, --enable
// and --disable accumulate — a second --add-dir adds a second directory — and
// -c, which is left alone for a narrower reason. A repeated scalar key does
// take its last value, but structured settings merge instead, so dropping an
// earlier -c can remove a part of a setting that the later one does not
// restate. Sorting one case from the other means understanding the value, and
// the harness already does that correctly.
//
// Two things this cannot express, and should not try to: --enable X with
// --disable X is not "the last one wins" (every enable is applied, then every
// disable), and flags that conflict under different names — an approval policy
// against a sandbox mode — are not found by comparing names at all.
func (codexHarness) SingleUseFlags() []harness.Flag {
	return []harness.Flag{
		{Spellings: []string{"--model", "-m"}, TakesValue: true},
		{Spellings: []string{"--cd", "-C"}, TakesValue: true},
		{Spellings: []string{"--sandbox", "-s"}, TakesValue: true},
		{Spellings: []string{"--ask-for-approval", "-a"}, TakesValue: true},
		{Spellings: []string{"--profile", "-p"}, TakesValue: true},
		{Spellings: []string{"--local-provider"}, TakesValue: true},
		{Spellings: []string{"--remote"}, TakesValue: true},
		{Spellings: []string{"--remote-auth-token-env"}, TakesValue: true},
		// Switches. They carry nothing, and saying so is what keeps a dropped
		// one from taking the argument standing behind it.
		{Spellings: []string{"--search"}},
		{Spellings: []string{"--oss"}},
		{Spellings: []string{"--worktree"}},
		{Spellings: []string{"--strict-config"}},
		{Spellings: []string{"--no-alt-screen"}},
		// --not-so-yolo is this switch's other name, declared beside the long
		// one in the reference tree and confirmed by running 0.155.1: the two
		// together are refused as one argument used twice. Neither alias
		// appears in the help.
		{Spellings: []string{"--approve-for-me", "--not-so-yolo"}},
		// --yolo is the same switch under another name: passing both is
		// refused as the argument used twice (verified live on 0.155.1).
		{Spellings: []string{"--dangerously-bypass-approvals-and-sandbox", "--yolo"}},
		{Spellings: []string{"--dangerously-bypass-hook-trust"}},
	}
}

func (codexHarness) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	args := append([]string{}, request.Args...)
	home := Home()
	var notes []string
	if request.Command != "" {
		// A wrapper may set its own CODEX_HOME, which rewake cannot see: the
		// briefing, the notes and the recorded home all come from this one.
		notes = append(notes, "rewake reads Codex configuration from "+home+"; if your wrapper sets CODEX_HOME, export it for rewake too")
	}
	generatedBrief := false

	// A profile, or a setting the caller passed themselves, is a layer this
	// adapter cannot see into, and every value it passes replaces one.
	layered := hasProfile(args)

	if request.Intro && !hasConfigKey(args, introKey) {
		if layered {
			notes = append(notes, "not adding the rewake briefing: a profile is selected and its instructions cannot be seen from here")
		} else if mentioned, why := configMentions(home, introKey); mentioned {
			notes = append(notes, "not adding the rewake briefing: "+why+", and passing the briefing would replace the user's instructions. Run rewake guide in the session instead")
		} else {
			generatedBrief = true
			args = harness.AddFlags(args, configFlag, introKey+"="+quoteTOML(brief.Intro(request.BriefContext())+mailboxBriefing))
		}
	}

	if !generatedBrief {
		notes = append(notes, "native mailbox notices use rewake_mailbox_notice tool output. The generated briefing was not added; your session instructions must direct the agent to read rewake inbox on these notices and finish tasks with a final reply; see docs/native-mailbox.md")
	}

	continuation, permissionOverride := continuationOptions(request.Args)
	if continuation && permissionOverride {
		notes = append(notes, "permission overrides cannot be used with resume/fork under rewake: the remote TUI rejects them; remove the permission flags or start a new thread; caller arguments are unchanged")
	}
	// The state directory uses the caller's existing temporary-directory
	// permissions. Role selection never extends filesystem access.
	if note := tmpNote(home, request.Dir, args, layered); note != "" {
		notes = append(notes, note)
	}

	if harness.HasFlag(request.Args, "--remote") || hasProfile(request.Args) || harness.HasFlag(request.Args, "--oss") || harness.HasFlag(request.Args, "--local-provider") {
		return harness.LaunchPlan{}, errors.New(localArgumentsRequired)
	}
	if harness.HasFlag(request.Args, worktreeFlag) {
		// The launch command takes the flag before a launch is planned; one
		// reaching here came some other way, and the terminal would refuse it
		// beside --remote.
		return harness.LaunchPlan{}, fmt.Errorf("%s reached the Codex launch; start it with rewake codex %s so rewake makes the checkout", worktreeFlag, worktreeFlag)
	}
	cwd, err := gitWorkingDirectory(request.Args)
	if err != nil {
		return harness.LaunchPlan{}, err
	}
	// Before the server arguments are derived, on purpose: serverConfigArgs
	// forwards -c settings to the app-server, so a reasoning effort added here
	// reaches both halves and they agree. Moving this call below it would
	// leave the server on a different setting, silently. The model is a flag
	// rather than a setting, so it reaches the interface only — which is the
	// adapter's existing behavior for every model flag, ours or the
	// caller's.
	args, defaultNotes := harness.ApplyDefaults(args, codexDefaults())
	notes = append(notes, defaultNotes...)

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
	server.legacyLandlock = legacyLandlock(args, home)
	server.mailbox, server.name, server.stateRoot = request.RoomDir, request.Name, request.Dir
	server.controlDir = request.ControlDir
	// The owned server is started with the same program as the terminal: a
	// person's wrapper sets the environment both halves need — a Codex home,
	// credentials — and a server started around it would run in another one.
	server.program = request.Program("codex")
	mode, _ := continuationMode(request.Args)
	server.startupFork = mode == "fork"
	return harness.LaunchPlan{
		Backend:    server,
		Socket:     socket,
		OwnsSocket: true,
		Command:    request.Program("codex"),
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
	// One parser for the spellings of -c/--config, and this only has to know
	// what a setting looks like. Three copies of the spelling rules is how one
	// of them ends up missing a form.
	//
	// The key is taken up to the first '=' and trimmed, which is what the CLI
	// itself does (utils/cli/src/config_override.rs at e29eceb75, splitn(2,
	// '=') with trim on both halves). So `-c 'model = "x"'` sets the same key
	// as `-c model="x"`, and treating them differently would let a default
	// override a choice already made.
	for _, setting := range harness.FlagValues(args, configFlag, "--config") {
		name, _, found := strings.Cut(setting, "=")
		if found && strings.TrimSpace(name) == key {
			return true
		}
	}
	return false
}

// hasProfile reports whether the caller selected a configuration profile. Its
// values are a layer this adapter cannot read, so overriding on top of one would
// replace something it never saw.
func hasProfile(args []string) bool {
	return len(harness.FlagValues(args, "--profile", "-p")) > 0
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

// ProtectedDirs: Codex keeps its configuration, credentials and sessions in
// its home, CODEX_HOME or ~/.codex.
func (codexHarness) ProtectedDirs() []string { return []string{Home()} }
