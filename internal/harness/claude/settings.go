package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/harness/claude/telemetry"
)

// Claude Code reads one --settings and takes the last one given, so what
// rewake needs for a launch — the end-of-turn hooks, the grant hooks, the
// telemetry hooks, the status-line tap — goes into a single layer. When the caller passed their own,
// ours is merged into theirs rather than replacing it or being dropped: their
// keys stay, their hooks run beside ours, and their status line is the one
// the tap runs.

type hookEntry struct {
	Kind    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
	// Async runs the hook in the background, so the session does not wait
	// for it (supported on 2.1.280, docs/research-launch.md). Only the telemetry hooks
	// take it: the end of a turn has to be recorded before the harness goes
	// idle, or a sender waiting on it could be woken for nothing.
	Async bool `json:"async,omitempty"`
}

type hookMatcher struct {
	// Matcher narrows a tool event to the tools it names; empty is every one.
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

// observeTimeout bounds a telemetry hook. It returns in milliseconds; this is
// the harness's ceiling for one that somehow does not.
const observeTimeout = 5

// grantHookTimeout bounds the grant hook. It holds a tool call while it runs,
// so it runs in the foreground: an answer that came after the call would be
// no answer. It asks its wrapper over a socket and returns in milliseconds; past the
// ceiling the harness goes on as if it had said nothing, which is the hook's
// own answer to anything it cannot decide.
const grantHookTimeout = 5

// grantPreToolUse are the tools whose calls the grant hook sees before they
// run: the file tools it denies inside a grant being taken back or sends to
// the person inside its shielded part, and the reads and commands it turns
// into the question that takes it back (permission.go). Every other tool is
// spared the process start.
const grantPreToolUse = "Write|Edit|MultiEdit|NotebookEdit|Read|Glob|Grep|LS|NotebookRead|Bash"

// launchLayer is what rewake adds for one launch.
type launchLayer struct {
	hooks  map[string][]hookMatcher
	status string // the tap's command line, or "" for no tap
}

// newLaunchLayer builds rewake's hooks and, when a telemetry socket is known,
// the telemetry hooks and the tap. rewakeRule says the launch adds the allow
// rule for rewake, which the grant hook is told (harness.GrantRewakeRule). sources and
// caller are what the tap needs to find the person's status line and cannot
// learn itself (statusline.go).
func newLaunchLayer(silent, rewakeRule bool, socket, sources string, caller json.RawMessage) (launchLayer, error) {
	executable, err := os.Executable()
	if err != nil {
		return launchLayer{}, fmt.Errorf("could not find the rewake binary: %w", err)
	}
	turnEnded := harness.ShellQuote([]string{executable, harness.TurnEnded})
	layer := launchLayer{hooks: map[string][]hookMatcher{}}
	add := func(event string, entry hookEntry) {
		layer.hooks[event] = append(layer.hooks[event], hookMatcher{Hooks: []hookEntry{entry}})
	}
	add(telemetry.StopFailure, hookEntry{Kind: "command", Command: turnEnded, Timeout: 10})
	if !silent {
		add(telemetry.Stop, hookEntry{Kind: "command", Command: turnEnded, Timeout: 10})
	}
	grantCommand := []string{executable, harness.GrantHook}
	if rewakeRule {
		grantCommand = append(grantCommand, "--"+harness.GrantRewakeRule)
	}
	grantHook := hookEntry{Kind: "command", Command: harness.ShellQuote(grantCommand), Timeout: grantHookTimeout}
	layer.hooks[preToolUse] = append(layer.hooks[preToolUse], hookMatcher{Matcher: grantPreToolUse, Hooks: []hookEntry{grantHook}})
	add(permissionRequest, grantHook)
	if socket == "" {
		return layer, nil
	}
	observe := harness.ShellQuote([]string{executable, harness.Observe, socket})
	for _, event := range telemetry.HookEvents {
		add(event, hookEntry{Kind: "command", Command: observe, Timeout: observeTimeout, Async: true})
	}
	tap := []string{executable, harness.StatusTap, socket, sources}
	if len(caller) > 0 && string(caller) != "null" {
		tap = append(tap, string(caller))
	}
	layer.status = harness.ShellQuote(tap)
	return layer, nil
}

// readCallerLayer reads the value of a caller's --settings the way the harness
// does: a JSON object given inline, or the path of a file holding one.
func readCallerLayer(value, cwd string) (map[string]json.RawMessage, error) {
	raw := []byte(strings.TrimSpace(value))
	if !strings.HasPrefix(string(raw), "{") {
		path := value
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		read, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		raw = read
	}
	var layer map[string]json.RawMessage
	if err := json.Unmarshal(raw, &layer); err != nil {
		return nil, err
	}
	if layer == nil {
		return nil, errors.New("not a settings object")
	}
	return layer, nil
}

// merge puts rewake's layer into the caller's and encodes the result. The
// caller's hooks for an event come first and ours are appended; their status
// line keeps every field but the command, which becomes the tap.
func (l launchLayer) merge(caller map[string]json.RawMessage) (string, error) {
	merged := map[string]json.RawMessage{}
	for key, value := range caller {
		merged[key] = value
	}
	hooks := map[string]json.RawMessage{}
	if raw, ok := merged["hooks"]; ok && len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return "", fmt.Errorf("its hooks are not an object: %w", err)
		}
	}
	for event, ours := range l.hooks {
		var matchers []json.RawMessage
		if raw, ok := hooks[event]; ok && string(raw) != "null" {
			if err := json.Unmarshal(raw, &matchers); err != nil {
				return "", fmt.Errorf("its %s hooks are not a list: %w", event, err)
			}
		}
		for _, matcher := range ours {
			encoded, err := json.Marshal(matcher)
			if err != nil {
				return "", err
			}
			matchers = append(matchers, encoded)
		}
		encoded, err := json.Marshal(matchers)
		if err != nil {
			return "", err
		}
		hooks[event] = encoded
	}
	encodedHooks, err := json.Marshal(hooks)
	if err != nil {
		return "", err
	}
	merged["hooks"] = encodedHooks
	if l.status != "" {
		status := map[string]json.RawMessage{}
		if raw, ok := merged["statusLine"]; ok && string(raw) != "null" {
			if err := json.Unmarshal(raw, &status); err != nil {
				return "", fmt.Errorf("its statusLine is not an object: %w", err)
			}
		}
		status["type"] = json.RawMessage(`"command"`)
		command, err := json.Marshal(l.status)
		if err != nil {
			return "", err
		}
		status["command"] = command
		encodedStatus, err := json.Marshal(status)
		if err != nil {
			return "", err
		}
		merged["statusLine"] = encodedStatus
	}
	encoded, err := json.Marshal(merged)
	return string(encoded), err
}

