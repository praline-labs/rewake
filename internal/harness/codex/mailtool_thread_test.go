package codex

import (
	"context"
	"encoding/json"
	"maps"
	"net"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

// The injection check (docs/mail-bridge-launch-codex.md#the-injection-check-at-start-and-at-every-thread):
// step 0 over every key set a request may carry, the steps on the server's
// answer over every entry it may hold, and every thread request through a
// server answering as 0.159.0's does.

// rulesTerminalKeys are the keys step 0 lets through, read from the rule
// itself rather than from the code under test: the backquoted names between
// the source it cites and the sentence on features.
func rulesTerminalKeys(t *testing.T) []string {
	t.Helper()
	text, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "mail-bridge-launch-codex.md"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(text), "`tui/src/app_server_session.rs`):")
	end := strings.Index(string(text), "`features`, which may reach registrations")
	if start < 0 || end < start {
		t.Fatal("the rule's list of terminal keys is not where the test reads it")
	}
	parts := strings.Split(string(text[start:end]), "`")
	var keys []string
	for i := 3; i < len(parts); i += 2 {
		keys = append(keys, parts[i])
	}
	return keys
}

func TestTheTerminalSendsFourteenKeys(t *testing.T) {
	// A fact of 0.159.0's terminal, read in its source and written in the
	// rule: the list grows only when a version is checked against it.
	keys := rulesTerminalKeys(t)
	if len(keys) != 14 || !slices.Equal(slices.Sorted(maps.Keys(terminalKeys)), slices.Sorted(slices.Values(keys))) {
		t.Fatalf("the code lets %v through, the rule %v", slices.Sorted(maps.Keys(terminalKeys)), keys)
	}
}

func TestStepZeroOverEveryKeySet(t *testing.T) {
	keys := rulesTerminalKeys(t)
	offList := []string{"", "mcp_servers", "model", "projects", "developer_instructions", "Features"}
	roots := []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`[]`), json.RawMessage(`["/x"]`)}
	cases := 0
	for set := 0; set < 1<<len(keys); set++ {
		config := map[string]json.RawMessage{}
		for index, key := range keys {
			if set&(1<<index) != 0 {
				config[key] = json.RawMessage(`{}`)
			}
		}
		for _, extra := range offList {
			withExtra := maps.Clone(config)
			if extra != "" {
				withExtra[extra] = json.RawMessage(`1`)
			}
			for _, root := range roots {
				var want []string
				if root != nil && string(root) != "null" {
					want = append(want, refusedRoots)
				}
				if extra != "" {
					want = append(want, refusedKeyOffList)
				}
				got := stepZero(threadRequest{Config: withExtra, RuntimeRoots: root})
				if (len(want) == 0) != (got == "") || len(want) > 0 && !slices.Contains(want, got) {
					t.Fatalf("keys %v roots %s: got %q, want one of %q", slices.Sorted(maps.Keys(withExtra)), root, got, want)
				}
				cases++
			}
		}
	}
	t.Logf("%d requests", cases)
}

// testLeaves are a run's leaves, as the launch adds them.
func testLeaves() []toolLeaf {
	return toolLeaves(harness.NewToolServer("/opt/rewake", "/state", "room", "api", "e1", "cap", "/ctx", "", harness.Gates{}))
}

// entryOf builds an entry from flattened leaves.
func entryOf(leaves map[string]any) map[string]any {
	entry := map[string]any{}
	for path, value := range leaves {
		parts := strings.Split(path, ".")
		table := entry
		for _, part := range parts[:len(parts)-1] {
			inner, ok := table[part].(map[string]any)
			if !ok {
				inner = map[string]any{}
				table[part] = inner
			}
			table = inner
		}
		table[parts[len(parts)-1]] = value
	}
	return entry
}

func oursFlat(leaves []toolLeaf) map[string]any {
	flat := map[string]any{}
	for _, leaf := range leaves {
		flat[leaf.leafPath()] = leaf.value
	}
	return flat
}

// edit returns ours with changes; a nil value removes the leaf, unless
// keepNil.
func edit(base map[string]any, changes map[string]any, keepNil bool) map[string]any {
	out := maps.Clone(base)
	for key, value := range changes {
		if value == nil && !keepNil {
			delete(out, key)
			continue
		}
		out[key] = value
	}
	return out
}

