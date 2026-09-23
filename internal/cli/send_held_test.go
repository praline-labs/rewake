package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// holdEverything makes the recipient's harness hold whatever it is sent, and
// calls then with the message id once the hold is reported.
func holdEverything(t *testing.T, then func(id string)) {
	t.Helper()
	previous := awaitStatus
	t.Cleanup(func() { awaitStatus = previous })
	awaitStatus = func(_ string, _ string, id string, _ time.Duration) (inbox.Status, bool) {
		if then != nil {
			then(id)
		}
		return inbox.Status{State: inbox.Held, Via: "socket", Detail: "the session holds the notice"}, true
	}
}

// A held message was accepted and not delivered: exit 3, and it says held.
func TestAHeldSendIsNotReportedDelivered(t *testing.T) {
	liveSession(t, "api")
	holdEverything(t, nil)
	code, out, errOut := run("send", "api", "rerun the smoke")
	if code != 3 {
		t.Fatalf("exit %d, want 3; stdout %q stderr %q", code, out, errOut)
	}
	if !strings.HasPrefix(out, "Rewake: held for api: the session holds the notice") {
		t.Fatalf("stdout %q", out)
	}
	code, out, _ = run("send", "api", "rerun the smoke", "--json")
	var model struct {
		State string `json:"state"`
	}
	if code != 3 || json.Unmarshal([]byte(out), &model) != nil || model.State != string(inbox.Held) {
		t.Fatalf("exit %d, json %q", code, out)
	}
}

// A hold outlives nothing: the session that ended can never release it, and
// held there would promise a delivery nobody makes.
func TestAHoldInAnEndedSessionFails(t *testing.T) {
	dir := liveSession(t, "api")
	holdEverything(t, func(string) {
		if err := os.Remove(state.SessionPath(dir, "api")); err != nil {
			t.Error(err)
		}
	})
	code, out, errOut := run("send", "api", "rerun the smoke")
	if code != 1 || !strings.Contains(out+errOut, "the session ended") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

// A question whose hold ends in failure stops waiting for an answer that is
// not coming, and says it failed.
func TestAQuestionStopsWaitingWhenItsHoldFails(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "web")
	t.Setenv(epochEnv, web.Epoch())
	holdEverything(t, func(id string) {
		go func() {
			time.Sleep(300 * time.Millisecond)
			status, _ := json.Marshal(inbox.Status{State: inbox.Failed, Via: "socket", Detail: "the hold expired", At: time.Now()})
			if err := state.WriteAtomic(filepath.Join(state.InboxPath(dir, "api"), id+".status"), status); err != nil {
				t.Error(err)
			}
		}()
	})
	started := time.Now()
	code, out, errOut := run("send", "api", "which port?", "--question", "--wait", "20")
	if waited := time.Since(started); waited > 5*time.Second {
		t.Fatalf("the question waited %v for an answer that was not coming", waited)
	}
	if code != 1 || !strings.Contains(out+errOut, "the hold expired") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}
