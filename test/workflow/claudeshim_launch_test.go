package workflow

import (
	"encoding/json"
	"testing"
)

// launchLayerLike is a settings layer shaped like the one rewake builds: the
// end-of-turn hook in the foreground, the telemetry hook in the background on
// every event, and the status-line tap.
func launchLayerLike() string {
	type hook struct {
		Kind    string `json:"type"`
		Command string `json:"command"`
		Async   bool   `json:"async,omitempty"`
	}
	type matcher struct {
		Hooks []hook `json:"hooks"`
	}
	hooks := map[string][]matcher{"StopFailure": {{Hooks: []hook{{"command", "'/r' 'turn-ended'", false}}}}}
	for _, event := range telemetryEvents {
		hooks[event] = append(hooks[event], matcher{Hooks: []hook{{"command", "'/r' 'observe' '/s.obs'", true}}})
	}
	layer, _ := json.Marshal(map[string]any{
		"hooks":      hooks,
		"statusLine": map[string]string{"type": "command", "command": "'/r' 'status-tap' '/s.obs'"},
	})
	return string(layer)
}

// What the Claude Code fixture accepts as a launch, checked directly. The
// fixture must not be looser than the harness: a launch the real one refuses
// is a launch this one refuses too.
func TestClaudeShimRefusesWhatTheHarnessRefuses(t *testing.T) {
	good := []string{
		"--messaging-socket-path", "/tmp/s.sock",
		"--append-system-prompt", "brief",
		"--settings", launchLayerLike(),
		"--allowedTools", "Bash(rewake:*)",
	}
	if _, err := parseClaudeLaunch(good); err != nil {
		t.Fatalf("the launch rewake builds was refused: %v", err)
	}
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"a short flag the harness does not know", append([]string{"-m", "x"}, good...)},
		{"a long flag rewake does not pass", append([]string{"--verbose-debug", "x"}, good...)},
		{"a positional prompt", append(append([]string{}, good...), "hello")},
		{"a flag given twice", append(append([]string{}, good...), "--allowedTools", "Bash(x:*)")},
		{"a flag without its value", append(append([]string{}, good...), "--model")},
		{"settings without telemetry", []string{
			"--messaging-socket-path", "/tmp/s.sock", "--append-system-prompt", "brief",
			"--settings", `{"hooks":{"StopFailure":[{"hooks":[{"type":"command","command":"'/r' 'turn-ended'"}]}]}}`, "--allowedTools", "Bash(rewake:*)",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseClaudeLaunch(tc.args); err == nil {
				t.Errorf("accepted %v", tc.args)
			}
		})
	}
}
