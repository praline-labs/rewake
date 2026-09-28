package codex

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

func TestMailboxBriefingLaunchPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, config     string
		args             []string
		intro, generated bool
	}{
		{name: "ordinary", intro: true, generated: true},
		{name: "disabled"},
		{name: "caller", intro: true, args: []string{"-c", `developer_instructions="caller instructions"`}},
		{name: "config", intro: true, config: `developer_instructions="file instructions"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			codexHome(t, tc.config)
			plan, err := New().Launch(harness.LaunchRequest{Name: "worker", Dir: t.TempDir(), Intro: tc.intro, Args: tc.args})
			if err != nil {
				t.Fatal(err)
			}
			value, _ := configValue(plan.Args, introKey)
			if strings.Contains(value, "rewake_mailbox_notice") != tc.generated {
				t.Fatalf("wrong briefing: %s", value)
			}
			backend := plan.Backend.(*serverSession)
			serverValue, _ := configValue(backend.args, introKey)
			if serverValue != value {
				t.Fatal("server and terminal briefing differ")
			}
			if len(tc.args) > 0 && value != `"caller instructions"` {
				t.Fatal("caller instructions replaced")
			}
			if !tc.generated && !strings.Contains(strings.Join(plan.Notes, " "), "native mailbox notices") {
				t.Fatal("missing omitted-briefing explanation")
			}
			if tc.generated {
				for _, text := range []string{"not a read of the task bodies", "rewake inbox", "peer data", "notifications", "final reply"} {
					if !strings.Contains(value, text) {
						t.Fatalf("briefing lacks %q", text)
					}
				}
			}
		})
	}
}
