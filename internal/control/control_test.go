package control

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

var fast = Limits{Pickup: 300 * time.Millisecond, Outcome: 300 * time.Millisecond, Poll: 5 * time.Millisecond}

// serve plays the served side once, the way the module does: wait for the
// request, take it, answer it with what reply makes of it.
func serve(t *testing.T, dir string, reply func(Request) string) <-chan Request {
	t.Helper()
	seen := make(chan Request, 1)
	go func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			raw, err := os.ReadFile(RequestPath(dir))
			if err != nil {
				time.Sleep(2 * time.Millisecond)
				continue
			}
			var request Request
			if json.Unmarshal(raw, &request) != nil {
				t.Errorf("a request that does not parse: %q", raw)
				return
			}
			_ = os.WriteFile(TakenPath(dir, request.ID), nil, 0o600)
			if answer := reply(request); answer != "" {
				_ = os.WriteFile(AnswerPath(dir, request.ID), []byte(answer), 0o600)
			}
			seen <- request
			return
		}
		close(seen)
	}()
	return seen
}

func prepared(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "control", "worker.1.2")
	if err := Prepare(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

// left is what a request left in the directory besides the lock.
func left(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if entry.Name() != lockFile {
			names = append(names, entry.Name())
		}
	}
	return names
}

func TestAskCarriesTheRequestAndTheAnswerAndCleansUp(t *testing.T) {
	dir := prepared(t)
	seen := serve(t, dir, func(r Request) string {
		return `{"id":"` + r.ID + `","outcome":"done","tokensBefore":120000,"tokensAfter":9000}`
	})
	answer, err := Ask(context.Background(), dir, Request{Action: Compact, Focus: "keep the plan", From: "lead"}, fast)
	if err != nil {
		t.Fatal(err)
	}
	request := <-seen
	if !ValidID.MatchString(request.ID) || request.Action != Compact || request.Focus != "keep the plan" || request.From != "lead" {
		t.Fatalf("the served side read %+v", request)
	}
	if answer.Outcome != Done || answer.TokensBefore == nil || *answer.TokensBefore != 120000 || *answer.TokensAfter != 9000 {
		t.Fatalf("answer %+v", answer)
	}
	if rest := left(t, dir); len(rest) != 0 {
		t.Fatalf("left behind: %v", rest)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("the directory is %v, %v", info.Mode(), err)
	}
}

func TestTheRequestIsWrittenWholeAndPrivate(t *testing.T) {
	dir := prepared(t)
	modes := make(chan os.FileMode, 1)
	serve(t, dir, func(r Request) string {
		if info, err := os.Stat(RequestPath(dir)); err == nil {
			modes <- info.Mode().Perm()
		}
		return `{"id":"` + r.ID + `","outcome":"done"}`
	})
	if _, err := Ask(context.Background(), dir, Request{Action: Interrupt, From: "lead"}, fast); err != nil {
		t.Fatal(err)
	}
	if mode := <-modes; mode != 0o600 {
		t.Fatalf("the request is %v", mode)
	}
}

func TestNobodyTakingTheRequestIsNotAnswering(t *testing.T) {
	dir := prepared(t)
	started := time.Now()
	answer, err := Ask(context.Background(), dir, Request{Action: Compact, From: "lead"}, fast)
	if err != nil {
		t.Fatal(err)
	}
	if answer.Outcome != Refused || answer.Reason != NotAnswering {
		t.Fatalf("answer %+v", answer)
	}
	if elapsed := time.Since(started); elapsed < fast.Pickup || elapsed > fast.Pickup+time.Second {
		t.Fatalf("gave up after %s", elapsed)
	}
	// The request is gone: a module that loads later must not carry it out.
	if rest := left(t, dir); len(rest) != 0 {
		t.Fatalf("left behind: %v", rest)
	}
}

