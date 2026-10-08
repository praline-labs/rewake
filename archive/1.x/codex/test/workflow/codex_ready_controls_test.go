package workflow

import (
	"testing"
	"time"
)

// The scenario's negative controls. Without them it would only show that two
// halves of our own making can agree with each other: a shim that always says
// yes and a gateway that always believes it would look exactly as green.
//
// Each control makes the shim answer like a server that is wrong in one
// specific way. Readiness must not follow. If it does, that is a finding about
// the gateway — the shim is not to be made more agreeable to fix it.

func TestReadinessNeedsDirectInputAccepted(t *testing.T) {
	refusedReadiness(t, "no-direct-input",
		"a server that never said the conversation takes direct input",
		shimNoDirectInput+"=1")
}

// Only a continuation names a conversation the client chose, so that is where
// "the server answered about a different one" can be asked at all. A new
// conversation is named by the server, and there is nothing to disagree with.
func TestReadinessNeedsTheConversationThatWasAskedFor(t *testing.T) {
	refusedReadiness(t, "wrong-thread", "a server that answered about a different conversation",
		shimResume+"=1", shimWrongThread+"=1")
}

// "A conversation started" is not "your request was accepted". A server that
// announces one without answering the request must not produce readiness.
func TestReadinessNeedsACorrelatedReply(t *testing.T) {
	refusedReadiness(t, "no-correlated-reply",
		"a server that announced a conversation without answering the request",
		shimNoCorrelatedReply+"=1")
}

// refusedReadiness launches a session whose shim answers wrongly and requires
// that the gateway never reports a selection.
func refusedReadiness(t *testing.T, name, description string, controls ...string) {
	t.Helper()
	binary := enterScenario(t, "readiness-control-"+name)

	c := Start(t, Spec{
		Name:    "readiness-control-" + name,
		Harness: "codex",
		Observations: []string{
			"the session registers itself",
			"the gateway refuses to select a conversation",
		},
		Deadline: 60 * time.Second,
	})
	iso := Isolate(t, c, binary)
	session := startCodexSession(t, c, iso, "worker", "--main", controls...)
	defer func() {
		if err := session.stop(c); err != nil {
			t.Errorf("ending the session: %v", err)
		}
	}()

	// Registration first, but for what it is seen *through*: the state file
	// only exists because the shim's client half ran `rewake list` after its
	// thread/start, so a listing proves the wire was exercised. Registration
	// itself happens before the harness is even launched, and would be true of
	// a session whose harness never started.
	if _, ok := session.await(c, "the session to appear in its own listing", func(l listing) bool {
		_, _, _, found := l.find(session.name)
		return found
	}); !ok {
		c.Contradicted("the session registers itself", "it never appeared in `rewake list`")
		return
	}
	c.Observed("the session registers itself", "listed by rewake as its own session")

	// Then a bounded window in which readiness must not appear. The shim has
	// already answered by the time it is registered, so this is waiting for a
	// state change that should never come, not for slow work.
	c.Note("watching for a selection that should not appear")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, selection, thread, found := session.latest().find(session.name); found && selection == "ready" {
			c.Contradicted("the gateway refuses to select a conversation",
				"selection became %q on thread %q against %s", selection, thread, description)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	c.Observed("the gateway refuses to select a conversation", description)
}
