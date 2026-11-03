package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// stalledWorker is main's worker after it read main's task and ended a turn
// pending twelve minutes ago, idle since by its telemetry.
func stalledWorker(t *testing.T) (string, registry.Session, registry.Session) {
	t.Helper()
	dir, self, peer := stateCaller(t, "main")
	task := mail.Message{ID: mail.NewID(), From: self.Name, FromEpoch: self.Epoch(), To: peer.Name, ToEpoch: peer.Epoch(), Kind: mail.Task, Text: "run the suite", CreatedAt: time.Now()}
	if err := mail.Put(dir, task); err != nil {
		t.Fatal(err)
	}
	ended := boottime.Now() - int64(12*time.Minute)
	if err := state.WithMailboxLock(context.Background(), dir, peer.Name, func() error {
		if err := mailtest.WriteDeliveryThread(dir, peer.Name, task.ID, "t"); err != nil {
			return err
		}
		if err := mail.LinkUnread(dir, peer.Name, task.ID); err != nil {
			return err
		}
		if err := mail.Open(dir, peer.Name, peer.Epoch()).MarkRead(task, true); err != nil {
			return err
		}
		return mail.MarkPending(dir, peer.Name, peer.Epoch(), mail.MarkName(ended-1, mail.NewID()), "the suite runs", ended-1)
	}); err != nil {
		t.Fatal(err)
	}
	if err := completeTurn(dir, peer, mail.TurnEnd{Text: "started it", Started: ended - 2, Ended: ended}, "t"); err != nil {
		t.Fatal(err)
	}
	snapshot := session.Load(dir, peer.Name, peer.Epoch())
	idle, now := session.ActivityIdle, time.Now()
	snapshot.Activity, snapshot.ActivityAt, snapshot.ActivityFresh, snapshot.Fresh, snapshot.PublishedAt = &idle, &now, true, true, &now
	if err := session.Save(dir, peer.Name, peer.Epoch(), snapshot); err != nil {
		t.Fatal(err)
	}
	return dir, self, peer
}

// Main sees a worker idle under a pending mark with its task open for what it
// is: the state header and rewake list say how long it has been idle, and
// rewake inbox --awaited marks the wait; a turn running, or one heard of
// since, takes it back to plain pending.
func TestMainSeesAStalledPending(t *testing.T) {
	dir, _, peer := stalledWorker(t)

	code, out, errOut := run("list")
	if code != ExitOK || !strings.Contains(out, "pending, idle 12m") {
		t.Fatalf("list: %d %s %s", code, out, errOut)
	}
	code, out, errOut = run("list", "--json")
	var listed struct {
		Sessions []struct {
			Name      string            `json:"name"`
			Telemetry *session.Snapshot `json:"telemetry"`
		} `json:"sessions"`
	}
	if code != ExitOK || json.Unmarshal([]byte(out), &listed) != nil {
		t.Fatalf("list --json: %d %s %s", code, out, errOut)
	}
	var shown *session.Snapshot
	for _, row := range listed.Sessions {
		if row.Name == peer.Name {
			shown = row.Telemetry
		}
	}
	if shown == nil || shown.Verdict == nil || shown.Verdict.Label != "pending, idle 12m" {
		t.Fatalf("the worker's telemetry under --json: %+v", shown)
	}
	if line := stateLine(peer.Name, sessionSnapshot(dir, peer.Name, peer.Epoch()), false); !strings.HasPrefix(line, peer.Name+": pending, idle 12m | context") {
		t.Fatalf("the state header: %s", line)
	}

	code, out, errOut = run("inbox", "--awaited")
	if code != ExitOK || !strings.Contains(out, "pending, idle 12m: the suite runs") {
		t.Fatalf("inbox --awaited: %d %s %s", code, out, errOut)
	}

	// A turn running whose start no harness timed shows only in the activity.
	working := session.Load(dir, peer.Name, peer.Epoch())
	busy, now := session.ActivityWorking, time.Now()
	working.Activity, working.ActivityAt, working.PublishedAt = &busy, &now, &now
	if err := session.Save(dir, peer.Name, peer.Epoch(), working); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := run("inbox", "--awaited"); code != ExitOK || strings.Contains(out, "idle") || !strings.Contains(out, "pending: the suite runs") {
		t.Fatalf("inbox --awaited while a turn runs: %s", out)
	}
	if err := mail.Open(dir, peer.Name, peer.Epoch()).RecordTurnStart(boottime.Now()); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := run("list"); code != ExitOK || strings.Contains(out, "pending, idle") {
		t.Fatalf("list after a turn started: %s", out)
	}
	if code, out, _ := run("inbox", "--awaited"); code != ExitOK || strings.Contains(out, "idle") || !strings.Contains(out, "pending: the suite runs") {
		t.Fatalf("inbox --awaited after a turn started: %s", out)
	}
}