func TestATakenRequestWithoutAnOutcomeFailsAndTheNextAskClearsIt(t *testing.T) {
	dir := prepared(t)
	var first Request
	seen := serve(t, dir, func(Request) string { return "" })
	answer, err := Ask(context.Background(), dir, Request{Action: Compact, From: "lead"}, fast)
	if err != nil {
		t.Fatal(err)
	}
	first = <-seen
	if answer.Outcome != Failed || answer.Reason != "" {
		t.Fatalf("answer %+v", answer)
	}
	// The outcome arrives late, after the asker gave up.
	late := `{"id":"` + first.ID + `","outcome":"done"}`
	if err := os.WriteFile(AnswerPath(dir, first.ID), []byte(late), 0o600); err != nil {
		t.Fatal(err)
	}
	seen = serve(t, dir, func(r Request) string { return `{"id":"` + r.ID + `","outcome":"refused","reason":"in a turn"}` })
	answer, err = Ask(context.Background(), dir, Request{Action: Compact, From: "lead"}, fast)
	if err != nil {
		t.Fatal(err)
	}
	if second := <-seen; second.ID == first.ID || answer.Reason != InTurn {
		t.Fatalf("the second request %s after %s answered %+v", second.ID, first.ID, answer)
	}
	if rest := left(t, dir); len(rest) != 0 {
		t.Fatalf("left behind: %v", rest)
	}
}

func TestAnAnswerIsReadOnlyWholeAndOnlyForItsRequest(t *testing.T) {
	dir := prepared(t)
	serve(t, dir, func(r Request) string {
		// Half written, then another request's answer: neither is this one's.
		_ = os.WriteFile(AnswerPath(dir, r.ID), []byte(`{"id":"`+r.ID+`","outco`), 0o600)
		time.Sleep(30 * time.Millisecond)
		_ = os.WriteFile(AnswerPath(dir, r.ID), []byte(`{"id":"`+NewID()+`","outcome":"done"}`), 0o600)
		time.Sleep(30 * time.Millisecond)
		return `{"id":"` + r.ID + `","outcome":"refused","reason":"no turn running","detail":"$.turn.abort: no turn is running"}`
	})
	answer, err := Ask(context.Background(), dir, Request{Action: Interrupt, From: "lead"}, fast)
	if err != nil {
		t.Fatal(err)
	}
	if answer.Outcome != Refused || answer.Reason != NoTurn || answer.Detail != "$.turn.abort: no turn is running" {
		t.Fatalf("answer %+v", answer)
	}
}