func TestTheStepsOverEveryEntryTheServerMayHold(t *testing.T) {
	leaves := testLeaves()
	ours := oursFlat(leaves)
	sessions := []struct {
		flat  map[string]any
		extra bool
	}{
		{nil, false},
		{ours, false},
		{edit(ours, map[string]any{"startup_timeout_sec": float64(5)}, false), true},
		{edit(ours, map[string]any{"env.K": sentinel}, false), true},
		// One of our keys at another value is the caller's, not ours.
		{edit(ours, map[string]any{"command": sentinel}, false), true},
		// Tables that hold nothing still name something: the server whole,
		// and one nested next to our leaves.
		{map[string]any{}, true},
		{edit(ours, map[string]any{"env.EMPTY": map[string]any{}}, false), true},
		{edit(ours, map[string]any{"tools": map[string]any{}}, false), true},
	}
	// Another layer names the server with values, with an empty table, or
	// with exactly our leaves, which only the session flags may hold.
	others := []struct {
		kind  string
		empty bool
		ours  bool
	}{
		{"", false, false},
		{layerUser, false, false},
		{layerProject, false, false},
		{"system", false, false},
		{layerUser, true, false},
		{layerProject, true, false},
		{"system", true, false},
		{layerUser, false, true},
		{layerProject, false, true},
		{"system", false, true},
	}
	effectives := []struct {
		flat  map[string]any
		want  string // a refusal, or a readsOff, by the oracle's words below
		reads bool   // reads off for omit_tools_from
	}{
		{nil, refusedNotOurs, false},
		{ours, "", false},
		{edit(ours, map[string]any{"enabled": true, "environment_id": "local", "startup_timeout_sec": nil}, true), "", false},
		{edit(ours, map[string]any{"omit_tools_from": nil}, false), "", true},
		{edit(ours, map[string]any{"omit_tools_from": []any{"other"}}, false), "", true},
		{edit(ours, map[string]any{"command": sentinel}, false), refusedNotOurs, false},
		{edit(ours, map[string]any{"command": nil}, false), refusedNotOurs, false},
		{edit(ours, map[string]any{"enabled": false}, false), refusedNotOurs, false},
		{edit(ours, map[string]any{"startup_timeout_sec": float64(5)}, false), refusedNotOurs, false},
	}
	cases := 0
	for _, session := range sessions {
		for _, other := range others {
			for _, effective := range effectives {
				for _, requires := range []bool{false, true} {
					for _, limit := range []any{"unset", nil, float64(1000)} {
						var reply configReply
						if other.kind != "" {
							layer := layerOf(other.kind, true)
							switch {
							case other.empty:
								layer.Config["mcp_servers"].(map[string]any)["rewake"] = map[string]any{}
							case other.ours:
								layer.Config["mcp_servers"].(map[string]any)["rewake"] = entryOf(ours)
							}
							reply.Layers = append(reply.Layers, layer)
						}
						flags := map[string]any{}
						if session.flat != nil {
							flags["mcp_servers"] = map[string]any{"rewake": entryOf(session.flat)}
						}
						reply.Layers = append(reply.Layers, configLayer{Name: layerSource{Kind: layerSessionFlags}, Config: flags})
						reply.Config = map[string]any{}
						if effective.flat != nil {
							reply.Config["mcp_servers"] = map[string]any{"rewake": entryOf(effective.flat)}
						}
						if limit != "unset" {
							reply.Config["tool_output_token_limit"] = limit
						}
						requirements := map[string]any{"requirements": nil}
						if requires {
							requirements = map[string]any{"requirements": map[string]any{"mcpServers": map[string]any{}}}
						}
						refusal, readsOff := judgeOracle(other.kind, session.extra, effective.want, effective.reads, requires, limit)
						injection := &toolInjection{args: []string{"-c", `model="x"`}, leaves: leaves}
						got := injection.judge(reply, requirements)
						if !strings.HasPrefix(got.refusal, refusal) || refusal == "" && got.refusal != "" || got.readsOff != readsOff {
							t.Fatalf("session %v other %+v effective %v requires %v limit %v: got %+v, want %q %q",
								session.flat, other, effective.flat, requires, limit, got, refusal, readsOff)
						}
						if strings.Contains(got.refusal, sentinel) {
							t.Fatalf("the refusal carries a value: %s", got.refusal)
						}
						cases++
					}
				}
			}
		}
	}
	t.Logf("%d answers", cases)
}

