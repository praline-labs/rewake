package claude

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/role"
)

// world is a private HOME and project directory, so the layers read are the
// test's and never the person's running it.
type world struct {
	home, project string
}

func newWorld(t *testing.T) world {
	t.Helper()
	w := world{home: t.TempDir(), project: t.TempDir()}
	t.Setenv("HOME", w.home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLAUDE_CODE_USE_COWORK_PLUGINS", "")
	t.Chdir(w.project)
	previous := managedDir
	managedDir = filepath.Join(t.TempDir(), "managed")
	t.Cleanup(func() { managedDir = previous })
	return w
}

func writeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func statusLine(command string) string {
	encoded, _ := json.Marshal(command)
	return `{"statusLine":{"type":"command","command":` + string(encoded) + `}}`
}

// launchObserved launches with a telemetry socket, as the wrapper does.
func launchObserved(t *testing.T, args []string, silent bool) harness.LaunchPlan {
	t.Helper()
	request := harness.LaunchRequest{Name: "api", Dir: t.TempDir(), Args: args, ObservationSocket: "/tmp/rw/api.1.obs"}
	if silent {
		request.Role = role.Main
	}
	plan, err := New().Launch(request)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	return plan
}

type layerView struct {
	Keys       map[string]json.RawMessage
	Hooks      map[string][]hookMatcher
	StatusLine map[string]json.RawMessage
}

// onlySettings is the one --settings a launch carries, decoded.
func onlySettings(t *testing.T, plan harness.LaunchPlan) layerView {
	t.Helper()
	values := harness.FlagValues(plan.Args, settingsFlag)
	if len(values) != 1 {
		t.Fatalf("args carry %d --settings, want one: %v", len(values), plan.Args)
	}
	var view layerView
	if err := json.Unmarshal([]byte(values[0]), &view.Keys); err != nil {
		t.Fatalf("the layer is not JSON: %v", err)
	}
	_ = json.Unmarshal(view.Keys["hooks"], &view.Hooks)
	_ = json.Unmarshal(view.Keys["statusLine"], &view.StatusLine)
	return view
}

// tapArgv is the argv the status line's command line stands for, read back
// through a shell the way the harness runs it.
func tapArgv(t *testing.T, view layerView) []string {
	t.Helper()
	var command string
	if err := json.Unmarshal(view.StatusLine["command"], &command); err != nil {
		t.Fatalf("no status command: %s", view.StatusLine["command"])
	}
	out, err := exec.Command("sh", "-c", `set -- `+command+`; for a; do printf '%s\0' "$a"; done`).Output()
	if err != nil {
		t.Fatalf("the status command does not parse: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
}

// Every telemetry hook and the tap are in the one layer, the telemetry hooks
// in the background and the end-of-turn hooks not.
func TestTelemetryIsInTheLaunchLayer(t *testing.T) {
	newWorld(t)
	view := onlySettings(t, launchObserved(t, nil, false))
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreCompact", "PostCompact", "Notification", "SessionEnd"} {
		matchers := view.Hooks[event]
		if len(matchers) != 1 || !matchers[0].Hooks[0].Async || !strings.Contains(matchers[0].Hooks[0].Command, "'observe' '/tmp/rw/api.1.obs'") {
			t.Errorf("%s hooks = %+v, want one background observe", event, matchers)
		}
	}
	for _, event := range []string{"Stop", "StopFailure"} {
		var turnEnded, observe int
		for _, matcher := range view.Hooks[event] {
			hook := matcher.Hooks[0]
			switch {
			case strings.HasSuffix(hook.Command, "'turn-ended'") && !hook.Async:
				turnEnded++
			case strings.Contains(hook.Command, "'observe'") && hook.Async:
				observe++
			}
		}
		if turnEnded != 1 || observe != 1 {
			t.Errorf("%s hooks = %+v, want turn-ended in the foreground and observe in the background", event, view.Hooks[event])
		}
	}
	argv := tapArgv(t, view)
	if len(argv) != 4 || argv[1] != "status-tap" || argv[2] != "/tmp/rw/api.1.obs" || argv[3] != "user,project,local" {
		t.Errorf("tap argv = %q, want status-tap, the socket and the sources", argv)
	}
	if _, err := os.Stat(argv[0]); err != nil {
		t.Errorf("the tap names no executable: %v", err)
	}
}

// A silent role still reports telemetry; only its turn-ended Stop hook goes.
func TestASilentRoleStillObserves(t *testing.T) {
	newWorld(t)
	view := onlySettings(t, launchObserved(t, nil, true))
	for _, matcher := range view.Hooks["Stop"] {
		if strings.HasSuffix(matcher.Hooks[0].Command, "'turn-ended'") {
			t.Error("a silent role reports the end of its turns")
		}
	}
	if len(view.Hooks["Stop"]) != 1 || view.StatusLine == nil {
		t.Errorf("hooks %v status %v, want the telemetry", view.Hooks["Stop"], view.StatusLine)
	}
}

// The tap is told at launch only what it cannot learn when it runs: which
// settings files the launch reads, and the caller's own status line. The
// person's command is found by the tap itself (telemetry.ResolveStatusCommand).
func TestTheTapIsToldWhatOnlyTheLaunchKnows(t *testing.T) {
	w := newWorld(t)
	writeJSON(t, filepath.Join(w.home, ".claude", "settings.json"), statusLine("user-line"))
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"every file", nil, []string{"user,project,local"}},
		{"restricted", []string{"--restricted"}, []string{"none"}},
		{"named sources", []string{"--setting-sources", "user, local"}, []string{"user,local"}},
		{"empty sources", []string{"--setting-sources="}, []string{"none"}},
		{
			"a caller's status line",
			[]string{"--settings", `{"statusLine": {"type": "command", "command": "it's theirs"}}`},
			[]string{"user,project,local", `{"type":"command","command":"it's theirs"}`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			argv := tapArgv(t, onlySettings(t, launchObserved(t, tc.args, false)))
			if strings.Join(argv[3:], "|") != strings.Join(tc.want, "|") {
				t.Errorf("tap arguments %q, want %q", argv[3:], tc.want)
			}
			if strings.Contains(strings.Join(argv, " "), "user-line") {
				t.Error("the launch resolved the person's status line; the tap must, at call time")
			}
		})
	}
}

