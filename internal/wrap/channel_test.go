package wrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

// The channel keeper (docs/mail-bridge-channel.md): what it folds, when a
// close counts as a failure, and where its notices go or are dropped.

// mainPeer publishes a ready, live main of this build: this process.
func mainPeer(t *testing.T, dir, name string) registry.Session {
	t.Helper()
	if err := state.EnsureSubdir(state.SessionsPath(dir)); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Minute)
	peer := registry.Session{
		Name: name, Role: role.Main.ID, Harness: "fixture", Room: filepath.Base(dir), CWD: "/workspace",
		ServicePID: os.Getpid(), ServiceStart: selfStart(t), Boot: thisBoot(t), PIDNamespace: proc.Namespace(), StartedAt: at, MessagingReadyAt: &at,
	}
	if err := registry.Publish(dir, peer); err != nil {
		t.Fatal(err)
	}
	return peer
}

// keeperOf is the channel keeper of name's live run, which this process
// serves: a notice to the run lands only while the run lives.
func keeperOf(t *testing.T, dir, name string) *channelKeeper {
	t.Helper()
	if current, err := registry.LookupReadOnly(dir, name); err == nil {
		return newChannelKeeper(dir, name, current.Epoch(), "codex")
	}
	at := time.Now().Add(-time.Minute)
	run := registry.Session{
		Name: name, Harness: "fixture", Room: filepath.Base(dir), CWD: "/workspace",
		ServicePID: os.Getpid(), ServiceStart: selfStart(t), Boot: thisBoot(t), PIDNamespace: proc.Namespace(), StartedAt: at, MessagingReadyAt: &at,
	}
	if err := registry.Publish(dir, run); err != nil {
		t.Fatal(err)
	}
	return newChannelKeeper(dir, name, run.Epoch(), "codex")
}

// ago is a moment a while back on both clocks.
func ago(d time.Duration) channel.Stamp {
	now := endpoint.Stamp()
	return channel.Stamp{Boot: now.Boot - int64(d), Wall: now.Wall.Add(-d)}
}

// failingKeeper is a keeper whose server said hello and closed a heartbeat
// ago while the harness lives or not: on Codex, which leaves a dead server
// down, a failure.
func failingKeeper(t *testing.T, dir, name string, alive bool) *channelKeeper {
	t.Helper()
	k := keeperOf(t, dir, name)
	k.begin(true, "")
	k.alive = func() bool { return alive }
	k.tell(channel.Event{Kind: channel.Hello, Generation: 1, At: ago(2 * heartbeat)})
	k.tell(channel.Event{Kind: channel.Closed, Generation: 1, At: ago(heartbeat)})
	return k
}

func bodies(messages []inbox.Message) string {
	var texts []string
	for _, message := range messages {
		texts = append(texts, message.Text)
	}
	return strings.Join(texts, "\n---\n")
}

func TestAClosedServerFailsOnlyWhileItsHarnessLives(t *testing.T) {
	for _, alive := range []bool{true, false} {
		dir := stateDir(t)
		k := failingKeeper(t, dir, "api", alive)
		if record := k.snapshot(); record.Tool == channel.ToolFailing {
			t.Fatal("a close was folded before the wrapper knew whether the harness lives")
		}
		k.beat(context.Background())
		record := k.snapshot()
		if failing := record.Tool == channel.ToolFailing; failing != alive {
			t.Fatalf("harness alive %v: tool %s", alive, record.Tool)
		}
		got := availabilityFiles(t, dir, "api")
		if told := strings.Contains(bodies(got), channel.FirstWorkerFailing); told != alive {
			t.Fatalf("harness alive %v: the worker was told %q", alive, bodies(got))
		}
	}
}

func TestANoticeLandsOnceAcrossHeartbeats(t *testing.T) {
	dir := stateDir(t)
	k := failingKeeper(t, dir, "api", true)
	for range 3 {
		k.beat(context.Background())
	}
	if got := availabilityFiles(t, dir, "api"); len(got) != 1 || got[0].Kind != inbox.Note || got[0].From != "api" {
		t.Fatalf("worker mailbox %+v", got)
	}
}

