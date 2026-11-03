package modhost

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/session"
)

type command struct {
	request control.Request
	// text is a prompt's or a trace's, which no request of the control
	// directory carries; turn is the one an interrupt stops.
	text       string
	turn       string
	load       string
	generation int
	sent       bool
	// settled is set by the mod's final reply; the request stays to answer
	// a repeated one.
	settled bool
	answer  chan control.Answer
}

type management struct {
	cancel context.CancelFunc
	wake   chan struct{}
	// queued wait for the mod's next poll, oldest first: a trace the host
	// sends of itself may meet a request or a prompt of the face.
	queued   []*command
	requests map[string]*command
	order    []string
	observed observations
	// turn is the main loop's running turn, from turn/start and the newest
	// state, which names it or none; interrupts are who stopped which turn
	// (control.Interrupts), both handed to the next image.
	turn       string
	interrupts control.Interrupts
}

// awaiting says a request the mod took waits for its final reply, which only
// this image would hear. One settled is kept only to answer a repeated reply,
// and one of an earlier load or generation would be refused its reply; neither
// keeps the run from a handover, or a run main ever compacted or interrupted
// would stay busy until 64 more requests pushed it out.
func (m *management) awaiting(current *loadState) bool {
	if current == nil || !current.live {
		return false
	}
	for _, pending := range m.requests {
		if pending.sent && !pending.settled && pending.load == current.hello.LoadID && pending.generation == current.generation {
			return true
		}
	}
	return false
}

