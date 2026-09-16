package inbox

import (
	"context"
	"testing"
	"time"
)

func TestAnAnswerToAnotherQuestionIsNotConsumed(t *testing.T) {
	dir := stateDir(t)
	report := message("answer for a different question")
	report.Kind = Finished
	report.InReplyTo = []string{"different-question"}
	report.ToEpoch = "5.5"
	unread(t, dir, report)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	got, found, err := AwaitAnswer(ctx, dir, "api", "5.5", "my-question", func(Message) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatalf("consumed another question's report: %s", got.Text)
	}
}
