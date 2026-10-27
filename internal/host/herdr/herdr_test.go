package herdr

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/session"
)

func snapshot(activity string) session.Snapshot {
	return session.Snapshot{Activity: &activity, ActivityFresh: true}
}

func TestTheStateFollowsTheRunsActivity(t *testing.T) {
	yes, dialog := true, "trust this folder"
	compacting := snapshot("working")
	compacting.Compacting = &yes
	permission := snapshot("working")
	permission.Waiting = session.WaitingPermission
	question := snapshot("idle")
	question.Waiting = session.WaitingQuestion
	atDialog := snapshot("idle")
	atDialog.Dialog = &dialog
	stale := snapshot("idle")
	stale.ActivityFresh = false
	for _, c := range []struct {
		name           string
		snapshot       session.Snapshot
		state, message string
	}{
		{"idle", snapshot("idle"), Idle, ""},
		{"working", snapshot("working"), Working, ""},
		{"compacting", compacting, Working, ""},
		{"permission", permission, Blocked, "waiting for a permission"},
		{"question", question, Blocked, "waiting for an answer"},
		{"dialog", atDialog, Blocked, "waiting at a dialog"},
		{"not reported yet", session.Unknown("e"), Unknown, ""},
		{"stale", stale, Unknown, ""},
		{"another word", snapshot("sleeping"), Unknown, ""},
	} {
		state, message := StateOf(c.snapshot)
		if state != c.state || message != c.message {
			t.Errorf("%s: got %q %q, want %q %q", c.name, state, message, c.state, c.message)
		}
	}
}

func TestAResumeCommandKeepsHerdrsRules(t *testing.T) {
	many := slices.Repeat([]string{"a"}, maxResumeArgs)
	for _, c := range []struct {
		name string
		argv []string
		ok   bool
	}{
		{"plain", []string{"rewake", "agent", "--resume", "0f0e0d0c-0b0a"}, true},
		{"at the limit", many, true},
		{"empty", nil, false},
		{"a path first", []string{"/usr/bin/rewake", "agent"}, false},
		{"an apostrophe", []string{"rewake", "it's"}, false},
		{"a control character", []string{"rewake", "a\nb"}, false},
		{"too many", append(many, "a"), false},
		{"too long", []string{"rewake", strings.Repeat("a", maxResumeBytes)}, false},
	} {
		if ValidResume(c.argv) != c.ok {
			t.Errorf("%s: got %v", c.name, !c.ok)
		}
	}
}

func TestOnlyARunInsideHerdrReports(t *testing.T) {
	full := map[string]string{"HERDR_ENV": "1", "HERDR_PANE_ID": "w1:p1", "HERDR_BIN_PATH": "/bin/herdr", "HERDR_SOCKET_PATH": "/run/herdr.sock"}
	if pane, ok := FromEnv(func(k string) string { return full[k] }); !ok || pane != (Pane{ID: "w1:p1", Bin: "/bin/herdr"}) {
		t.Fatalf("inside herdr: %+v %v", pane, ok)
	}
	for missing := range full {
		if _, ok := FromEnv(func(k string) string {
			if k == missing {
				return ""
			}
			return full[k]
		}); ok {
			t.Errorf("reports without %s", missing)
		}
	}
}

type recorder struct {
	mu    sync.Mutex
	calls [][]string
	state session.Snapshot
}

func (r *recorder) run(_ context.Context, _ string, args ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, args)
	return nil
}

func (r *recorder) set(s session.Snapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = s
}

func (r *recorder) snapshot() session.Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *recorder) waitFor(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for r.count() < n {
		if time.Now().After(deadline) {
			t.Fatalf("%d reports, want %d", r.count(), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func start(r *recorder, build string) func() {
	return Start(context.Background(), Config{
		Pane: Pane{ID: "w1:p1", Bin: "herdr"}, Agent: "agent", State: r.snapshot, Build: build, Run: r.run,
		Resume: func(c string) []string { return []string{"agent", "--resume", c} },
		Probe:  func(context.Context) string { return "this-build" },
	})
}

func TestARunReportsEachChangeOnceAndReleasesItsPane(t *testing.T) {
	r := &recorder{}
	idle := snapshot("idle")
	idle.Thread = "c1"
	r.set(idle)
	stop := start(r, "this-build")
	r.waitFor(t, 1)
	time.Sleep(3 * poll)
	if r.count() != 1 {
		t.Fatalf("an unchanged state was reported again: %q", r.calls)
	}
	working := snapshot("working")
	working.Thread = "c1"
	r.set(working)
	r.waitFor(t, 2)
	stop()

	want := [][]string{
		{"pane", "report-agent", "w1:p1", "--source", "rewake", "--agent", "agent", "--state", "idle", "--seq", "", "--agent-session-id", "c1", "--", "rewake", "agent", "--resume", "c1"},
		{"pane", "report-agent", "w1:p1", "--source", "rewake", "--agent", "agent", "--state", "working", "--seq", "", "--agent-session-id", "c1", "--", "rewake", "agent", "--resume", "c1"},
		{"pane", "release-agent", "w1:p1", "--source", "rewake", "--agent", "agent", "--seq", ""},
	}
	if len(r.calls) != len(want) {
		t.Fatalf("calls %q", r.calls)
	}
	last := int64(0)
	for i, call := range r.calls {
		seq := slices.Index(call, "--seq") + 1
		n, err := strconv.ParseInt(call[seq], 10, 64)
		if err != nil || n <= last {
			t.Errorf("seq %s does not rise past %d", call[seq], last)
		}
		last = n
		call = slices.Clone(call)
		call[seq] = ""
		if !slices.Equal(call, want[i]) {
			t.Errorf("call %d: %q, want %q", i, call, want[i])
		}
	}
}

func TestNoResumeCommandWhenPathFindsAnotherBuild(t *testing.T) {
	r := &recorder{}
	idle := snapshot("idle")
	idle.Thread = "c1"
	r.set(idle)
	stop := start(r, "another-build")
	r.waitFor(t, 1)
	stop()
	if slices.Contains(r.calls[0], "--") {
		t.Errorf("a resume command for another build: %q", r.calls[0])
	}
}