func TestASecondAskerIsRefusedNotQueued(t *testing.T) {
	dir := prepared(t)
	holder, err := os.OpenFile(filepath.Join(dir, lockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Close() }()
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	answer, err := Ask(context.Background(), dir, Request{Action: Compact, From: "lead"}, fast)
	if err != nil || answer.Outcome != Refused || answer.Reason != Busy {
		t.Fatalf("answer %+v, %v", answer, err)
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatalf("waited %s for the lock", time.Since(started))
	}
	if _, err := os.Stat(RequestPath(dir)); err == nil {
		t.Fatal("a refused asker wrote a request")
	}
}

func TestNoDirectoryIsNoControl(t *testing.T) {
	_, err := Ask(context.Background(), filepath.Join(t.TempDir(), "gone"), Request{Action: Compact}, fast)
	if !errors.Is(err, ErrNoControl) {
		t.Fatalf("got %v", err)
	}
}

// markThenCheck plays the served side's rule from the moment it has read the
// request: mark it taken, then check it is still in place; carry it out only
// if so, and answer that it was withdrawn otherwise. It says whether it acted.
func markThenCheck(dir, id string) bool {
	_ = os.WriteFile(TakenPath(dir, id), nil, 0o600)
	raw, err := os.ReadFile(RequestPath(dir))
	var still Request
	if err == nil && json.Unmarshal(raw, &still) == nil && still.ID == id {
		_ = os.WriteFile(AnswerPath(dir, id), []byte(`{"id":"`+id+`","outcome":"done"}`), 0o600)
		return true
	}
	_ = os.WriteFile(AnswerPath(dir, id), []byte(`{"id":"`+id+`","outcome":"refused","reason":"`+Withdrawn+`"}`), 0o600)
	return false
}

// withLastLook puts a served side's move into the instant between the asker
// withdrawing a request and its last look for the mark.
func withLastLook(t *testing.T, move func(dir, id string)) {
	t.Helper()
	saved := lastLook
	lastLook = move
	t.Cleanup(func() { lastLook = saved })
}

// A served side that read the request just as the asker gave up is answered
// honestly in every order of the two sides' steps: the asker withdraws and
// then looks for the mark, the served side marks and then looks for the
// request, so the one that looks second sees the other.
func TestARequestTakenAsTheAskerGivesUpIsHonestInEveryOrder(t *testing.T) {
	short := Limits{Pickup: 20 * time.Millisecond, Outcome: 300 * time.Millisecond, Poll: 5 * time.Millisecond}

	// Marked and checked before the withdrawal: carried out, and the asker,
	// finding the mark, waits for it.
	dir := prepared(t)
	withLastLook(t, func(dir, id string) {
		_ = os.WriteFile(TakenPath(dir, id), nil, 0o600)
		_ = os.WriteFile(AnswerPath(dir, id), []byte(`{"id":"`+id+`","outcome":"done"}`), 0o600)
	})
	if answer, err := Ask(context.Background(), dir, Request{Action: Compact, From: "lead"}, short); err != nil || answer.Outcome != Done {
		t.Fatalf("checked before the withdrawal: %+v, %v", answer, err)
	}

	// Marked before the asker's last look, checked after the withdrawal: not
	// carried out, and the asker reports exactly that.
	dir = prepared(t)
	acted := true
	withLastLook(t, func(dir, id string) { acted = markThenCheck(dir, id) })
	answer, err := Ask(context.Background(), dir, Request{Action: Compact, From: "lead"}, short)
	if err != nil || answer.Outcome != Refused || answer.Reason != Withdrawn || acted {
		t.Fatalf("marked in the instant: %+v, %v, acted %v", answer, err, acted)
	}
	if names := left(t, dir); len(names) != 0 {
		t.Fatalf("left behind %v", names)
	}

	// Marked after the asker's last look: the asker says nobody answered, and
	// the served side, finding the request gone, does nothing either.
	dir = prepared(t)
	var id string
	withLastLook(t, func(_, asked string) { id = asked })
	answer, err = Ask(context.Background(), dir, Request{Action: Interrupt, From: "lead"}, short)
	if err != nil || answer.Reason != NotAnswering {
		t.Fatalf("marked after the last look: %+v, %v", answer, err)
	}
	if markThenCheck(dir, id) {
		t.Fatal("a request the asker gave up on was carried out")
	}
	withLastLook(t, func(string, string) {})
	serve(t, dir, func(r Request) string { return `{"id":"` + r.ID + `","outcome":"done"}` })
	if answer, err := Ask(context.Background(), dir, Request{Action: Interrupt, From: "lead"}, fast); err != nil || answer.Outcome != Done {
		t.Fatalf("the next request after a withdrawn one: %+v, %v", answer, err)
	}
	if names := left(t, dir); len(names) != 0 {
		t.Fatalf("the withdrawn request's files outlived the next one: %v", names)
	}
}

// Once taken, the request stays in place until the answer: the served side
// checks it after marking it, and an asker that removed it at the mark would
// turn a check a poll later into a false "withdrawn".
func TestATakenRequestStaysInPlaceWhileItIsServed(t *testing.T) {
	dir := prepared(t)
	stayed := make(chan bool, 1)
	go func() {
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
			raw, err := os.ReadFile(RequestPath(dir))
			if err != nil {
				continue
			}
			var request Request
			if json.Unmarshal(raw, &request) != nil {
				break
			}
			_ = os.WriteFile(TakenPath(dir, request.ID), nil, 0o600)
			time.Sleep(5 * fast.Poll)
			_, err = os.Stat(RequestPath(dir))
			stayed <- err == nil
			markThenCheck(dir, request.ID)
			return
		}
		stayed <- false
	}()
	answer, err := Ask(context.Background(), dir, Request{Action: Compact, From: "lead"}, fast)
	if !<-stayed {
		t.Fatal("the request was gone while it was being served")
	}
	if err != nil || answer.Outcome != Done {
		t.Fatalf("answer %+v, %v", answer, err)
	}
	if names := left(t, dir); len(names) != 0 {
		t.Fatalf("left behind %v", names)
	}
}

// An asker cut short — its shell call interrupted — withdraws the request on
// the way out, so a session that stalls and resumes finds nothing to carry
// out.
func TestAnAskerCutShortWithdrawsTheRequest(t *testing.T) {
	dir := prepared(t)
	ctx, cancel := context.WithCancel(context.Background())
	slow := Limits{Pickup: 10 * time.Second, Outcome: 10 * time.Second, Poll: 5 * time.Millisecond}
	go func() {
		for !exists(RequestPath(dir)) {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	started := time.Now()
	answer, err := Ask(ctx, dir, Request{Action: Compact, From: "lead"}, slow)
	if err != nil || answer.Reason != NotAnswering || time.Since(started) > 2*time.Second {
		t.Fatalf("answer %+v, %v after %s", answer, err, time.Since(started))
	}
	if names := left(t, dir); len(names) != 0 {
		t.Fatalf("left behind %v", names)
	}
}
