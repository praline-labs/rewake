package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// A question blocks until the other session ends its turn, and prints its
// final message; the answer is taken from this session's mailbox, marked read.
func TestAQuestionPrintsTheAnswer(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "web")
	t.Setenv(epochEnv, web.Epoch())

	answered := make(chan string, 1)
	go func() {
		// The other side: wait for the question, then leave the report where
		// web's wrapper would put it once linked.
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			found, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, "api"), "*.json"))
			if len(found) == 1 {
				id := strings.TrimSuffix(filepath.Base(found[0]), ".json")
				answered <- rawUnread(t, dir, "web", map[string]any{
					"from": "api", "kind": "finished", "toEpoch": web.Epoch(),
					"inReplyTo": []string{id}, "text": "port 8088",
				})
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	code, out, errOut := run("send", "api", "which port?", "--question", "--wait", "10")
	if code != ExitOK || !strings.Contains(out, "answer from api:") || !strings.Contains(out, "port 8088") {
		t.Fatalf("exit = %d, out = %q, err = %q; want the answer printed", code, out, errOut)
	}
	report := <-answered
	if status, _ := inbox.ReadStatus(dir, "web", report); status.State != inbox.Read {
		t.Errorf("the answer's status is %q, want read: it was handed over, not left to announce", status.State)
	}
	marks, _ := os.ReadDir(state.AnsweringPath(dir, "web"))
	if len(marks) != 0 {
		t.Errorf("the waiting mark was left behind: %v", marks)
	}
}

var idThenNoAnswer = regexp.MustCompile(`^id \S+\nRewake: no answer from api yet; it arrives later as its report\n$`)

// Without an answer in time the question stays open, and says how the answer
// will arrive.
func TestAnUnansweredQuestionStaysOpen(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "web")
	t.Setenv(epochEnv, web.Epoch())

	code, out, _ := run("send", "api", "which port?", "--question", "--wait", "0.5")
	// The id comes first, before the wait: an edit or an addendum takes it.
	if code != ExitPending || !idThenNoAnswer.MatchString(out) {
		t.Errorf("exit = %d, out = %q; want pending with how the answer will come", code, out)
	}
	waiting, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, "api"), "*.json"))
	if len(waiting) != 1 {
		t.Fatalf("api holds %v, want the question", waiting)
	}
	raw, _ := os.ReadFile(waiting[0])
	if !strings.Contains(string(raw), `"kind": "question"`) {
		t.Errorf("message = %s, want kind question", raw)
	}
}
