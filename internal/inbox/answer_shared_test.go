package inbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

func reserve(t *testing.T, dir, question string) func() {
	t.Helper()
	release, err := ReserveAnswer(dir, "api", question)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

func TestASharedAnswerSurvivesAnAbandonedConsumer(t *testing.T) {
	for _, reason := range []string{"output failed", "wait ended"} {
		t.Run(reason, func(t *testing.T) {
			dir := stateDir(t)
			first, second := reserve(t, dir, "q1"), reserve(t, dir, "q2")
			defer second()
			report := message("shared result")
			report.Kind = Finished
			report.InReplyTo = []string{"q1", "q2"}
			report.ToEpoch = "5.5"
			unread(t, dir, report)
			if reason == "output failed" {
				_, _, err := AwaitAnswer(context.Background(), dir, "api", "5.5", "q1", func(Message) error { return errors.New("output failed") })
				if err == nil {
					t.Error("output failure ignored")
				}
			}
			first()
			_, found, err := AwaitAnswer(context.Background(), dir, "api", "5.5", "q2", func(Message) error { return nil })
			if err != nil || !found {
				t.Fatalf("second answer: %v %v", found, err)
			}
			remaining, err := AvailableUnread(dir, "api", "5.5")
			if err != nil || len(remaining) != 1 || remaining[0].ID != report.ID {
				t.Fatalf("unreceived answer lost: %v %v", remaining, err)
			}
		})
	}
}

func TestAReservationLivesThroughDelivery(t *testing.T) {
	dir := stateDir(t)
	release := reserve(t, dir, "q1")
	defer release()
	mark := filepath.Join(state.AnsweringPath(dir, "api"), "q1")
	old := time.Now().Add(-2 * answeringFresh)
	if err := os.Chtimes(mark, old, old); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * answerPoll)
	for time.Now().Before(deadline) {
		info, err := os.Stat(mark)
		if err == nil && info.ModTime().After(old) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("reservation was not renewed while waiting for delivery")
}

func TestAnActiveAnswerDoesNotExpire(t *testing.T) {
	dir := stateDir(t)
	release := reserve(t, dir, "q1")
	defer release()
	report := message("long running question")
	report.Kind = Finished
	report.InReplyTo = []string{"q1"}
	report.CreatedAt = time.Now().Add(-2 * defaultTTL)
	if err := Put(dir, report); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Deliver: func(context.Context, Message) Result { t.Fatal("reserved answer announced"); return Result{} }}
	server.drain(context.Background())
	messages, err := PeekUnread(dir, "api", "")
	if err != nil || len(messages) != 1 {
		t.Fatalf("active answer expired: %v %v", messages, err)
	}
}