// applySettings rewrites the launch arguments so they carry exactly one
// --settings: the caller's, with rewake's merged in. When the caller's cannot
// be read, it stays as they gave it and rewake adds nothing — a merge that
// guessed would change settings they chose — and the note says what is lost.
func applySettings(args []string, cwd string, silent, rewakeRule bool, socket string) ([]string, []string) {
	var notes []string
	caller := map[string]json.RawMessage{}
	if values := harness.FlagValues(args, settingsFlag); len(values) > 0 {
		read, err := readCallerLayer(values[len(values)-1], cwd)
		if err != nil {
			return args, []string{"not reporting the end of turns or collecting telemetry: the --settings given could not be read, and only one is read: " + err.Error()}
		}
		caller = read
	}
	if socket != "" && policyStatusLine() {
		notes = append(notes, "the managed policy sets the status line, so model, effort and context stay unknown for this session")
	}
	var callerStatus json.RawMessage
	if raw := caller["statusLine"]; len(raw) > 0 {
		// Compacted: it travels as one argument of the tap's command line.
		var compact bytes.Buffer
		if json.Compact(&compact, raw) == nil {
			callerStatus = compact.Bytes()
		}
	}
	layer, err := newLaunchLayer(silent, rewakeRule, socket, launchSources(args), callerStatus)
	if err != nil {
		return args, append(notes, "not reporting the end of turns: "+err.Error())
	}
	merged, err := layer.merge(caller)
	if err != nil {
		return args, append(notes, "not reporting the end of turns or collecting telemetry: the --settings given could not be merged: "+err.Error())
	}
	return harness.AddFlags(harness.WithoutFlag(args, settingsFlag), settingsFlag, merged), notes
}
