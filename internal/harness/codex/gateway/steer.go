package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/control"
)

// interruptedText is what a stopped outcome says when a main aborted the turn
// with `rewake interrupt`, the same words the Claude Code side sends.
func interruptedText(by string) string { return by + " interrupted this turn with rewake interrupt" }

// markHoldLimit bounds how long a compaction mark holds deliveries and main's
// wait. A compaction is a model call over the whole conversation: the live
// run took 4 to 9 seconds on a nearly empty one, and a long conversation takes
// longer. A delivery sent while it runs is refused by the server, so ending
// the hold early fails letters that would have gone moments later; and main's
// own wait for a compaction is 80 seconds, so one bound ends both together.
// Gateway.markHold shortens it for the tests.
const markHoldLimit = 80 * time.Second

func (g *Gateway) markLimit() time.Duration {
	if g.markHold > 0 {
		return g.markHold
	}
	return markHoldLimit
}

// compactionRefused lays aside a compaction the server refused, or whose
// request was never written: it cannot start. Called under c.mu.
func (c *connection) compactionRefused(thread string, sent uint64) {
	if marker := c.admitted.manual[thread]; marker != nil && marker.sent == sent {
		delete(c.admitted.manual, thread)
	}
	c.state.ops.refused(thread, sent)
}

// steered is the one interrupt a main asked for on this connection: the turn
// it aborts, as thread/turn, and who asked. A newer interrupt replaces it.
type steered struct{ id, by string }

// busyWith says why a compaction must not be sent now, or "" when nothing
// runs: a compaction still holding, a turn the record says runs — announced,
// or only acknowledged in a reply — a request of the terminal's that starts
// one, a delivery being admitted, or an operation the server accepted whose
// end has not been read. When in doubt it refuses: a false refusal costs main
// a second ask, a compaction sent into a turn aborts the turn. Codex's own compact() aborts the running turn
// rather than refusing, so the refusal has to be rewake's
// (docs/remote-control-codex.md). Called under c.mu while the caller holds the
// admission gate, so no request of the terminal's is admitted between this
// answer and the compaction's request.
func (c *connection) busyWith(thread string) string {
	if c.admitted.holding(thread) {
		return "a compaction is running"
	}
	if d := c.state.doing; d.running {
		if !d.announced {
			return "a turn the server started has not ended"
		}
		return "a turn is running"
	}
	for _, p := range c.state.pending {
		switch p.method {
		case "turn/start", "turn/steer", "review/start", "thread/compact/start":
			if p.target == "" || p.target == thread {
				return "a " + p.method + " from the terminal is in flight"
			}
		}
	}
	for _, p := range c.admitted.pending {
		if p.binding.Thread == thread {
			return "a delivery is being admitted"
		}
	}
	return c.state.ops.uncertain(thread)
}

// Compact compacts the selected conversation for a main's request and waits
// for the compaction's turn to end, within ctx. It is refused while anything
// runs. The compaction is marked manual as the terminal's /compact is, so
// main's answer goes by its turn's end, and the mark carries the request and
// the asker, so the telemetry counts it as theirs. Its turn is never published
// either way: it shows no proof of work (proof.go).
func (g *Gateway) Compact(ctx context.Context, request, by string) control.Answer {
	c := g.currentConnection()
	if c == nil {
		return control.Answer{Outcome: control.Failed, Detail: "no conversation is selected"}
	}
	if err := g.acquire(ctx); err != nil {
		return control.Answer{Outcome: control.Failed, Detail: err.Error()}
	}
	c.mu.Lock()
	binding := c.state.Binding
	if !binding.Ready || !g.owns(c) {
		c.mu.Unlock()
		<-g.gate
		return control.Answer{Outcome: control.Failed, Detail: "no conversation is selected: " + binding.Reason}
	}
	if why := c.busyWith(binding.Thread); why != "" {
		c.mu.Unlock()
		<-g.gate
		return control.Answer{Outcome: control.Refused, Reason: control.InTurn, Detail: why}
	}
	// Codex compacts a conversation that has run no turn, and spends a model
	// call on it; Claude Code refuses it as too short, and so does rewake.
	if c.state.fresh {
		c.mu.Unlock()
		<-g.gate
		return control.Answer{Outcome: control.Refused, Reason: control.NothingToCompact, Detail: "the conversation has run no turn"}
	}
	sent := c.state.ops.next()
	if !c.admitted.manualStart(binding.Thread, c.state.events, sent, binding.Generation, time.Now().Add(g.markLimit())) {
		c.mu.Unlock()
		<-g.gate
		return control.Answer{Outcome: control.Failed, Detail: "manual-scope capacity reached"}
	}
	marker := c.admitted.manual[binding.Thread]
	ended := make(chan struct{})
	marker.request, marker.by, marker.ended = request, by, ended
	c.state.ops.opened(binding.Thread, sent, "")
	if entry := c.observations.threads[binding.Thread]; entry != nil {
		marker.tokensBefore = entry.snapshot.ContextUsed
	}
	c.mu.Unlock()
	_, err := c.callReserved(ctx, binding, "thread/compact/start", map[string]any{})
	<-g.gate
	if err != nil {
		detail, refused := strings.CutPrefix(err.Error(), "native request refused: ")
		// Only a request never written, or one the server refused, is known
		// not to compact. Any other failure may still start the compaction:
		// the mark stays, holding deliveries until its bound, and its turn
		// is taken for main's, not for work.
		var never unsent
		if !refused && !errors.As(err, &never) {
			c.mu.Lock()
			marker.released, marker.ended = true, nil
			c.mu.Unlock()
			return control.Answer{Outcome: control.Failed, Detail: "no answer to the compaction request (" + detail + "); it may still start, and rewake list counts it if it does"}
		}
		c.mu.Lock()
		c.compactionRefused(binding.Thread, sent)
		c.mu.Unlock()
		return control.Answer{Outcome: control.Failed, Detail: detail}
	}
	select {
	case <-ended:
	case <-ctx.Done():
	case <-c.ctx.Done():
		return control.Answer{Outcome: control.Failed, Detail: "the connection ended during the compaction"}
	}
	return c.waitEnded(marker)
}

