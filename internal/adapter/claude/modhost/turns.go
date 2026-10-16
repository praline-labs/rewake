package modhost

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/boottime"
)

// The run's turns as the mod reports them (docs/v2/stage4-turns.md#the-hosts-side):
// a start is passed on with no time, since the mod has none to give; an end is
// captured at its first arrival and that capture is the end from then on,
// however often the mod sends it again, until the core took it.

// eventPrefix names an end's event: the mod's end id under the adapter's id.
const eventPrefix = "claude/"

// capture is an end's first arrival: what the core is handed every time the
// end is sent, and the digest of what the request carried, against which a
// later request of the same end id is checked.
type capture struct {
	end    adapter.TurnEnd
	digest [sha256.Size]byte
}

// ReportTurns takes where the run's turns go; called before Start.
func (s *Server) ReportTurns(handler adapter.TurnHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turns = handler
}

// reasons maps the protocol's reasons to the adapter's.
var reasons = map[EndReason]adapter.EndReason{
	EndReasonAnswered: adapter.Answered,
	EndReasonAborted:  adapter.Aborted,
	EndReasonRefused:  adapter.Refused,
	EndReasonFailed:   adapter.Failed,
}

// errNoHello refuses a turn of a load the run never admitted.
var errNoHello = errors.New("no hello of this load was admitted in this run, so its turns are not taken")

