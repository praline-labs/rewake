package modhost

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/mirror"
)

// actionPrompt is a control command of rewake's face only: a prompt the mod
// submits as typed by the person. No request of the control directory
// carries it, since none carries a text.
const actionPrompt = "prompt"

// mirrorEvents hands a batch of the conversation to the model the face
// draws. The batch is the current load's: one of a lapsed generation, or of
// a run not drawn by the face, is refused, and the mod drops it.
func (s *Server) mirrorEvents(w http.ResponseWriter, r *http.Request) {
	var batch MirrorBatch
	if status, err := decode(w, r, &batch, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	if s.cfg.Mirror == nil {
		refuse(w, http.StatusNotFound, "rewake's face does not draw this run")
		return
	}
	if err := s.fromHarness(r.Context()); err != nil {
		s.refuseEffect(w, http.StatusForbidden, err)
		return
	}
	s.mu.Lock()
	status, err := s.currentGeneration(batch.LoadID, batch.Generation)
	s.mu.Unlock()
	if err != nil {
		s.refuseEffect(w, status, err)
		return
	}
	events := make([]mirror.Event, 0, len(batch.Events))
	for _, e := range batch.Events {
		events = append(events, mirrorEvent(e))
	}
	s.cfg.Mirror.Apply(events...)
	s.answer(w, r, Done{})
}

func mirrorEvent(e MirrorEvent) mirror.Event {
	text := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	number := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}
	out := mirror.Event{
		Kind: mirror.EventKind(e.Kind), ID: text(e.ID), Turn: text(e.Turn), Text: text(e.Text),
		Origin: text(e.Origin), Name: text(e.Name), Part: number(e.Part), Of: number(e.Of), Lost: number(e.Lost),
	}
	if e.Block != nil {
		out.Block = mirror.ItemKind(*e.Block)
	}
	if e.Status != nil {
		out.Status = mirror.ItemStatus(*e.Status)
	}
	if e.State != nil {
		out.State = mirror.TurnState(*e.State)
	}
	if e.Permit != nil {
		out.Permit = &mirror.Permit{By: mirror.PermitBy(e.Permit.By), Allowed: e.Permit.Allowed, Reason: e.Permit.Reason}
	}
	if e.Mode != nil {
		out.Mode = modes[*e.Mode]
	}
	return out
}

// modes names Claude Code's permission modes in the model's words; one the
// table does not know leaves the mode as it was.
var modes = map[string]mirror.PermissionMode{
	"default":           mirror.PermissionModeDefault,
	"plan":              mirror.PermissionModePlan,
	"acceptEdits":       mirror.PermissionModeEdits,
	"auto":              mirror.PermissionModeAuto,
	"bypassPermissions": mirror.PermissionModeBypass,
	"dontAsk":           mirror.PermissionModeUnasked,
}

// Prompt carries a prompt typed in rewake's face to the harness as the
// person's, through the next heartbeat as every command goes. Its item comes
// from the mod once the engine has taken the prompt in or dropped it, since
// only the mod learns which.
func (s *Server) Prompt(ctx context.Context, input mirror.Input) error {
	raw := make([]byte, 8)
	_, _ = rand.Read(raw)
	id := "face-" + hex.EncodeToString(raw)
	answer := s.dispatchText(ctx, control.Request{ID: id, Action: actionPrompt, From: "face"}, input.Text)
	if answer.Outcome != control.Done {
		why := answer.Reason
		if why == "" {
			why = answer.Detail
		}
		if why == "" {
			why = answer.Outcome
		}
		return errors.New(why)
	}
	// The mod mirrors the prompt itself once the engine takes it in or drops it.
	return nil
}
