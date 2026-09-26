package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/buildtime"
	"github.com/iiiokojiadbi/rewake/internal/control"
)

// interruptedText is what a stopped outcome says when a main aborted the turn
// with `rewake interrupt`, the same words the Claude Code side sends.
func interruptedText(by string) string { return by + " interrupted this turn with rewake interrupt" }

// The bounds of a compaction mark. A compaction is a model call over the
// whole conversation: 4 to 9 seconds live on a nearly empty one, and 104
// seconds from the request on one whose context was 85 per cent full
// (docs/research-codex.md). The
// server refuses a delivery sent while it runs, so a hold that ends early only
// turns letters into refusals.
//
// markHoldLimit bounds the hold while the compaction's turn has not been seen
// to start: nothing then shows that anything runs. compactionRunLimit bounds it
// once its item has tied the mark to its turn, which shows the compaction
// running until that turn ends; it is also how long the wrapper waits for the
// end on main's behalf, so one bound ends both. Ten minutes is several times
// the longest compaction seen, and main's wrapper waits longer still for its
// letter (internal/wrap, letterBound). A build may set either (see
// package buildtime); Gateway.markHold and Gateway.runHold shorten them for the
// tests.
var (
	markHoldLimit      = buildtime.Duration("builtCompactionStart", builtCompactionStart, 80*time.Second)
	compactionRunLimit = buildtime.Duration("builtCompactionRun", builtCompactionRun, 10*time.Minute)
)

var builtCompactionStart, builtCompactionRun string

func (g *Gateway) markLimit() time.Duration {
	if g.markHold > 0 {
		return g.markHold
	}
	return markHoldLimit
}

func (g *Gateway) runLimit() time.Duration {
	if g.runHold > 0 {
		return g.runHold
	}
	return compactionRunLimit
}

// CompactionWait is how long the wrapper waits for the end of a compaction a
// main asked for: the bound of its running mark.
func (g *Gateway) CompactionWait() time.Duration { return g.runLimit() }

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

// Compact compacts the selected conversation for a main's request. It is
// refused while anything runs. The compaction is marked manual as the
// terminal's /compact is, so main's letter goes by its turn's end, and the mark
// carries the request and the asker, so the telemetry counts it as theirs. Its
// turn is never published either way: it shows no proof of work (proof.go).
//
// It answers as soon as the request's fate is known, and never waits for the
// end: refused or failed before anything started; started once the
// compaction's own item has tied the mark to its turn, which its end needs
// too; requested when no tie came within start, or when the wait for the end
// ended before one — the hold's bound, or sight lost. The server's reply to the request
// is not the start: it comes once the compaction is queued. When the answer is
// started or requested, later waits for the end within ctx and answers it, for
// main's letter (docs/remote-control-codex.md); otherwise later is nil.
func (g *Gateway) Compact(ctx context.Context, request, by string, start time.Duration) (control.Answer, func() control.Answer) {
	answer, later := g.compact(ctx, request, by, start)
	// A final answer is the compaction's outcome too: a command whose wait was
	// cut short once the request was taken holds an open answer, and main's
	// letter comes from this at once rather than at its bound. A command that
	// read the answer has closed its record, so nothing is sent.
	if later == nil && !answer.Open {
		g.CompactionEnded(request, by, answer)
	}
	return answer, later
}

