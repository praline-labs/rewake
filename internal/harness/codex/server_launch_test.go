package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
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
	if !strings.Contains(strings.Join(server.args, " "), introKey+"=") || !strings.Contains(strings.Join(server.args, " "), "app-server --listen unix:///tmp/session.sock") {
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

func TestDifferentServerVersionWarnsWithoutRefusing(t *testing.T) {
	fakeServerExecutable(t)
	codexHome(t, "")
	t.Setenv("RW_SERVER_VERSION", "codex-cli 9.9.9")
	dir := t.TempDir()
	path := filepath.Join(dir, "s.sock")
	server := newServer(path, []string{"app-server", "--listen", "unix://" + path}, os.Environ(), dir)
	var notes []string
	if err := server.Start(context.Background(), nil, func(note string) { notes = append(notes, note) }); err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if len(notes) != 1 || !strings.Contains(notes[0], "0.154.0") {
		t.Fatalf("version mismatch warning=%v", notes)
	}
}
