package workflow

import "testing"

// What the Claude Code fixture accepts as a launch, checked directly. The
// fixture must not be looser than the harness: a launch the real one refuses
// is a launch this one refuses too.
func TestClaudeShimRefusesWhatTheHarnessRefuses(t *testing.T) {
	good := []string{
		"--messaging-socket-path", "/tmp/s.sock",
		"--append-system-prompt", "brief",
		"--settings", `{"hooks":{"StopFailure":[{"hooks":[{"type":"command","command":"x","timeout":10}]}]}}`,
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseClaudeLaunch(tc.args); err == nil {
				t.Errorf("accepted %v", tc.args)
			}
		})
	}
}