// judgeOracle is the steps' answer as the rules order them: another source
// of the name, then our entry leaf by leaf, then requirements, then the
// output limit.
func judgeOracle(other string, sessionExtra bool, effective string, omitOff, requires bool, limit any) (string, string) {
	switch {
	case other == layerUser:
		return "another MCP server named rewake is configured for it (user scope, /cfg/user.toml)", ""
	case other == layerProject:
		return "another MCP server named rewake is configured for it (project scope, /work/.codex)", ""
	case other != "":
		return "another MCP server named rewake is configured for it (managed scope", ""
	case sessionExtra:
		return "another MCP server named rewake is configured for it (session flags)", ""
	case effective != "":
		return effective, ""
	case requires:
		return refusedRequirements, ""
	case omitOff:
		return "", readsOffOmit
	case limit != "unset" && limit != nil:
		return "", readsOffLimit
	}
	return "", ""
}

// fakeUpstream serves a recorded answer in this process, at a short path.
func fakeUpstream(t *testing.T, reply string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "rwt")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close(); _ = os.RemoveAll(dir) })
	go servePreflight(listener, "answer", []byte(reply))
	return socket
}

func TestEveryThreadRequestThroughTheCheck(t *testing.T) {
	leaves := testLeaves()
	launchDir, trusted, untrusted := t.TempDir(), t.TempDir(), t.TempDir()
	reply, _ := json.Marshal(map[string]any{
		"config": map[string]any{
			"mcp_servers": map[string]any{"rewake": entryOf(oursFlat(leaves))},
			"projects": map[string]any{
				trusted:   map[string]any{"trust_level": "untrusted"},
				launchDir: map[string]any{"trust_level": "trusted"},
			},
		},
		"layers": []any{map[string]any{"name": map[string]any{"type": layerSessionFlags}, "config": map[string]any{
			"features":    map[string]any{"web": true},
			"mcp_servers": map[string]any{"rewake": entryOf(oursFlat(leaves))},
		}}},
	})
	upstream := fakeUpstream(t, string(reply))
	// Absent, null, empty, relative, and two absolute ones: every form a
	// request may give its cwd in.
	cwds := []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`""`), json.RawMessage(`"sub"`), quoted(trusted), quoted(untrusted)}
	decided := map[string]bool{trusted: true, launchDir: true}
	features := []json.RawMessage{nil, json.RawMessage(`{"web":true}`), json.RawMessage(`{"web":false}`), json.RawMessage(`true`)}
	cases := 0
	for _, method := range []string{"thread/start", "thread/resume", "thread/fork", "turn/start"} {
		for _, cwd := range cwds {
			for _, assumed := range [][]string{nil, {harness.GateG9}, {harness.GateG3}, {harness.GateG3, harness.GateG9}} {
				for _, feature := range features {
					for _, offList := range []bool{false, true} {
						params := map[string]any{}
						if cwd != nil {
							params["cwd"] = cwd
						}
						config := map[string]any{}
						if feature != nil {
							config["features"] = feature
						}
						if offList {
							config["model"] = "x"
						}
						params["config"] = config
						raw, _ := json.Marshal(params)
						gates := harness.ResolveGates(ID, "", assumed)
						injection := &toolInjection{upstream: upstream, cwd: launchDir, args: []string{"-c", "features.web=true"}, leaves: leaves, gates: gates}
						got, applies := injection.checkThread(context.Background(), method, raw)
						want, wantApplies := threadOracle(method, raw, launchDir, decided, gates, feature, offList)
						if applies != wantApplies || got.refusal != want || got.readsOff != "" {
							t.Fatalf("case %d: %s %s assumed %v features %s off-list %v: got %+v %v, want %q", cases, method, raw, assumed, feature, offList, got, applies, want)
						}
						cases++
					}
				}
			}
		}
	}
	t.Logf("%d thread requests", cases)
}

func quoted(value string) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

// threadOracle is a thread request's answer as the rules state it, from the
// request as sent: a cwd is named when the key holds a string, any string,
// and the server takes a relative one from its own directory.
func threadOracle(method string, raw json.RawMessage, launchDir string, decided map[string]bool, gates harness.Gates, feature json.RawMessage, offList bool) (string, bool) {
	var params map[string]any
	_ = json.Unmarshal(raw, &params)
	given, named := params["cwd"].(string)
	if !path.IsAbs(given) {
		given = path.Join(launchDir, given)
	}
	switch {
	case !strings.HasPrefix(method, "thread/"):
		return "", false
	case offList:
		return refusedKeyOffList, true
	case named && gates.Open(harness.GateG9):
		return refusedCwdUnproven, true
	case !named && method != "thread/start" && gates.Open(harness.GateG3):
		return refusedNoCwd, true
	case feature != nil && string(feature) != `{"web":true}`:
		return refusedFeatures, true
	case named && !decided[given]:
		return refusedNoTrust, true
	}
	return "", true
}
