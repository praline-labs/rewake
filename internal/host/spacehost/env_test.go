package spacehost

import (
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/host/terminal"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The worker's environment carries nothing of main's session — its name and
// run, which would make the worker's launch refuse as nested, its room,
// main's harness markers, a bridge's transport, the space host's spec — and
// always names main's base, whatever the space host's own environment said or
// left to the default. A worker under the default base would join another
// room of the same name, and nothing live would show it.
func TestTheWorkerGetsMainsBaseAndNothingOfItsSession(t *testing.T) {
	inherited := []string{
		"HOME=/home/someone", "PATH=/usr/bin", "LANG=C.UTF-8",
		state.SessionEnv + "=main-claude", state.EpochEnv + "=4242.1700.boot",
		state.RoomEnv + "=work", state.DirEnv + "=/elsewhere",
		"REWAKE_BRIDGE_TICKET_FD=3", "MAIN_MARKER=1", "REWAKE_MOD_SOCKET=/run/main.mod.sock",
		Env + `={"stage":"serve"}`, "TERM=xterm-kitty",
	}
	got, err := WorkerEnv(inherited, "/tmp/rewake-1000", []string{"MAIN_MARKER", "REWAKE_MOD_SOCKET"})
	if err != nil {
		t.Fatal(err)
	}
	got = terminal.Environ(got)
	for _, name := range []string{state.SessionEnv, state.EpochEnv, state.RoomEnv, "REWAKE_BRIDGE_TICKET_FD", "MAIN_MARKER", "REWAKE_MOD_SOCKET", Env} {
		if slices.ContainsFunc(got, func(kv string) bool { return strings.HasPrefix(kv, name+"=") }) {
			t.Errorf("the worker inherits %s: %q", name, got)
		}
	}
	var dirs []string
	for _, kv := range got {
		if value, ok := strings.CutPrefix(kv, state.DirEnv+"="); ok {
			dirs = append(dirs, value)
		}
	}
	if !slices.Equal(dirs, []string{"/tmp/rewake-1000"}) {
		t.Errorf("REWAKE_DIR is %q, want main's base once", dirs)
	}
	for _, kept := range []string{"HOME=/home/someone", "PATH=/usr/bin", "TERM=xterm-256color"} {
		if !slices.Contains(got, kept) {
			t.Errorf("the worker lacks %s: %q", kept, got)
		}
	}
	for _, base := range []string{"", "relative/base"} {
		if _, err := WorkerEnv(inherited, base, nil); err == nil {
			t.Errorf("a base of %q was given to the worker", base)
		}
	}
}

// A worker runs in no pane of herdr's, so it carries none of the variables
// herdr gives main's pane: a main the worker starts, a live check's in a
// private room, would otherwise report itself as the agent of the person's
// pane. The space host drops them, so the worker made from its environment
// has none, while main's own environment is left as it was.
func TestTheWorkerCarriesNoneOfHerdrsVariables(t *testing.T) {
	mains := []string{
		"HOME=/home/someone", "PATH=/usr/bin",
		"HERDR_ENV=1", "HERDR_PANE_ID=w2:p7M", "HERDR_TAB_ID=w2:t1", "HERDR_WORKSPACE_ID=w2",
		"HERDR_SOCKET_PATH=/run/herdr.sock", "HERDR_BIN_PATH=/usr/bin/herdr", "HERDR_STARTUP_CWD=/home/someone",
	}
	given := slices.Clone(mains)
	host := hostEnv(given, Spec{Base: "/tmp/rewake-1000", Room: "work"})
	worker, err := WorkerEnv(host, "/tmp/rewake-1000", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range slices.Concat(host, worker) {
		if strings.HasPrefix(kv, "HERDR_") {
			t.Errorf("the space host or the worker carries %s", kv)
		}
	}
	for _, kept := range []string{"HOME=/home/someone", "PATH=/usr/bin"} {
		if !slices.Contains(worker, kept) {
			t.Errorf("the worker lacks %s: %q", kept, worker)
		}
	}
	if !slices.Equal(given, mains) {
		t.Errorf("main's environment was changed: %q", given)
	}
}