// turnStart passes a turn's start on, with no time: the mod has none, and the
// arrival is not the turn's start.
func (s *Server) turnStart(w http.ResponseWriter, r *http.Request) {
	var start TurnStartRequest
	if status, err := decode(w, r, &start, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	if start.LoadID == "" || start.Conversation == "" || start.TurnID == "" {
		refuse(w, http.StatusBadRequest, "a turn's start names its load, its conversation and its turn")
		return
	}
	handler, status, err := s.effect(r.Context(), start.LoadID)
	if err != nil {
		s.refuseEffect(w, status, err)
		return
	}
	if handler.Started != nil {
		handler.Started(start.Conversation, start.TurnID, 0)
	}
	s.answer(w, r, Receipt{TurnID: start.TurnID})
}

// turnEnd hands an end to the core, captured once, and answers a receipt
// only when the core took it.
func (s *Server) turnEnd(w http.ResponseWriter, r *http.Request) {
	var end TurnEndRequest
	if status, err := decode(w, r, &end, MaxEndBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	reason, known := reasons[end.Reason]
	if end.LoadID == "" || end.Conversation == "" || end.TurnID == "" || end.EndID == "" || !known {
		refuse(w, http.StatusBadRequest, "a turn's end names its load, its conversation, its turn, its end and a reason the protocol knows")
		return
	}
	completion, status, err := s.captured(r.Context(), end, reason)
	if err != nil {
		s.refuseEffect(w, status, err)
		return
	}
	switch err := s.publish(r.Context(), completion); {
	case errors.Is(err, adapter.ErrRunGone):
		refuse(w, http.StatusGone, err.Error())
		return
	case err != nil:
		refuse(w, http.StatusServiceUnavailable, "the end was not taken yet; send it again: "+err.Error())
		return
	}
	endID := end.EndID
	s.answer(w, r, Receipt{TurnID: end.TurnID, EndID: &endID})
}

// publish runs the core's take of an end, counted among the run's workers
// as the handler it runs in.
func (s *Server) publish(ctx context.Context, end adapter.TurnEnd) error {
	s.mu.Lock()
	publish := s.turns.Publish
	s.mu.Unlock()
	if publish == nil {
		return errors.New("this run takes no turn ends")
	}
	return publish(ctx, end)
}

// captured is the end the request names: its first capture, made now if this
// is its first arrival. It is decided under the lock that Close takes, after
// the peer is checked again, for a capture made before as for a new one: a
// request that finds the server closing captures and publishes nothing, so no
// capture comes after the run's read clock closes, and one past this point
// is counted among the run's workers until its publication returns.
func (s *Server) captured(ctx context.Context, end TurnEndRequest, reason adapter.EndReason) (adapter.TurnEnd, int, error) {
	answer := ""
	if end.Answer != nil {
		answer = *end.Answer
	}
	// The load id is left out: it names the load that sends the end, and a
	// later load of the run sends what an earlier one stored.
	digest := sha256.Sum256([]byte(end.Conversation + "\x00" + end.TurnID + "\x00" + string(end.Reason) + "\x00" + answer))
	if s.deciding != nil {
		s.deciding()
	}
	if err := s.fromHarness(ctx); err != nil {
		return adapter.TurnEnd{}, http.StatusForbidden, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if status, err := s.open(end.LoadID); err != nil {
		return adapter.TurnEnd{}, status, err
	}
	if known, ok := s.ends[end.EndID]; ok {
		if known.digest != digest {
			return adapter.TurnEnd{}, http.StatusConflict, errors.New("an end of this id was sent before with other content; it is not taken again")
		}
		return known.end, 0, nil
	}
	completion := adapter.TurnEnd{
		Event: eventPrefix + end.EndID, Reason: reason, Answer: answer, Thread: end.Conversation,
	}
	if s.turns.EndCapture != nil {
		completion.Boundary, completion.Ended = s.turns.EndCapture()
	}
	if completion.Boundary == nil && s.turns.Capture != nil {
		completion.Boundary = s.turns.Capture()
	}
	if completion.Ended == 0 {
		completion.Ended = boottime.Now()
	}
	s.ends[end.EndID] = &capture{end: completion, digest: digest}
	return completion, 0, nil
}

// effect is the start's decision to take effect: the peer checked again and
// the server open, under the lock Close takes.
func (s *Server) effect(ctx context.Context, load string) (adapter.TurnHandler, int, error) {
	if s.deciding != nil {
		s.deciding()
	}
	if err := s.fromHarness(ctx); err != nil {
		return adapter.TurnHandler{}, http.StatusForbidden, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if status, err := s.open(load); err != nil {
		return adapter.TurnHandler{}, status, err
	}
	return s.turns, 0, nil
}

// open says the server takes an effect of the load: not closing, and the
// load admitted. Called under mu.
func (s *Server) open(load string) (int, error) {
	if s.closed {
		return http.StatusServiceUnavailable, errClosing
	}
	if !s.loads[load] {
		return http.StatusBadRequest, errNoHello
	}
	return 0, nil
}

// refuseEffect refuses a request whose effect was not taken. A peer that
// failed its check is no longer the harness that asked: nobody is there to
// take a refusal, so the connection is cut, as a hello's is.
func (s *Server) refuseEffect(w http.ResponseWriter, status int, err error) {
	if status == http.StatusForbidden {
		panic(http.ErrAbortHandler)
	}
	refuse(w, status, err.Error())
}

// ended answers which of the runs the mod names rewake proves ended, so it
// may remove what the harness's store keeps of them; it changes nothing here.
func (s *Server) ended(w http.ResponseWriter, r *http.Request) {
	var asked EndedRequest
	if status, err := decode(w, r, &asked, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	if !s.admittedLoad(asked.LoadID) {
		refuse(w, http.StatusBadRequest, errNoHello.Error())
		return
	}
	if len(asked.Epochs) > maxEndedEpochs {
		refuse(w, http.StatusBadRequest, "ask about at most 32 runs at once")
		return
	}
	answer := EndedAnswer{Ended: []string{}}
	for _, epoch := range asked.Epochs {
		if s.cfg.Ended != nil && s.cfg.Ended(epoch) {
			answer.Ended = append(answer.Ended, epoch)
		}
	}
	s.answer(w, r, answer)
}

// maxEndedEpochs is how many runs one mod/ended asks about, as the schema
// says.
const maxEndedEpochs = 32
