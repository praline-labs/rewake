package cli

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// asEarlierBuild rewrites name's session record as the earlier build wrote
// it: no boot, so its epoch is a pid and a start alone.
func asEarlierBuild(t *testing.T, dir, name string) registry.Session {
	t.Helper()
	session, err := registry.Load(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	session.Boot, session.Build = "", ""
	encoded, _ := json.MarshalIndent(session, "", "  ")
	if err := os.WriteFile(state.SessionPath(dir, name), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return session
}

// A run of the earlier build keeps that build's protocol for its whole life:
// what it calls to change a mailbox is refused, exit 1, naming the restart;
// reading without changing still answers; main hears of it once
// (docs/protocol-cutover.md#what-this-build-refuses-or-holds).
func TestARunOfTheEarlierBuildIsRefused(t *testing.T) {
	dir, _, web := toolSession(t)
	otherRun(t, dir, "lead")
	markMain(t, dir, "lead")
	self := asEarlierBuild(t, dir, "api")
	t.Setenv(epochEnv, self.Epoch())
	rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "work"})
	turnStarted(t, dir, self, markAt-1)
	for _, words := range [][]string{
		{"inbox"},
		{"pending", "the suite runs"},
		{"send", "web", "hello"},
		{"turn-ended", `{"type":"agent-turn-complete","turn-id":"t1","last-assistant-message":"done"}`},
	} {
		code, _, errOut := run(words...)
		if code != ExitFailed || !strings.Contains(errOut, "rewake was upgraded after this session started") || !strings.Contains(errOut, "resuming its conversation") {
			t.Errorf("%s: %d %s", words[0], code, errOut)
		}
	}
	if code, out, errOut := run("inbox", "--peek"); code != ExitOK || !strings.Contains(out, "work") {
		t.Errorf("a peek of the earlier build's run: %d %s %s", code, out, errOut)
	}
	if len(reportsTo(t, dir, "web")) != 0 {
		t.Error("a refused run reached web")
	}
	if notes := reportsTo(t, dir, "lead"); len(notes) != 1 || !strings.Contains(notes[0].Text, "api") {
		t.Errorf("main heard %d notes, want one", len(notes))
	}
}

// A send of this build to a running run of the earlier build is refused
// naming it: this build does not change such a mailbox.
func TestASendToARunningRunOfTheEarlierBuildIsRefused(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	asEarlierBuild(t, dir, "api")
	t.Setenv(state.SessionEnv, "web")
	t.Setenv(epochEnv, web.Epoch())
	for _, words := range [][]string{{"send", "api", "work"}, {"send", "api", "work", "--question", "--wait", "0.2"}} {
		code, _, errOut := run(words...)
		if code != ExitFailed || !strings.Contains(errOut, "api was started by a rewake build before this one") {
			t.Errorf("%v: %d %s", words, code, errOut)
		}
	}
	if entries, _ := os.ReadDir(state.UnreadPath(dir, "api")); len(entries) != 0 {
		t.Errorf("the earlier build's mailbox changed: %d letters", len(entries))
	}
}

// Every record this build writes names a run by boot and epoch: an epoch
// without a boot is read as the earlier build's, so one written by this build
// would send its run's mail down the cutover's path
// (docs/protocol-cutover.md#naming-a-run-across-restarts). A whole exchange
// runs, and no file name or string in the state directory is a bare epoch.
func TestNoRecordOfThisBuildNamesARunWithoutItsBoot(t *testing.T) {
	dir, self, web := toolSession(t)
	otherRun(t, dir, "lead")
	markMain(t, dir, "lead")
	readKind(t, dir, web, inbox.Task)
	turnStarted(t, dir, self, markAt-1)
	if code, _, errOut := run("pending", "the suite runs"); code != ExitOK {
		t.Fatalf("pending: %d %s", code, errOut)
	}
	if err := completeTurn(dir, self, turnResult{Boundary: boundaryNow(t, dir, self), ID: "one", Ended: markAt + 1, Text: "waits"}, ""); err != nil {
		t.Fatal(err)
	}
	readKind(t, dir, web, inbox.Question)
	if err := completeTurn(dir, self, turnResult{Boundary: boundaryNow(t, dir, self), ID: "two", Ended: markAt + 2, Text: "done"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := completeTurn(dir, self, turnResult{Failed: true, Text: "broken"}, ""); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("send", "web", "a note", "--notify", "--wait", "0"); code != ExitOK && code != ExitPending {
		t.Fatalf("send: %d %s", code, errOut)
	}
	walked := 0
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if bareEpoch(entry.Name()) {
			t.Errorf("a path names a bare epoch: %s", path)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var value any
		if json.Unmarshal(raw, &value) != nil {
			return nil
		}
		walked++
		for _, text := range jsonStrings(value) {
			if bareEpoch(text) {
				t.Errorf("%s names a bare epoch: %s", path, text)
			}
		}
		return nil
	})
	if err != nil || walked < 10 {
		t.Fatalf("walked %d records: %v", walked, err)
	}
}

// bareEpoch says text reads as an epoch without a boot. A pid is at most the
// kernel's limit, 2^22, which keeps the test helpers' time-shaped message ids
// out.
func bareEpoch(text string) bool {
	pid, _, _ := registry.ParseEpoch(text)
	return registry.EarlierBuildEpoch(text) && pid <= 1<<22
}

// jsonStrings is every string in a decoded JSON value, keys included.
func jsonStrings(value any) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, item := range v {
			out = append(out, jsonStrings(item)...)
		}
		return out
	case map[string]any:
		var out []string
		for key, item := range v {
			out = append(out, key)
			out = append(out, jsonStrings(item)...)
		}
		return out
	}
	return nil
}
