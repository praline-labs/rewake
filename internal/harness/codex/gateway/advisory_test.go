package gateway

import (
	"strings"
	"testing"
	"time"
)

// goalTurn is a turn the server starts with no request, as a goal's: no reply
// names it, and one that fails at its first model call has no item.
func goalTurn(status, failure string) []string {
	return []string{
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"active","activeFlags":[]}}}`, started("G"),
		`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"G","items":[],"status":"` + status + `","error":{"message":"` + failure + `"}}}}`,
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`,
	}
}

// A turn that ends with no proof of work is not dropped: it reaches the waiter
// as an advisory report that settles nothing, carrying what is known of it.
func TestATurnWithoutProofOfWorkIsAdvisory(t *testing.T) {
	cases := map[string]struct{ status, text string }{
		"failed":      {"failed", advisoryText + "; usage limit reached"},
		"interrupted": {"interrupted", advisoryText + "; the person at the keyboard stopped this turn"},
		"completed":   {"completed", advisoryText},
	}
	for name, want := range cases {
		t.Run("a goal's turn, "+name, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			out := callbacks(g)
			events(t, ui, native, goalTurn(want.status, "usage limit reached")...)
			publishedAs(t, out, "A/G/advisory", "stopped", want.text)
		})
	}
	t.Run("a turn whose reply was lost with the connection", func(t *testing.T) {
		g, ui, peers, path := setup(t)
		native := <-peers
		bindUI(t, g, ui, native)
		out := callbacks(g)
		write(t, ui, []byte(`{"id":30,"method":"turn/start","params":{"threadId":"A","input":[]}}`))
		_ = readWithin(t, native)
		next, peer := reconnect(t, g, ui, native, peers, path, "active")
		events(t, next, peer, completed("U", "completed"))
		publishedAs(t, out, "A/U/advisory", "stopped", advisoryText)
	})
}

// A compaction's turn has nothing to report, and is no outcome of anyone's
// task: one shown by its item publishes nothing, advisory or not.
func TestACompactionShownByItsItemPublishesNothing(t *testing.T) {
	for _, status := range []string{"completed", "failed", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			ranATurn(t, ui, native)
			out := callbacks(g)
			exchange(t, ui, native, `{"id":8,"method":"thread/compact/start","params":{"threadId":"A"}}`, `{"id":8,"result":{}}`)
			events(t, ui, native, started("C"), compactionItem("C", "item/started"), completed("C", status))
			nothingPublished(t, out)
		})
	}
}

// A proof that comes after the advisory report still publishes the turn's own
// outcome, of any kind, a stopped one included: the advisory has an identity of
// its own, so neither is taken for the other.
func TestAProofAfterTheAdvisoryPublishesTheOutcome(t *testing.T) {
	cases := map[string]struct{ status, kind, text string }{
		"finished": {"completed", "finished", ""},
		"stopped":  {"interrupted", "stopped", "the person at the keyboard stopped this turn"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			out := callbacks(g)
			write(t, ui, []byte(`{"id":30,"method":"turn/start","params":{"threadId":"A","input":[]}}`))
			_ = readWithin(t, native)
			events(t, ui, native, started("U"), completed("U", want.status))
			advisory := advisoryText
			if want.text != "" {
				advisory += "; " + want.text
			}
			publishedAs(t, out, "A/U/advisory", "stopped", advisory)
			write(t, native, []byte(`{"id":30,"result":{"turn":{"id":"U","items":[],"status":"inProgress"}}}`))
			_ = readWithin(t, ui)
			publishedAs(t, out, "A/U", want.kind, want.text)
		})
	}
}

// A turn still waiting for its proof when the connection ends, or pushed out
// by later ones, is reported as advisory rather than dropped.
func TestATurnWaitingForItsProofIsNotDropped(t *testing.T) {
	t.Run("the connection ends", func(t *testing.T) {
		g, ui, peers, _ := setup(t)
		g.proofHold = time.Minute
		native := <-peers
		bindUI(t, g, ui, native)
		out := callbacks(g)
		events(t, ui, native, goalTurn("failed", "network error")...)
		nothingPublished(t, out)
		_ = ui.conn.Close()
		_ = native.conn.Close()
		publishedAs(t, out, "A/G/advisory", "stopped", advisoryText+"; network error")
	})
	t.Run("sixteen more wait", func(t *testing.T) {
		c := &connection{admitted: newAdmittedWork(), state: newState("epoch", 1)}
		var out []Completion
		for i := range 17 {
			out = append(out, Completion{ID: "A/" + itoa(uint64(i)), Thread: "A", Kind: "finished"})
		}
		ready := c.proven(out, time.Now())
		if len(ready) != 1 || ready[0].ID != "A/0/advisory" || ready[0].Kind != "stopped" || !strings.HasPrefix(ready[0].Text, advisoryText) {
			t.Fatalf("ready %+v", ready)
		}
	})
}
