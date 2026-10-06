package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// announcedUnread leaves a letter where a serving process leaves one it has
// announced: readable, and no longer waiting.
func announcedUnread(t *testing.T, dir string, message Message) Message {
	t.Helper()
	if err := state.EnsureSubdir(state.InboxPath(dir, message.To)); err != nil {
		t.Fatal(err)
	}
	if err := state.EnsureSubdir(state.UnreadPath(dir, message.To)); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(message)
	if err := os.WriteFile(filepath.Join(state.UnreadPath(dir, message.To), message.ID+".json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return message
}

// A letter being read in parts is final for its sender: part of it may be in
// front of the reader already. Once read to the end, the claim goes with it.
func TestALetterBeingReadInPartsCannotBeTakenBack(t *testing.T) {
	dir := stateDir(t)
	claimedTask := announcedUnread(t, dir, Message{ID: NewID(), From: "web", FromEpoch: "w1", To: "api", ToEpoch: "e1", Kind: Task, Text: "long", CreatedAt: time.Now()})
	free := announcedUnread(t, dir, Message{ID: NewID(), From: "web", To: "api", ToEpoch: "e1", Kind: Task, Text: "short", CreatedAt: time.Now()})
	if err := ClaimRead(dir, "api", claimedTask.ID, "token"); err != nil {
		t.Fatal(err)
	}
	if token, ok := ReadClaimed(dir, "api", claimedTask.ID); !ok || token != "token" {
		t.Fatalf("claim = %q %v", token, ok)
	}
	_, err := withdrawNow(t, dir, claimedTask, nil)
	if !errors.Is(err, ErrReadInProgress) || !errors.Is(err, ErrAlreadyRead) {
		t.Fatalf("withdrawing a letter in progress = %v", err)
	}
	if _, err := withdrawNow(t, dir, free, nil); err != nil {
		t.Fatalf("an unclaimed letter could not be withdrawn: %v", err)
	}
	if senders, err := ClaimedOwedBy(dir, "api", "e1"); err != nil || len(senders) != 1 || senders[0] != "web" {
		t.Fatalf("owed by = %v", senders)
	}
	if err := MarkRead(dir, "api", "e1", claimedTask, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadClaimed(dir, "api", claimedTask.ID); ok {
		t.Fatal("the claim outlived the read")
	}
	if senders, err := ClaimedOwedBy(dir, "api", "e1"); err != nil || len(senders) != 0 {
		t.Fatalf("a read letter still counts in progress: %v", senders)
	}
}

// The sweep leaves a letter in progress however old: it must not become
// replaceable, and its reader may come back for the rest.
func TestTheSweepKeepsALetterBeingRead(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	letter := announcedUnread(t, dir, Message{ID: NewID(), From: "web", To: "api", ToEpoch: run, Text: "long", CreatedAt: time.Now()})
	if err := ClaimRead(dir, "api", letter.ID, "token"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-keepFinished - time.Hour)
	path := filepath.Join(state.UnreadPath(dir, "api"), letter.ID+".json")
	_ = os.Chtimes(path, old, old)
	_ = os.Chtimes(filepath.Join(claimsPath(dir, "api"), letter.ID), old, old)
	(&Server{Dir: dir, Name: "api", Epoch: run}).sweepFinished()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the letter in progress was swept: %v", err)
	}
	if _, ok := ReadClaimed(dir, "api", letter.ID); !ok {
		t.Fatal("the claim on an unread letter was swept")
	}
}

// A heads-up published once stays published once: a retry after the letter
// was read, or after the sweep removed it, writes nothing.
func TestALetterIsPublishedOnceAcrossReadAndSweep(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	letter := Message{ID: NewID(), From: "web", To: "api", ToEpoch: run, Kind: Note, Text: "heads-up", CreatedAt: time.Now()}
	publish := func() bool {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		wrote, err := PublishOnce(ctx, dir, letter, nil)
		if err != nil {
			t.Fatal(err)
		}
		return wrote
	}
	if !publish() {
		t.Fatal("the first publication wrote nothing")
	}
	if publish() {
		t.Fatal("a second publication wrote the letter again")
	}
	// Gone from every stage, as after a day's sweep: the mark still answers.
	_ = os.Remove(filepath.Join(state.InboxPath(dir, "api"), letter.ID+".json"))
	if publish() || isPresent(t, dir, letter.ID) {
		t.Fatal("a retry after the sweep wrote the letter again")
	}
	// A run that has ended takes no letter, and its marks are not the live
	// run's to keep.
	other := letter
	other.ToEpoch = "e0"
	if _, err := PublishOnce(context.Background(), dir, other, nil); !errors.Is(err, ErrRecipientEnded) {
		t.Fatalf("a letter to an ended run: %v", err)
	}
	writeRaw(t, filepath.Join(state.InboxPath(dir, "api"), "once", "e0", other.ID), oncePublished)
	sweepOnce(dir, "api", run)
	if _, err := os.Stat(filepath.Join(state.InboxPath(dir, "api"), "once", "e0")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the marks of an ended run stay")
	}
}

// An intent whose letter never landed is taken up again; one whose letter did
// is only settled.
func TestAnIntentWithoutItsLetterIsWrittenAgain(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	letter := Message{ID: NewID(), From: "web", To: "api", ToEpoch: run, Kind: Note, Text: "heads-up", CreatedAt: time.Now()}
	path, _ := oncePath(dir, "api", run, letter.ID)
	_ = state.EnsureSubdir(state.InboxPath(dir, "api"))
	_ = state.EnsureSubdir(filepath.Dir(filepath.Dir(path)))
	_ = state.EnsureSubdir(filepath.Dir(path))
	if err := os.WriteFile(path, []byte(onceIntent), 0o600); err != nil {
		t.Fatal(err)
	}
	if wrote, err := PublishOnce(context.Background(), dir, letter, nil); err != nil || !wrote || !isPresent(t, dir, letter.ID) {
		t.Fatalf("an unfinished publication stayed unfinished: %v %v", wrote, err)
	}
	if mark, _ := os.ReadFile(path); string(mark) != oncePublished {
		t.Fatalf("mark = %q", mark)
	}
	// The sweep settles an intent before removing the letter it names.
	_ = os.WriteFile(path, []byte(onceIntent), 0o600)
	settleOnce(dir, "api", run, letter.ID)
	if mark, _ := os.ReadFile(path); string(mark) != oncePublished {
		t.Fatalf("settled mark = %q", mark)
	}
}

// A retried mark of an earlier call writes over nothing: each call keeps its
// own mark, and the later one decides the turn whichever is written last.
func TestARetriedMarkLeavesANewerOne(t *testing.T) {
	dir := stateDir(t)
	older := MarkName(150, NewID())
	mark(t, dir, "e1", "newer", 200)
	if err := MarkPending(dir, "api", "e1", older, "older", 150); err != nil {
		t.Fatal(err)
	}
	if text, ok := within(t, dir, "e1", 100, 300); !ok || text != "newer" {
		t.Fatalf("mark = %q %v", text, ok)
	}
	mark(t, dir, "e0", "earlier run", 900)
	if text, ok := within(t, dir, "e1", 100, 300); !ok || text != "newer" {
		t.Fatalf("a mark of an ended run changed this one: %q %v", text, ok)
	}
}

// A late failure of a letter being read in parts is recorded as delivered and
// leaves the letter where its reader is: only a read takes it out of unread/.
// A failed status an earlier run left behind does not take it out either.
func TestTheServerKeepsALetterBeingRead(t *testing.T) {
	dir := stateDir(t)
	letter := announcedUnread(t, dir, Message{ID: NewID(), From: "web", FromEpoch: "w1", To: "api", ToEpoch: "e1", Kind: Task, Text: "long", CreatedAt: time.Now()})
	if err := ClaimRead(dir, "api", letter.ID, "token"); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", Epoch: "e1", outcomes: map[string]Result{}}
	outcome, err := server.recordLocked(letter.ID, Result{State: Failed, Detail: "late delivery failed"})
	if err != nil {
		t.Fatal(err)
	}
	server.settleOutcome(letter.ID, outcome, false)
	if !stillUnread(dir, "api", letter.ID) {
		t.Fatal("the server took a letter being read out of unread/")
	}
	if status, _, _ := ReadStatus(dir, "api", letter.ID); status.State != Delivered {
		t.Fatalf("its sender reads %s", status.State)
	}
	settle(dir, "api", letter.ID, Failed)
	if !stillUnread(dir, "api", letter.ID) {
		t.Fatal("an old failed status took a letter being read out of unread/")
	}
}

// An intent the sweep could not settle keeps its letter, the one proof it
// was written: without it a retry would write the letter again.
func TestTheSweepKeepsALetterWhoseIntentItCouldNotSettle(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	letter := announcedUnread(t, dir, Message{ID: NewID(), From: "web", To: "api", ToEpoch: run, Kind: Note, Text: "old", CreatedAt: time.Now()})
	path, _ := oncePath(dir, "api", run, letter.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(onceIntent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(path), 0o700) })
	old := time.Now().Add(-keepFinished - time.Hour)
	_ = os.Chtimes(filepath.Join(state.UnreadPath(dir, "api"), letter.ID+".json"), old, old)
	(&Server{Dir: dir, Name: "api", Epoch: run}).sweepFinished()
	_ = os.Chmod(filepath.Dir(path), 0o700)
	wrote, err := PublishOnce(context.Background(), dir, letter, nil)
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Fatal("a retry after an unsettled sweep wrote the letter again")
	}
}