// StartControl keeps the file protocol with the run, counted until it stops.
func (s *Server) StartControl(parent context.Context) {
	if s.cfg.ControlDir == "" || !s.admit() {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	s.management.cancel = cancel
	if s.closed {
		cancel()
	}
	s.mu.Unlock()
	go func() {
		defer s.dismiss()
		control.Serve(ctx, s.cfg.ControlDir, 50*time.Millisecond, s.carry, s.recordOutcome)
	}()
}

func (s *Server) carry(ctx context.Context, request control.Request) control.Answer {
	answer := s.dispatch(ctx, request)
	if answer.Outcome != control.Requested && answer.Outcome != control.Started && !answer.Open {
		s.recordOutcome(request, answer)
	}
	return answer
}

func (s *Server) dispatch(ctx context.Context, request control.Request) control.Answer {
	return s.dispatchText(ctx, request, "")
}

// actionTrace is a control command of the host's own: text the mod appends
// to the conversation's history as an interruption's actor.
const actionTrace = "trace"

// dispatchText queues a command for the mod's next heartbeat; a prompt
// carries its text, and comes only from rewake's face (mirror.go), and a
// trace its own. An interrupt names the turn the host knows running, which
// it attributes to the asker before the command goes out, and waits for that
// turn's end on the mail path (turns.go) up to control.SettleWait.
func (s *Server) dispatchText(ctx context.Context, request control.Request, text string) control.Answer {
	sent := time.Now()
	s.mu.Lock()
	current := s.current
	if s.closed || current == nil || !current.live {
		s.mu.Unlock()
		return control.Answer{Outcome: control.Refused, Reason: control.NotAnswering}
	}
	if s.frozen {
		s.mu.Unlock()
		return control.Answer{Outcome: control.Refused, Reason: control.Busy, Detail: errHanding.Error()}
	}
	if request.Action == control.Reply {
		s.mu.Unlock()
		return s.answerAsk(ctx, request)
	}
	if request.Action != control.Compact && request.Action != control.Interrupt && (request.Action != actionPrompt && request.Action != actionTrace || text == "") {
		s.mu.Unlock()
		return control.Answer{Outcome: control.Refused, Detail: "unknown control action"}
	}
	thread := s.management.observed.thread
	if thread == "" {
		thread = current.hello.SessionID
	}
	if request.Conversation != "" && request.Conversation != thread {
		s.mu.Unlock()
		return control.Answer{Outcome: control.Refused, Reason: control.NotSelected}
	}
	request.Conversation = thread
	m := &s.management
	pending := &command{request: request, text: text, load: current.hello.LoadID, generation: current.generation, answer: make(chan control.Answer, 1)}
	if request.Action == control.Interrupt {
		if m.turn == "" {
			s.mu.Unlock()
			return control.Answer{Outcome: control.Refused, Reason: control.NoTurn}
		}
		pending.turn = m.turn
		m.interrupts.Record(request, m.turn, sent)
	}
	m.requests[request.ID] = pending
	m.order = append(m.order, request.ID)
	if len(m.order) > 64 {
		delete(m.requests, m.order[0])
		m.order = m.order[1:]
	}
	m.queued = append(m.queued, pending)
	select {
	case m.wake <- struct{}{}:
	default:
	}
	s.mu.Unlock()
	answer := s.acknowledged(ctx, pending)
	if request.Action != control.Interrupt {
		return answer
	}
	if answer.Outcome != control.Requested {
		m.interrupts.Forget(request.ID)
		return answer
	}
	// Its turn's end, or the wait's lapse, is the outcome; the attribution
	// goes with the run to the next image for an end heard later.
	answer = m.interrupts.Wait(ctx, request.ID, sent.Add(control.SettleWait), s.stop)
	s.mu.Lock()
	pending.settled = true
	s.mu.Unlock()
	return answer
}

// acknowledged is the mod's first reply to a dispatched command, or what
// its silence says.
func (s *Server) acknowledged(ctx context.Context, pending *command) control.Answer {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case answer := <-pending.answer:
		return answer
	case <-timer.C:
	case <-s.stop:
	case <-ctx.Done():
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := &s.management
	if at := slices.Index(m.queued, pending); at >= 0 {
		m.queued = slices.Delete(m.queued, at, at+1)
	}
	if !pending.sent {
		delete(m.requests, pending.request.ID)
		return control.Answer{Outcome: control.Refused, Reason: control.NotAnswering}
	}
	return control.Answer{Outcome: control.Failed, Reason: control.CutShort, Detail: "the mod did not acknowledge the request; its outcome is unknown", Open: true}
}

// traceLater has the mod put an interrupted turn's actor into the
// conversation's history once the end was captured, and settles the
// interrupt by whether it could.
func (s *Server) traceLater(conversation, actor string, settled func(traced bool)) {
	if !s.admit() {
		settled(false)
		return
	}
	go func() {
		defer s.dismiss()
		request := control.Request{ID: "trace-" + control.NewID(), Action: actionTrace, From: "rewake", Conversation: conversation}
		settled(s.dispatchText(context.Background(), request, actor).Outcome == control.Done)
	}()
}

// takeCommand checks the generation again after the held poll wakes, and
// gives the oldest command of that generation.
func (s *Server) takeCommand(poll AliveRequest) *ControlCommand {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.current == nil || !s.current.live || s.current.hello.LoadID != poll.LoadID || s.current.generation != poll.Generation {
		return nil
	}
	m := &s.management
	at := slices.IndexFunc(m.queued, func(pending *command) bool {
		return pending.load == poll.LoadID && pending.generation == poll.Generation
	})
	if at < 0 {
		return nil
	}
	pending := m.queued[at]
	m.queued = slices.Delete(m.queued, at, at+1)
	pending.sent = true
	request := pending.request
	command := &ControlCommand{ID: request.ID, Action: request.Action, From: request.From, Focus: request.Focus, Conversation: request.Conversation}
	if pending.text != "" {
		command.Text = &pending.text
	}
	if pending.turn != "" {
		command.Turn = &pending.turn
	}
	return command
}

func (s *Server) controlReply(w http.ResponseWriter, r *http.Request) {
	var result ControlReply
	if status, err := decode(w, r, &result, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	switch result.Outcome {
	case control.Requested, control.Done, control.Refused, control.Failed:
	default:
		refuse(w, http.StatusBadRequest, "unknown control outcome")
		return
	}
	if err := s.fromHarness(r.Context()); err != nil {
		s.refuseEffect(w, http.StatusForbidden, err)
		return
	}
	s.mu.Lock()
	if status, err := s.currentGeneration(result.LoadID, result.Generation); err != nil {
		s.mu.Unlock()
		s.refuseEffect(w, status, err)
		return
	}
	pending := s.management.requests[result.ID]
	if pending == nil || !pending.sent || pending.load != result.LoadID || pending.generation != result.Generation {
		s.mu.Unlock()
		refuse(w, http.StatusConflict, "no command of this generation has that id")
		return
	}
	answer := worded(pending.request.Action, result)
	if result.Outcome != control.Requested {
		pending.settled = true
		s.outcomeLocked(pending.request, answer)
	}
	select {
	case pending.answer <- answer:
	default:
	}
	s.mu.Unlock()
	s.answer(w, r, Done{})
}

func (s *Server) recordOutcome(request control.Request, answer control.Answer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomeLocked(request, answer)
}

func (s *Server) outcomeLocked(request control.Request, answer control.Answer) {
	if request.Action != control.Compact {
		return
	}
	s.management.observed.view.Outcome(session.CompactionOutcome{
		Request: request.ID, RequestedBy: request.From, Outcome: answer.Outcome, Reason: answer.Reason, Detail: answer.Detail,
		TokensBefore: answer.TokensBefore, TokensAfter: answer.TokensAfter, EndedAt: time.Now(),
	})
}

// worded is what a reply of the mod says of action: its outcome, a refusal
// by the code of the check that made it, and a native result read here.
func worded(action string, result ControlReply) control.Answer {
	switch result.Outcome {
	case control.Requested:
		return control.Answer{Outcome: control.Requested}
	case control.Refused:
		code := ""
		if result.Code != nil {
			code = string(*result.Code)
		}
		return control.Refusal(action, code)
	case control.Failed:
		answer := control.Answer{Outcome: control.Failed, Detail: failures[action]}
		if action == control.Compact {
			answer.Reason = control.CutShort
		}
		return answer
	}
	if action != control.Compact {
		return control.Answer{Outcome: control.Done}
	}
	// $.session.compact resolves with skip where the engine declined, which
	// the mod passes on as it resolved.
	var compacted struct {
		Skip         *string  `json:"skip"`
		TokensBefore *float64 `json:"tokensBefore"`
		TokensAfter  *float64 `json:"tokensAfter"`
	}
	_ = json.Unmarshal(result.Native, &compacted)
	if compacted.Skip != nil {
		skip := *compacted.Skip
		if len(skip) > 300 {
			skip = skip[:300]
		}
		return control.Answer{Outcome: control.Refused, Detail: skip}
	}
	return control.Answer{Outcome: control.Done, TokensBefore: tokenCount(compacted.TokensBefore), TokensAfter: tokenCount(compacted.TokensAfter)}
}

// failures word a native call that threw, by the command's action.
var failures = map[string]string{
	control.Compact:   "native compaction failed or was interrupted",
	control.Interrupt: "native interrupt could not be requested",
	actionPrompt:      "the engine did not take the prompt",
	actionTrace:       control.TraceLost,
}

func tokenCount(value *float64) *int64 {
	if value == nil || *value < 0 {
		return nil
	}
	count := int64(*value)
	return &count
}
