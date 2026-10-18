package modhost

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	asking "github.com/praline-labs/rewake/internal/core/ask"
	"github.com/praline-labs/rewake/internal/core/control"
)

// A worker's permission requests and questions reach main through the run's
// broker (docs/permission-requests.md): the mod opens a request and polls it,
// main's rewake answer comes through the control directory, and the mod
// settles the request with whichever answer came first. Nothing of it is
// written down; a request lost to a restart is asked again.

// askHold is how long a poll of a request is held: within the protocol's
// longest wait, so the mod's fetch is never cut.
const askHold = (MaxWaitSeconds - 5) * time.Second

// answerWait bounds how long main's answer waits for the mod to settle the
// request it was given: the mod's poll wakes on the answer, so a settling
// that has not come by then is not coming.
const answerWait = 15 * time.Second

// askedLoad says whether load is the run's current load, which alone opens,
// polls and settles requests.
func (s *Server) askedLoad(load string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.closed:
		return http.StatusServiceUnavailable, errClosing
	case s.current == nil || s.current.hello.LoadID != load:
		return http.StatusConflict, errNoHello
	}
	return 0, nil
}

func (s *Server) askOpen(w http.ResponseWriter, r *http.Request) {
	var asked AskOpenRequest
	if status, err := decode(w, r, &asked, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	if status, err := s.askedLoad(asked.LoadID); err != nil {
		refuse(w, status, err.Error())
		return
	}
	if s.cfg.Asked == nil {
		refuse(w, http.StatusConflict, "this run is no worker main answers for")
		return
	}
	offer := offerOf(asked)
	id, deadline := s.asks.Open(asked.LoadID, offer)
	if err := s.cfg.Asked(id, offer, deadline); err != nil {
		s.asks.Drop(id)
		refuse(w, http.StatusConflict, "main could not be told: "+err.Error())
		return
	}
	s.answer(w, r, AskOpenAnswer{ID: id, LifetimeMs: int(time.Until(deadline).Milliseconds())})
}

func offerOf(asked AskOpenRequest) asking.Offer {
	offer := asking.Offer{Kind: string(asked.Kind), Tool: text(asked.Tool), Input: text(asked.Input), Cwd: text(asked.Cwd)}
	for _, question := range asked.Questions {
		prompt := asking.Prompt{Text: question.Question, Header: text(question.Header), Multi: question.MultiSelect != nil && *question.MultiSelect}
		for _, option := range question.Options {
			prompt.Options = append(prompt.Options, asking.Option{Label: option.Label, Description: text(option.Description)})
		}
		offer.Questions = append(offer.Questions, prompt)
	}
	return offer
}

func text(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Server) askPoll(w http.ResponseWriter, r *http.Request) {
	var asked AskPollRequest
	if status, err := decode(w, r, &asked, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	if status, err := s.askedLoad(asked.LoadID); err != nil {
		refuse(w, status, err.Error())
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		select {
		case <-s.stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	poll, err := s.asks.Poll(ctx, asked.LoadID, asked.ID, askHold)
	switch {
	case errors.Is(err, context.Canceled):
		refuse(w, http.StatusServiceUnavailable, errClosing.Error())
		return
	case err != nil:
		refuse(w, http.StatusNotFound, err.Error())
		return
	}
	answer := AskPollAnswer{State: AskStatePending}
	switch {
	case poll.Settled:
		answer.State = AskStateSettled
	case poll.Expired:
		answer.State = AskStateExpired
	case poll.Answer != nil:
		answer.State = AskStateAnswered
		for question, text := range poll.Answer.Answers {
			answer.Answers = append(answer.Answers, AskAnswer{Question: question, Answer: text})
		}
		if poll.Answer.Reason != "" {
			reason := poll.Answer.Reason
			answer.Reason = &reason
		}
	}
	s.answer(w, r, answer)
}

func (s *Server) askSettle(w http.ResponseWriter, r *http.Request) {
	var asked AskSettleRequest
	if status, err := decode(w, r, &asked, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	switch asked.Winner {
	case AskWinnerMain, AskWinnerLocal, AskWinnerExpired, AskWinnerAborted:
	default:
		refuse(w, http.StatusBadRequest, "unknown winner")
		return
	}
	if status, err := s.askedLoad(asked.LoadID); err != nil {
		refuse(w, status, err.Error())
		return
	}
	if err := s.asks.Settle(asked.LoadID, asked.ID, string(asked.Winner)); err != nil {
		refuse(w, http.StatusNotFound, err.Error())
		return
	}
	s.answer(w, r, Done{})
}

// answerAsk carries main's answer to the request it names and waits, bounded,
// for the mod to settle it: Done once the mod took it, a refusal naming the
// answer that came first, or an open failure when no settling came.
func (s *Server) answerAsk(ctx context.Context, request control.Request) control.Answer {
	refused := func(reason, detail string) control.Answer {
		return control.Answer{Outcome: control.Refused, Reason: reason, Detail: detail}
	}
	offer, err := s.asks.Offer(request.Ask)
	if err != nil {
		return refused(control.NoRequest, err.Error())
	}
	answer, err := asking.Parse(offer, request.Choices)
	if err != nil {
		return refused(control.NotAChoice, err.Error())
	}
	outcome, err := s.asks.Propose(request.Ask, answer)
	switch {
	case errors.Is(err, asking.ErrAnswered):
		return refused(control.Busy, "another answer to this request is being given")
	case err != nil:
		return refused(control.NoRequest, err.Error())
	}
	timer := time.NewTimer(answerWait)
	defer timer.Stop()
	select {
	case got := <-outcome:
		switch {
		case got.Accepted:
			return control.Answer{Outcome: control.Done}
		case got.Winner == asking.Local:
			return refused(control.AnsweredFirst, "the person at the worker's screen answered before your answer reached it")
		case got.Winner == asking.Expired:
			return refused(control.Expired, "the request's deadline passed before your answer reached it")
		}
		return refused(control.CutShort, fmt.Sprintf("the worker's call ended before your answer reached it (%s)", got.Winner))
	case <-timer.C:
	case <-s.stop:
	case <-ctx.Done():
	}
	return control.Answer{Outcome: control.Failed, Detail: "the worker did not confirm it took the answer; it may still have", Open: true}
}
