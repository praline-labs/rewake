package codex

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

// The version, confirmed at start
// (docs/mail-bridge-launch-codex.md#the-version-confirmed-at-start): the
// transport's note takes the launch's read and the start runs no --version
// of its own; the owned server's initialize answer confirms the version or
// the choice is made again without one.

// startNoted starts a server on the fixture with the launch's version and
// answers its notes; the fixture says whether it was asked its version.
func startNoted(t *testing.T, version harness.Version) []string {
	t.Helper()
	fakeServerExecutable(t)
	codexHome(t, "")
	dir := t.TempDir()
	asked := filepath.Join(dir, "asked")
	t.Setenv("RW_SERVER_VERSION_ASKED", asked)
	path := filepath.Join(dir, "s.sock")
	server := newServer(path, []string{"app-server", "--listen", "unix://" + path}, os.Environ(), dir)
	server.version = version
	var notes []string
	if err := server.Start(context.Background(), harness.CompletionHandler{}, func(note string) { notes = append(notes, note) }); err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if _, err := os.Stat(asked); err == nil {
		t.Fatal("the server's start ran --version again")
	}
	return notes
}

func TestDifferentServerVersionWarnsWithoutRefusing(t *testing.T) {
	for _, version := range []harness.Version{{Value: "9.9.9"}, {Unknown: harness.VersionNotRead}} {
		if notes := startNoted(t, version); len(notes) != 1 || !strings.Contains(notes[0], "codex-cli "+lastObservedServerVersion) {
			t.Fatalf("%+v: version mismatch warning=%v", version, notes)
		}
	}
}

// The test names its own literal rather than the constant, so it fails on a
// typo in the pin instead of comparing the constant with itself.
func TestMatchingServerVersionWarnsAboutNothing(t *testing.T) {
	if notes := startNoted(t, harness.Version{Value: "0.155.1"}); len(notes) != 0 {
		t.Fatalf("pinned version produced notes=%v", notes)
	}
}

// confirmCase is one row of the table: what the launch read, what
// initialize answers, the gates assumed, and the outcome.
type confirmCase struct {
	name      string
	version   harness.Version
	agent     *string
	assumed   []string
	withdrawn bool
	gates     harness.Gates
}

func agentOf(value string) *string { return &value }

func confirmCases() []confirmCase {
	read := harness.Version{Value: "0.159.0"}
	unknown := harness.Version{Unknown: harness.VersionNotRead}
	g2 := []string{harness.GateG2}
	return []confirmCase{
		{"both name it", read, agentOf("rewake/0.159.0 (Ubuntu 24.4.0; x86_64) xterm (rewake; 1)"), g2, false, harness.ResolveGates(ID, "0.159.0", g2)},
		{"another version, assumed", read, agentOf("rewake/0.159.1 (Ubuntu 24.4.0; x86_64)"), g2, false, harness.ResolveGates(ID, "", g2)},
		{"another version", read, agentOf("rewake/0.159.1 (Ubuntu 24.4.0; x86_64)"), nil, true, harness.Gates{}},
		{"absent", read, nil, nil, true, harness.Gates{}},
		{"not parsed", read, agentOf("0.159.0"), nil, true, harness.Gates{}},
		{"empty", read, agentOf(""), g2, false, harness.ResolveGates(ID, "", g2)},
		{"unknown, not consulted", unknown, agentOf("rewake/0.1.0"), g2, false, harness.ResolveGates(ID, "", g2)},
	}
}

// startConfirmed runs the start's upstream half for a run with the tool:
// the server with our leaves, its probe and the confirmation.
func startConfirmed(t *testing.T, tc confirmCase) (*serverSession, []string, error) {
	t.Helper()
	fakeServerExecutable(t)
	codexHome(t, "")
	dir := t.TempDir()
	starts := filepath.Join(dir, "starts")
	t.Setenv("RW_SERVER_STARTS", starts)
	if tc.agent != nil {
		t.Setenv("RW_SERVER_USER_AGENT", *tc.agent)
	}
	path := filepath.Join(dir, "s.sock")
	listen := []string{"app-server", "--listen", "unix://" + path}
	server := newServer(path, append([]string{"-c", "mcp_servers.rewake.command=ours"}, listen...), os.Environ(), dir)
	server.plainArgs = upstreamArgs(path, listen)
	server.version = tc.version
	server.tool = &toolInjection{gates: harness.ResolveGates(ID, tc.version.Value, tc.assumed)}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); server.stopProcess() })
	err := server.startUpstream(ctx, cancel)
	raw, _ := os.ReadFile(starts)
	return server, strings.Split(strings.TrimSpace(string(raw)), "\n"), err
}

func TestTheVersionIsConfirmedAtStartByTheTable(t *testing.T) {
	for _, tc := range confirmCases() {
		t.Run(tc.name, func(t *testing.T) {
			server, starts, err := startConfirmed(t, tc)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.withdrawn {
				if len(starts) != 1 || server.tool == nil || server.ToolWithdrawn() != "" {
					t.Fatalf("kept: %d starts, tool %v, withdrawn %q", len(starts), server.tool != nil, server.ToolWithdrawn())
				}
				bound, ok := server.tool.gates.OutputBound()
				wantBound, wantOK := tc.gates.OutputBound()
				if bound != wantBound || ok != wantOK || server.tool.gates.Open(harness.GateG1) != tc.gates.Open(harness.GateG1) {
					t.Fatalf("kept with gates %+v, want %+v", server.tool.gates, tc.gates)
				}
				return
			}
			if len(starts) != 2 || server.tool != nil || server.ToolWithdrawn() != "harness version not confirmed" {
				t.Fatalf("withdrawn: starts %q, tool %v, reason %q", starts, server.tool != nil, server.ToolWithdrawn())
			}
			first, _, _ := strings.Cut(starts[0], " ")
			pid, _ := strconv.Atoi(first)
			if !strings.Contains(starts[0], "mcp_servers.rewake") || strings.Contains(starts[1], "mcp_servers.rewake") {
				t.Fatalf("the second start kept the leaves, or the first had none: %q", starts)
			}
			if syscall.Kill(pid, 0) == nil {
				t.Fatalf("the first server %d outlived its withdrawal", pid)
			}
			select {
			case <-server.Done():
				t.Fatal("the withdrawal ended the run")
			default:
			}
		})
	}
}

// An initialize that fails fails the start, as it did before the version
// was confirmed.
func TestAFailedInitializeFailsTheStart(t *testing.T) {
	t.Setenv("RW_SERVER_INIT_ERROR", "1")
	tc := confirmCases()[0]
	if _, _, err := startConfirmed(t, tc); err == nil || !strings.Contains(err.Error(), "app-server startup failed") {
		t.Fatalf("got %v", err)
	}
}

func TestTheAgentsVersionIsAfterTheLastSlashOfItsFirstWord(t *testing.T) {
	for agent, want := range map[string]string{
		"rewake/0.159.0 (Ubuntu 24.4.0; x86_64) xterm (rewake; 1)": "0.159.0",
		"a/b/0.159.0 (x)": "0.159.0",
		"rewake 0.159.0/": "",
		"0.159.0":         "",
		"":                "",
	} {
		if got := agentVersion(agent); got != want {
			t.Fatalf("%q: %q, want %q", agent, got, want)
		}
	}
}