// waitEnded answers main when its wait ends: by the compaction's end when it
// has been answered — also when the wait ran out as it came — and otherwise
// as the wait ending first. The hold then ends with it, and the answer says
// whether the compaction's turn was seen at all.
func (c *connection) waitEnded(marker *manualWork) control.Answer {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !marker.answered {
		marker.released, marker.ended = true, nil
		if marker.turn == "" {
			return control.Answer{Outcome: control.Failed, Detail: "the compaction's turn was not seen to start when the wait ended; deliveries go on, and its turn is still not taken for work if it starts"}
		}
		return control.Answer{Outcome: control.Failed, Detail: "the compaction was started and had not ended when the wait did; it may still finish, and deliveries go on meanwhile"}
	}
	switch marker.status {
	case "completed":
		answer := control.Answer{Outcome: control.Done, TokensAfter: marker.after}
		if marker.after != nil {
			answer.TokensBefore = marker.tokensBefore
		}
		return answer
	case "interrupted":
		return control.Answer{Outcome: control.Failed, Detail: "the compaction was interrupted"}
	}
	return control.Answer{Outcome: control.Failed, Detail: marker.failure}
}

// Interrupt aborts the selected conversation's running turn, the one the
// record names, for a main's request. No turn running, or only a compaction,
// is refused without asking the server; the server's own refusal of a turn that ended meanwhile is
// passed on as the same. The stopped outcome of that turn names the asker.
func (g *Gateway) Interrupt(ctx context.Context, by string) control.Answer {
	c := g.currentConnection()
	if c == nil {
		return control.Answer{Outcome: control.Refused, Reason: control.NoTurn, Detail: "no conversation is selected"}
	}
	c.mu.Lock()
	binding := c.state.Binding
	if !binding.Ready || !g.owns(c) {
		c.mu.Unlock()
		return control.Answer{Outcome: control.Refused, Reason: control.NoTurn, Detail: "no conversation is selected: " + binding.Reason}
	}
	d := c.state.doing
	// The running turn is the compaction's when its item said so; while a
	// mark still holds and the running turn is unnamed, it may be.
	if marker := c.admitted.manual[binding.Thread]; marker != nil && (marker.turn != "" && d.turn == marker.turn || !marker.released && (!d.running || d.turn == "")) {
		c.mu.Unlock()
		return control.Answer{Outcome: control.Refused, Reason: control.NoTurn, Detail: "a compaction is running, not a turn"}
	}
	if !d.running {
		c.mu.Unlock()
		return control.Answer{Outcome: control.Refused, Reason: control.NoTurn}
	}
	// turn/interrupt takes the turn's id, and a resume without turns does
	// not say it; the turn's next event will.
	if d.turn == "" {
		c.mu.Unlock()
		return control.Answer{Outcome: control.Failed, Detail: "a turn is running whose id this connection has not seen yet; ask again in a moment, or stop it at the keyboard"}
	}
	turn := d.turn
	c.interrupted = steered{id: binding.Thread + "/" + turn, by: by}
	c.mu.Unlock()
	_, err := c.callReserved(ctx, binding, "turn/interrupt", map[string]any{"turnId": turn})
	if err == nil {
		return control.Answer{Outcome: control.Done}
	}
	c.mu.Lock()
	if c.interrupted.id == binding.Thread+"/"+turn {
		c.interrupted = steered{}
	}
	c.mu.Unlock()
	detail := strings.TrimPrefix(err.Error(), "native request refused: ")
	if strings.Contains(detail, "no active turn") || strings.Contains(detail, "expected active turn id") {
		return control.Answer{Outcome: control.Refused, Reason: control.NoTurn, Detail: detail}
	}
	return control.Answer{Outcome: control.Failed, Detail: detail}
}

// steeredText names the main in the stopped outcome of the turn it
// interrupted. Called under c.mu.
func (c *connection) steeredText(v *Completion) {
	if v.Kind == "stopped" && c.interrupted.id != "" && v.ID == c.interrupted.id {
		v.Text = interruptedText(c.interrupted.by)
		c.interrupted = steered{}
	}
}

// refuseTerminalCompact answers the terminal's thread/compact/start itself
// while a main's compaction of that thread runs, instead of passing it on:
// the server would abort main's compaction for it, and the marker would pass
// to the terminal's, leaving main's answer to fail. A second compaction right
// after the first compacts nothing more.
// Called under c.mu; returns the reply to queue, or nil to pass it on.
func (c *connection) refuseTerminalCompact(m meta, raw []byte) []byte {
	marker := c.admitted.manual[m.thread]
	if m.method != "thread/compact/start" || marker == nil || marker.by == "" || marker.released {
		return nil
	}
	reply, err := json.Marshal(map[string]any{
		"id":    json.RawMessage(field(raw, "id")),
		"error": map[string]any{"code": -32600, "message": "rewake: " + marker.by + " asked for a compaction with rewake compact and it is running; try again once it ends"},
	})
	if err != nil {
		return nil
	}
	return reply
}