// What a mailbox shows of a letter published once to a run: the letter or a
// published mark, an intent alone, or nothing left.
func TestPublicationOfReadsTheMailbox(t *testing.T) {
	dir := stateDir(t)
	written := announcedUnread(t, dir, Message{ID: NewID(), From: "web", To: "api", ToEpoch: "e1", Kind: Note, Text: "here", CreatedAt: time.Now()})
	intended, marked, forgotten := NewID(), NewID(), NewID()
	for id, mark := range map[string]string{intended: onceIntent, marked: oncePublished} {
		path, _ := oncePath(dir, "api", "e1", id)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(mark), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for id, want := range map[string]Publication{written.ID: PublicationWritten, marked: PublicationWritten, intended: PublicationAbsent, forgotten: PublicationUnknown} {
		if got := publicationOf(t, dir, id); got != want {
			t.Errorf("%s: %d, want %d", id, got, want)
		}
	}
}

// A check that fails under the recipient's lock writes nothing, not even the
// intent.
func TestPublishOnceWritesNothingWhenItsCheckFails(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	letter := Message{ID: NewID(), From: "web", To: "api", ToEpoch: run, Kind: Note, Text: "late", CreatedAt: time.Now()}
	late := errors.New("the deadline passed")
	if _, err := PublishOnce(context.Background(), dir, letter, func() error { return late }); !errors.Is(err, late) {
		t.Fatalf("got %v", err)
	}
	if publication, err := PublicationOf(context.Background(), dir, "api", run, letter.ID); err != nil || isPresent(t, dir, letter.ID) || publication != PublicationUnknown {
		t.Fatal("a refused publication left a trace")
	}
}

// isPresent is present for a mailbox that can be searched.
func isPresent(t *testing.T, dir, id string) bool {
	t.Helper()
	found, err := present(dir, "api", id)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// publicationOf is PublicationOf for a mailbox that can be read.
func publicationOf(t *testing.T, dir, id string) Publication {
	t.Helper()
	got, err := PublicationOf(context.Background(), dir, "api", "e1", id)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// A stage of the mailbox that cannot be searched may hold the letter: an intent
// beside it proves nothing, nothing is written a second time, and the lookup
// says it could not tell.
func TestAMailboxThatCannotBeSearchedProvesNothing(t *testing.T) {
	dir := stateDir(t)
	letter := announcedUnread(t, dir, Message{ID: NewID(), From: "web", To: "api", ToEpoch: "e1", Kind: Note, Text: "already published", CreatedAt: time.Now()})
	unmarked := announcedUnread(t, dir, Message{ID: NewID(), From: "web", To: "api", ToEpoch: "e1", Kind: Note, Text: "no mark", CreatedAt: time.Now()})
	path, _ := oncePath(dir, "api", "e1", letter.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(onceIntent), 0o600); err != nil {
		t.Fatal(err)
	}
	unread := state.UnreadPath(dir, "api")
	if err := os.Chmod(unread, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unread, 0o700) })
	if got, err := PublicationOf(context.Background(), dir, "api", "e1", letter.ID); err == nil || got == PublicationAbsent {
		t.Fatalf("an unsearchable mailbox answered %d, %v", got, err)
	}
	if wrote, err := PublishOnce(context.Background(), dir, letter, nil); wrote || err == nil {
		t.Fatalf("published again beside a letter it could not see: %v %v", wrote, err)
	}
	// Nothing is written on a guess, not even the intent.
	if wrote, err := PublishOnce(context.Background(), dir, unmarked, nil); wrote || err == nil {
		t.Fatalf("published beside a letter it could not see: %v %v", wrote, err)
	}
	if mark, _ := oncePath(dir, "api", "e1", unmarked.ID); fileExists(mark) {
		t.Fatal("an intent was written for a letter that may be there")
	}
	if err := PutOnce(dir, letter); err == nil {
		t.Fatal("PutOnce wrote beside a letter it could not see")
	}
	if _, err := os.Stat(filepath.Join(state.InboxPath(dir, "api"), letter.ID+".json")); err == nil {
		t.Fatal("a second, waiting copy was written")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
