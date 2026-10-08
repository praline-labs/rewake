package codex

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

func TestLaunchOwnsServerAndKeepsPromptOnTUI(t *testing.T) {
	codexHome(t, "")
	plan, err := New().Launch(harness.LaunchRequest{Name: "writer", Dir: t.TempDir(), Socket: "/tmp/session.sock", Intro: true, Epoch: "1.2", Room: "work", Args: []string{"-c", `notify=["user-hook"]`, "--", "caller prompt"}})
	if err != nil {
		t.Fatal(err)
	}
	server, ok := plan.Backend.(*serverSession)
	if !ok {
		t.Fatal("launch does not own an app-server")
	}
	if !strings.Contains(strings.Join(server.args, " "), introKey+"=") || !strings.Contains(strings.Join(server.args, " "), "app-server --listen unix:///tmp/session.sock.up") {
		t.Fatalf("server args=%q", server.args)
	}
	if !strings.Contains(strings.Join(plan.Args, " "), "--remote unix:///tmp/session.sock") || plan.Args[len(plan.Args)-1] != "caller prompt" {
		t.Fatalf("TUI args=%q", plan.Args)
	}
	if plan.Args[len(plan.Args)-2] != "--" {
		t.Fatal("caller prompt was consumed by an option")
	}
	if strings.Contains(strings.Join(server.args, " "), "turn-ended") {
		t.Fatal("legacy notify was installed")
	}
	if !strings.Contains(strings.Join(server.env, "\n"), "REWAKE_SESSION=writer") {
		t.Fatal("server did not inherit session identity")
	}
}

func TestForkLaunchContextDoesNotComeFromOptionValues(t *testing.T) {
	codexHome(t, "")
	for _, test := range []struct {
		args []string
		fork bool
	}{
		{[]string{"fork", "--last"}, true},
		{[]string{"-m", "fixture", "fork", "parent"}, true},
		{[]string{"-m", "fork"}, false},
		{[]string{"--", "fork"}, false},
		{[]string{"-c", `developer_instructions="fork"`}, false},
		{[]string{"resume", "parent"}, false},
	} {
		plan, err := New().Launch(harness.LaunchRequest{Name: "worker", Dir: t.TempDir(), Args: test.args})
		if err != nil {
			t.Fatal(err)
		}
		if got := plan.Backend.(*serverSession).startupFork; got != test.fork {
			t.Fatalf("args=%q fork=%v", test.args, got)
		}
	}
}