// The caller's --settings is merged into, not replaced and not skipped: their
// keys stay, their hooks come first, their status line keeps its fields.
func TestTheCallersSettingsAreMergedInto(t *testing.T) {
	w := newWorld(t)
	caller := `{"model":"x","hooks":{"Stop":[{"hooks":[{"type":"command","command":"mine"}]}],"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"guard"}]}]},` +
		`"statusLine":{"type":"command","command":"their-line","padding":4}}`
	file := filepath.Join(w.project, "team.json")
	writeJSON(t, file, caller)
	for name, value := range map[string]string{"inline": caller, "file": "team.json"} {
		t.Run(name, func(t *testing.T) {
			view := onlySettings(t, launchObserved(t, []string{"--settings", value}, false))
			if string(view.Keys["model"]) != `"x"` {
				t.Errorf("their model key is gone: %s", view.Keys["model"])
			}
			if stop := view.Hooks["Stop"]; len(stop) != 3 || stop[0].Hooks[0].Command != "mine" {
				t.Errorf("Stop = %+v, want theirs first and both of ours", stop)
			}
			if pre := view.Hooks["PreToolUse"]; len(pre) != 2 || pre[0].Hooks[0].Command != "guard" {
				t.Errorf("PreToolUse = %+v, want theirs first and the grant hook", pre)
			}
			if string(view.StatusLine["padding"]) != "4" {
				t.Errorf("their padding is gone: %v", view.StatusLine)
			}
			if argv := tapArgv(t, view); !strings.Contains(argv[len(argv)-1], "their-line") {
				t.Errorf("tap = %q, want their line handed on", argv)
			}
		})
	}
	// The matcher field of their hook is kept as they wrote it.
	view := onlySettings(t, launchObserved(t, []string{"--settings=" + caller}, false))
	var raw map[string][]map[string]json.RawMessage
	_ = json.Unmarshal(view.Keys["hooks"], &raw)
	if string(raw["PreToolUse"][0]["matcher"]) != `"Bash"` {
		t.Errorf("their matcher changed: %s", view.Keys["hooks"])
	}
}

// A --settings rewake cannot read stays as given, alone, and the note says
// what is not happening.
func TestAnUnreadableCallerLayerIsLeftAlone(t *testing.T) {
	newWorld(t)
	for _, value := range []string{"/nonexistent/team.json", "{not json", `{"hooks":[1]}`} {
		plan := launchObserved(t, []string{"--settings", value}, false)
		values := harness.FlagValues(plan.Args, settingsFlag)
		if len(values) != 1 || values[0] != value {
			t.Errorf("%q: args = %v, want theirs alone", value, plan.Args)
		}
		if len(plan.Notes) == 0 || !strings.Contains(strings.Join(plan.Notes, " "), "--settings") {
			t.Errorf("%q: notes = %v, want a word on what is skipped", value, plan.Notes)
		}
	}
}

