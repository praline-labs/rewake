package codex

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

// What a launch adds for the tool (docs/mail-bridge-launch-codex.md#what-is-added):
// launched with and without it over the caller's lines, both halves differ
// only by the leaves under mcp_servers.rewake, after every -c of the caller's,
// and nothing under CODEX_HOME changes a byte.

func digestDir(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		digest := info.Mode().String()
		if entry.Type().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(raw)
			digest += " " + hex.EncodeToString(sum[:])
		}
		tree[path] = digest
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

var codexCallerLines = [][]string{
	nil,
	{"-c", `model="x"`},
	{"--config", `mcp_servers.other.command="y"`, "-c", "features.web=true"},
	{"--config=tools.web=true", "-m", "x"},
	{"-c", `model="x"`, "--", "a prompt naming -c mcp_servers.rewake.command=z"},
	{"resume", "--last"},
}

func TestTheCodexLaunchOnlyAddsTheLeaves(t *testing.T) {
	home := codexHome(t, `model = "mine"`+"\n"+`[mcp_servers.mine]`+"\n"+`command = "`+sentinel+`"`+"\n")
	before := digestDir(t, home)
	tool := harness.NewToolServer("/opt/rewake", "/state", "room", "api", "e1", "CAPABILITY-secret",
		"/ctx", "", harness.ResolveGates(ID, "", nil))
	ours := toolConfigArgs(toolLeaves(tool))
	for _, line := range codexCallerLines {
		request := harness.LaunchRequest{Name: "api", Dir: t.TempDir(), Args: line}
		without, err := New().Launch(request)
		if err != nil {
			t.Fatal(err)
		}
		request.MailTool = tool
		with, err := New().Launch(request)
		if err != nil {
			t.Fatal(err)
		}
		// The terminal: ours right after the caller's flags, before --remote
		// and any "--".
		if problem := addedAfterCallers(without.Args, with.Args, ours); problem != "" {
			t.Fatalf("%q terminal: %s\nwithout %q\nwith    %q", line, problem, without.Args, with.Args)
		}
		// The owned server: the same -c values in the same order, ours last.
		serverWithout, serverWith := without.Backend.(*serverSession).args, with.Backend.(*serverSession).args
		if problem := addedAfterCallers(serverWithout, serverWith, ours); problem != "" {
			t.Fatalf("%q server: %s\nwithout %q\nwith    %q", line, problem, serverWithout, serverWith)
		}
		if with.Backend.(*serverSession).tool == nil || without.Backend.(*serverSession).tool != nil {
			t.Fatalf("%q: the injection check is not armed for the tool alone", line)
		}
	}
	if !reflect.DeepEqual(before, digestDir(t, home)) {
		t.Fatal("a launch changed CODEX_HOME")
	}
}

// addedAfterCallers says how with differs from without beyond ours, added
// as one run directly after the last -c of the caller's, or "".
func addedAfterCallers(without, with, ours []string) string {
	at := slices.Index(with, ours[0])
	for at >= 0 && !slices.Equal(with[at:min(at+len(ours), len(with))], ours) {
		next := slices.Index(with[at+1:], ours[0])
		if next < 0 {
			at = -1
			break
		}
		at += next + 1
	}
	if at < 0 {
		return "the leaves are not there as one run"
	}
	rest := slices.Concat(with[:at], with[at+len(ours):])
	if !slices.Equal(rest, without) {
		return "more than the leaves changed"
	}
	for index, arg := range without[at:] {
		if arg == "--" {
			break
		}
		if arg == configFlag || arg == "--config" || strings.HasPrefix(arg, "--config=") {
			return "a caller's -c at " + without[at+index] + " comes after the leaves"
		}
	}
	if end := slices.Index(with, "--"); end >= 0 && end < at {
		return "the leaves come after --"
	}
	for _, leaf := range ours {
		if strings.Contains(leaf, "CAPABILITY") && !strings.Contains(leaf, "mcp_servers.rewake.env.") {
			return "the capability travels outside the server's environment"
		}
	}
	return ""
}

// The leaves are exactly the entry the rules name, omit_tools_from on our
// server included, under mcp_servers.rewake alone.
func TestTheLeavesAreTheEntry(t *testing.T) {
	tool := harness.NewToolServer("/opt/re wake", "/state", "room", "api", "e1", `cap"x`, "/ctx", "", harness.Gates{})
	got := map[string]string{}
	args := toolConfigArgs(toolLeaves(tool))
	for index := 0; index < len(args); index += 2 {
		if args[index] != configFlag {
			t.Fatalf("argument %q", args[index])
		}
		key, value, _ := strings.Cut(args[index+1], "=")
		if !strings.HasPrefix(key, "mcp_servers.rewake.") {
			t.Fatalf("a leaf outside our table: %s", key)
		}
		got[strings.TrimPrefix(key, "mcp_servers.rewake.")] = value
	}
	want := map[string]string{
		"command": `"/opt/re wake"`, "args": `["bridge-serve"]`,
		"env.REWAKE_DIR": `"/state"`, "env.REWAKE_ROOM": `"room"`, "env.REWAKE_SESSION": `"api"`, "env.REWAKE_EPOCH": `"e1"`,
		"env.REWAKE_BRIDGE_CAPABILITY": `"cap\"x"`,
		"enabled_tools":                `["rewake"]`, "tool_timeout_sec": "30", "default_tools_approval_mode": `"approve"`,
		"omit_tools_from": `["code_mode"]`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("leaves %v", got)
	}
}
