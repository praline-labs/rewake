package codex

import (
	"slices"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

// A launch through a person's wrapper starts that program for both halves —
// the terminal and the owned server — and changes nothing else.
func TestACommandReplacesOnlyTheProgram(t *testing.T) {
	codexHome(t, "")
	dir := t.TempDir()
	plain, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: dir, Socket: dir + "/s.sock", Epoch: "1"})
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: dir, Socket: dir + "/s.sock", Epoch: "1", Command: "my-codex"})
	if err != nil {
		t.Fatal(err)
	}
	if plain.Command != "codex" || wrapped.Command != "my-codex" {
		t.Errorf("commands %q and %q, want codex and my-codex", plain.Command, wrapped.Command)
	}
	if !slices.Equal(plain.Args, wrapped.Args) || !slices.Equal(plain.Env, wrapped.Env) || plain.Socket != wrapped.Socket {
		t.Errorf("the wrapper changed more than the program:\n%v\n%v", plain.Args, wrapped.Args)
	}
	plainServer, wrappedServer := plain.Backend.(*serverSession), wrapped.Backend.(*serverSession)
	if plainServer.executable() != "codex" || wrappedServer.executable() != "my-codex" {
		t.Errorf("servers run %q and %q", plainServer.executable(), wrappedServer.executable())
	}
	if !slices.Equal(plainServer.args, wrappedServer.args) {
		t.Errorf("server arguments differ: %v vs %v", plainServer.args, wrappedServer.args)
	}
}
