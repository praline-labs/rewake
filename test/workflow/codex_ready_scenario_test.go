package workflow

import (
	"testing"
	"time"
)

// TestCodexConversationAccepted is the first scenario with a real harness.
//
// What it claims is about rewake, not about the shim: that the wrapper and its
// gateway take a freshly launched Codex session all the way to an accepted
// conversation, against a peer that answers the way the real server does. That
// path is exercised every day by hand and by nothing else automatically.
//
// It does not deliver anything — mail, reading and reports are the next
// scenario. Readiness is the claim here, and it is observed through what the
// session itself can see.
func TestCodexConversationAccepted(t *testing.T) {
	binary := enterScenario(t, "codex-conversation-accepted")

	c := Start(t, Spec{
		Name:    "codex-conversation-accepted",
		Harness: "codex",
		Observations: []string{
			"the session registers itself",
			"the gateway accepts a conversation",
			"the accepted conversation is the one the session was given",
		},
		Deadline: 60 * time.Second,
	})
	iso := Isolate(t, c, binary)
	session := startCodexSession(t, c, iso, "worker", "--main")
	defer func() {
		if err := session.stop(c); err != nil {
			t.Errorf("ending the session: %v", err)
		}
	}()

	_, ok := session.await(c, "the session to appear in its own listing", func(l listing) bool {
		_, _, _, found := l.find(session.name)
		return found
	})
	if !ok {
		c.Contradicted("the session registers itself", "it never appeared in `rewake list`")
		return
	}
	c.Observed("the session registers itself", "listed by rewake as its own session")

	state, ok := session.await(c, "the gateway to accept a conversation", func(l listing) bool {
		_, selection, thread, found := l.find(session.name)
		return found && selection != "" && selection != "unavailable" && thread != ""
	})
	if !ok {
		_, selection, thread, _ := state.find(session.name)
		c.Contradicted("the gateway accepts a conversation",
			"selection stayed %q with thread %q", selection, thread)
		return
	}
	_, selection, thread, _ := state.find(session.name)
	c.Observed("the gateway accepts a conversation", "selection "+selection)

	// Compared against what the server told this session's own client, not
	// against an id the test chose: a new conversation is named by the server,
	// and the client learns it from its reply.
	accepted := session.acceptedThread()
	if accepted == "" {
		c.Contradicted("the accepted conversation is the one the session was given",
			"the session never recorded a conversation id from its reply")
		return
	}
	if thread != accepted {
		c.Contradicted("the accepted conversation is the one the session was given",
			"telemetry reports %q, the server gave the client %q", thread, accepted)
		return
	}
	c.Observed("the accepted conversation is the one the session was given", thread)
}