func TestMainIsToldUnlessItIsTheRun(t *testing.T) {
	dir := stateDir(t)
	lead := mainPeer(t, dir, "lead")
	k := failingKeeper(t, dir, "api", true)
	k.beat(context.Background())
	got := availabilityFiles(t, dir, "lead")
	if len(got) != 1 || !strings.HasPrefix(got[0].Text, channel.FirstMainFailing) || got[0].ToEpoch != lead.Epoch() {
		t.Fatalf("main's mailbox %+v", got)
	}
	// The main's own channel tells main nothing a second time.
	self := failingKeeper(t, dir, "lead", true)
	self.epoch = lead.Epoch()
	self.beat(context.Background())
	if got := availabilityFiles(t, dir, "lead"); len(got) != 2 || strings.Count(bodies(got), channel.FirstMainFailing) != 1 {
		t.Fatalf("main's mailbox after its own failure %q", bodies(got))
	}
}

func TestAdviceFixedBeforeTheBlockIsDropped(t *testing.T) {
	dir := stateDir(t)
	k := failingKeeper(t, dir, "api", true)
	k.mu.Lock()
	k.record.Fold(channel.Event{Kind: channel.Closed, Generation: 1, Alive: true, At: endpoint.Stamp()})
	publication, planned := k.notices.Plan(&k.record, channel.Recipient{Role: channel.ToWorker, Name: "api", Epoch: k.epoch}, endpoint.Stamp())
	k.record.Fold(channel.Event{Kind: channel.Denied, At: endpoint.Stamp()})
	k.mu.Unlock()
	if !planned || !publication.Advice {
		t.Fatalf("planned %v %+v", planned, publication)
	}
	if state, _ := k.attempt(context.Background(), publication); state != channel.Dropped {
		t.Fatalf("an advising notice after the block: %s", state)
	}
	if got := availabilityFiles(t, dir, "api"); len(got) != 0 {
		t.Fatalf("a dropped notice was published: %+v", got)
	}
}

func TestANoticeForAMainThatLeftIsDropped(t *testing.T) {
	dir := stateDir(t)
	lead := mainPeer(t, dir, "lead")
	k := failingKeeper(t, dir, "api", true)
	gone := channel.Publication{Seq: 1, To: channel.Recipient{Role: channel.ToMain, Name: "lead", Epoch: lead.Epoch() + "x"}, Key: "k", Body: "b", Fixed: endpoint.Stamp()}
	if state, _ := k.attempt(context.Background(), gone); state != channel.Dropped {
		t.Fatalf("a notice for a main that left: %s", state)
	}
	if got := availabilityFiles(t, dir, "lead"); len(got) != 0 {
		t.Fatalf("published to the main that came after: %+v", got)
	}
}

func TestTheShellObservationsAreFoldedAndTaken(t *testing.T) {
	dir := stateDir(t)
	k := keeperOf(t, dir, "api")
	k.begin(false, "gate G2: not settled")
	now := endpoint.Stamp()
	receipt.WriteShell(dir, "api", k.epoch, receipt.ShellNote{OK: false, Class: receipt.ShellReadOnly, Boot: now.Boot, Wall: now.Wall})
	k.beat(context.Background())
	record := k.snapshot()
	if record.Shell == nil || record.Shell.OK || record.Shell.Class != receipt.ShellReadOnly {
		t.Fatalf("shell %+v", record.Shell)
	}
	if notes := receipt.TakeShell(dir, "api", k.epoch); len(notes) != 0 {
		t.Fatalf("the keeper left %d observations behind", len(notes))
	}
}

func TestNothingIsToldAfterTheHarnessExits(t *testing.T) {
	dir := stateDir(t)
	k := failingKeeper(t, dir, "api", true)
	k.exited()
	k.beat(context.Background())
	if got := availabilityFiles(t, dir, "api"); len(got) != 0 {
		t.Fatalf("told after the exit: %+v", got)
	}
}