func (g *Gateway) compact(ctx context.Context, request, by string, start time.Duration) (control.Answer, func() control.Answer) {
	c := g.currentConnection()
	if c == nil {
		return control.Answer{Outcome: control.Failed, Detail: "no conversation is selected"}, nil
	}
	admit, cancel := context.WithTimeout(ctx, start)
	err := g.acquire(admit)
	cancel()
	if err != nil {
		return control.Answer{Outcome: control.Failed, Detail: "the gateway admitted nothing within " + start.String() + ": " + err.Error()}, nil
	}
	c.mu.Lock()
	binding := c.state.Binding
	if !binding.Ready || !g.owns(c) {
		c.mu.Unlock()
		<-g.gate
		return control.Answer{Outcome: control.Failed, Detail: "no conversation is selected: " + binding.Reason}, nil
	}
	if why := c.busyWith(binding.Thread); why != "" {
		c.mu.Unlock()
		<-g.gate
		return control.Answer{Outcome: control.Refused, Reason: control.InTurn, Detail: why}, nil
	}
	// Codex compacts a conversation that has run no turn, and spends a model
	// call on it; Claude Code refuses it as too short, and so does rewake.
	if c.state.fresh {
		c.mu.Unlock()
		<-g.gate
		return control.Answer{Outcome: control.Refused, Reason: control.NothingToCompact, Detail: "the conversation has run no turn"}, nil
	}
	sent := c.state.ops.next()
	if !c.admitted.manualStart(binding.Thread, c.state.events, sent, binding.Generation, time.Now()) {
		c.mu.Unlock()
		<-g.gate
		return control.Answer{Outcome: control.Failed, Detail: "manual-scope capacity reached"}, nil
	}
	marker := c.admitted.manual[binding.Thread]
	ended, tied := make(chan struct{}), make(chan struct{})
	marker.request, marker.by, marker.ended, marker.tied = request, by, ended, tied
	c.state.ops.opened(binding.Thread, sent, "")
	if entry := c.observations.threads[binding.Thread]; entry != nil {
		marker.tokensBefore = entry.snapshot.ContextUsed
	}
	c.mu.Unlock()
	replied := make(chan error, 1)
	go func() {
		_, err := c.callReserved(ctx, binding, "thread/compact/start", map[string]any{})
		<-g.gate
		replied <- err
	}()
	bound := time.NewTimer(start)
	defer bound.Stop()
	heard, reply := false, replied
	answer := control.Answer{Outcome: control.Started}
wait:
	for {
		select {
		case err := <-reply:
			if err != nil {
				return c.compactUnsent(binding.Thread, sent, marker, err), nil
			}
			heard, reply = true, nil
		case <-tied:
			break wait
		case <-ended:
			// An end before the tie is no start: the hold's bound or the
			// gateway losing sight of it ended the wait (lostSight).
			c.mu.Lock()
			if marker.turn == "" {
				answer = control.Answer{Outcome: control.Requested, Detail: "its start was not seen: " + marker.failure}
			}
			c.mu.Unlock()
			break wait
		case <-bound.C:
			answer = control.Answer{Outcome: control.Requested, Detail: "the server has not answered the request within " + start.String()}
			if heard {
				answer.Detail = "the server took the request, and its turn was not seen to start within " + start.String()
			}
			break wait
		case <-ctx.Done():
			// The wait for the end is over before the start was seen: later
			// answers how it ended.
			answer = control.Answer{Outcome: control.Requested, Detail: "the wait for it ended before its start was seen"}
			break wait
		case <-c.ctx.Done():
			// The request went out: the server may still compact.
			return control.Answer{Outcome: control.Failed, Detail: "the connection ended during the compaction", Open: true}, nil
		}
	}
	later := func() control.Answer {
		if !heard {
			select {
			case err := <-replied:
				if err != nil {
					return c.compactUnsent(binding.Thread, sent, marker, err)
				}
			case <-c.ctx.Done():
				return control.Answer{Outcome: control.Failed, Detail: "the connection ended during the compaction"}
			}
		}
		select {
		case <-ended:
		case <-ctx.Done():
		case <-c.ctx.Done():
			return control.Answer{Outcome: control.Failed, Detail: "the connection ended during the compaction"}
		}
		return c.waitEnded(marker)
	}
	return answer, later
}

// compactUnsent answers a compaction whose request failed. Only a request
// never written, or one the server refused, is known not to compact. Any
// other failure may still start the compaction: the mark stays, holding
// deliveries until its bound, and its turn is taken for main's, not for work.
func (c *connection) compactUnsent(thread string, sent uint64, marker *manualWork, err error) control.Answer {
	detail, refused := strings.CutPrefix(err.Error(), "native request refused: ")
	var never unsent
	c.mu.Lock()
	defer c.mu.Unlock()
	if !refused && !errors.As(err, &never) {
		marker.released, marker.ended = true, nil
		return control.Answer{Outcome: control.Failed, Detail: "no answer to the compaction request (" + detail + "); it may still start, and rewake list counts it if it does", Open: true}
	}
	c.compactionRefused(thread, sent)
	return control.Answer{Outcome: control.Failed, Detail: detail}
}

// waitEnded answers main when its wait ends: by how the mark's wait ended when
// it has — the compaction's end, also when the wait ran out as it came, or a
// bound — and otherwise as the wait ending first. The hold then ends with it,
// and the answer says whether the compaction's turn was seen at all; one seen
// is still running, and its end is main's outcome when seen.
func (c *connection) waitEnded(marker *manualWork) control.Answer {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !marker.answered {
		marker.released = true
		if marker.turn == "" {
			marker.ended = nil
			return control.Answer{Outcome: control.Failed, Detail: "the compaction's turn was not seen to start when the wait ended; deliveries go on, and its turn is still not taken for work if it starts"}
		}
		marker.stillRunning("the compaction was not seen to end when the wait did")
	}
	return marker.outcome()
}

// recordLateEnds records the end of each compaction of a main's that ended
// after main was told it was still running: main's wrapper still waits for
// it. Called under c.mu, after an event, following the connection-to-owner
// lock order.
func (c *connection) recordLateEnds() {
	for _, marker := range c.admitted.lateEnds {
		c.owner.CompactionEnded(marker.request, marker.by, marker.outcome())
	}
	c.admitted.lateEnds = nil
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
// to the terminal's, leaving main's letter to fail. A second compaction right
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