// A policy that names a status line wins over the tap, and the launch says
// the status values will stay unknown.
func TestAPolicyStatusLineIsNoted(t *testing.T) {
	newWorld(t)
	writeJSON(t, filepath.Join(managedDir, "managed-settings.d", "10-line.json"), statusLine("policy-line"))
	plan := launchObserved(t, nil, false)
	if !strings.Contains(strings.Join(plan.Notes, " "), "managed policy") {
		t.Errorf("notes = %v, want the policy named", plan.Notes)
	}
}

// Without a telemetry socket the layer is what it was: the turn hooks and the
// grant hooks alone.
func TestWithoutASocketThereIsNoTelemetry(t *testing.T) {
	newWorld(t)
	view := onlySettings(t, launch(t, nil, ""))
	if view.StatusLine != nil || len(view.Hooks) != 4 {
		t.Errorf("layer = %+v, want Stop, StopFailure and the grant hooks only", view.Keys)
	}
	if plan := launch(t, nil, ""); plan.Observer != nil {
		t.Error("an observer without a socket")
	}
}

// The observer the wrapper starts listens where the hooks send.
func TestTheObserverListensWhereTheHooksSend(t *testing.T) {
	newWorld(t)
	plan := launchObserved(t, nil, false)
	if plan.Observer == nil {
		t.Fatal("no observer")
	}
	type pathed interface{ Path() string }
	if observer, ok := plan.Observer.(pathed); !ok || observer.Path() != "/tmp/rw/api.1.obs" {
		t.Errorf("observer = %#v, want the socket the hooks name", plan.Observer)
	}
}

// Removing a flag takes its value with it, in both spellings, and nothing
// after the terminator.
func TestWithoutFlagDropsEveryForm(t *testing.T) {
	got := harness.WithoutFlag([]string{"--settings", "a", "-c", "--settings=b", "--", "--settings", "c"}, "--settings")
	want := []string{"-c", "--", "--settings", "c"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A launch through a person's wrapper starts that program and changes
// nothing else; the tap and the hooks still call rewake, not the wrapper.
func TestACommandReplacesOnlyTheProgram(t *testing.T) {
	newWorld(t)
	request := harness.LaunchRequest{Name: "api", Dir: t.TempDir(), Socket: "/tmp/rw/api.1.sock", ObservationSocket: "/tmp/rw/api.1.obs"}
	plain, err := New().Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Command = "claude-worker"
	wrapped, err := New().Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Command != "claude" || wrapped.Command != "claude-worker" {
		t.Errorf("commands %q and %q", plain.Command, wrapped.Command)
	}
	if strings.Join(plain.Args, "\x00") != strings.Join(wrapped.Args, "\x00") || strings.Join(plain.Env, "\x00") != strings.Join(wrapped.Env, "\x00") {
		t.Errorf("the wrapper changed more than the program")
	}
	if argv := tapArgv(t, onlySettings(t, wrapped)); argv[1] != "status-tap" || strings.Contains(argv[0], "claude-worker") {
		t.Errorf("the tap runs %q, want rewake", argv)
	}
}

// The grant hook is told whether the launch added the allow rule for rewake:
// only then may a plain rewake command take a grant back, since a person's own
// allowed tools may leave rewake to be asked about.
func TestTheGrantHookIsToldWhetherTheRewakeRuleIsOurs(t *testing.T) {
	newWorld(t)
	for _, c := range []struct {
		args []string
		ours bool
	}{{nil, true}, {[]string{toolFlag, "Read"}, false}, {[]string{"--allowed-tools=Read"}, false}} {
		plan := launchObserved(t, c.args, false)
		rules := harness.FlagValues(plan.Args, toolFlag)
		if ours := slices.Contains(rules, "Bash(rewake:*)"); ours != c.ours {
			t.Fatalf("launched with %v: allowed tools %v", c.args, rules)
		}
		view := onlySettings(t, plan)
		for _, event := range []string{preToolUse, permissionRequest} {
			matchers := view.Hooks[event]
			if len(matchers) != 1 {
				t.Fatalf("%s hooks = %+v, want the grant hook alone", event, matchers)
			}
			told := strings.HasSuffix(matchers[0].Hooks[0].Command, "'grant-hook' '--"+harness.GrantRewakeRule+"'")
			if told != c.ours {
				t.Errorf("launched with %v: the %s grant hook runs %s", c.args, event, matchers[0].Hooks[0].Command)
			}
		}
	}
}
