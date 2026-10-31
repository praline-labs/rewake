package pi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mirror"
)

// Mirrors offers rewake's own face: the extension sends the conversation of
// a run whose hello answer says mirror, and hands pi the face's prompts
// (docs/face.md, docs/research-pi.md).
func (harness) Mirrors() {}

// FacesWorkers draws every pi worker with rewake's face, as a Claude Code
// session looks, without --dev (the owner's word, October 31, 2026).
func (harness) FacesWorkers() {}

// mirrorEvent is one thing the session showed, on the wire in the face's
// neutral words (docs/face.md, "The model"): the extension maps pi's events
// and tools to them, so the host only hands them on.
type mirrorEvent struct {
	Kind   string        `json:"kind"`
	ID     string        `json:"id,omitempty"`
	Turn   string        `json:"turn,omitempty"`
	Block  string        `json:"block,omitempty"`
	Text   string        `json:"text,omitempty"`
	Origin string        `json:"origin,omitempty"`
	Name   string        `json:"name,omitempty"`
	Status string        `json:"status,omitempty"`
	State  string        `json:"state,omitempty"`
	Part   int           `json:"part,omitempty"`
	Of     int           `json:"of,omitempty"`
	Lost   int           `json:"lost,omitempty"`
	Permit *mirrorPermit `json:"permit,omitempty"`
}

type mirrorPermit struct {
	By      string  `json:"by"`
	Allowed bool    `json:"allowed"`
	Reason  *string `json:"reason,omitempty"`
}

// mirrorKinds are the events pi's extension sends; pi has no permission mode
// to say, so a mode event is not among them.
var mirrorKinds = map[string]mirror.EventKind{
	"prompt": mirror.EventPrompt, "delta": mirror.EventDelta, "settle": mirror.EventSettle,
	"tool": mirror.EventTool, "input": mirror.EventInput, "result": mirror.EventResult,
	"turn": mirror.EventTurn, "notice": mirror.EventNotice, "lost": mirror.EventLost, "resync": mirror.EventResync,
}

// mirrored hands a batch of the conversation to the model the face draws.
// An event of a kind the model does not take is left out, never guessed at.
func (r *run) mirrored(request frame) frame {
	if r.mirror == nil {
		return frame{Error: "rewake's face does not draw this run"}
	}
	events := make([]mirror.Event, 0, len(request.Events))
	for _, e := range request.Events {
		kind, ok := mirrorKinds[e.Kind]
		if !ok {
			continue
		}
		out := mirror.Event{
			Kind: kind, ID: e.ID, Turn: e.Turn, Block: mirror.ItemKind(e.Block), Text: e.Text, Origin: e.Origin,
			Name: e.Name, Status: mirror.ItemStatus(e.Status), State: turnStates[e.State], Part: e.Part, Of: e.Of, Lost: e.Lost,
		}
		if kind == mirror.EventTurn && out.State == "" {
			continue
		}
		if e.Permit != nil {
			out.Permit = &mirror.Permit{By: mirror.PermitBy(e.Permit.By), Allowed: e.Permit.Allowed, Reason: e.Permit.Reason}
		}
		events = append(events, out)
	}
	r.mirror.Apply(events...)
	return frame{OK: true}
}

var turnStates = map[string]mirror.TurnState{"working": mirror.TurnStateWorking, "idle": mirror.TurnStateIdle}

// input carries an input of rewake's face to pi: a prompt typed there as
// the person's, which the extension mirrors once pi hears it as its input,
// or the person's Esc, which stops the run as pi's own Esc does.
func (r *run) input(ctx context.Context, input mirror.Input) error {
	l, thread := r.connection()
	if l == nil {
		return errors.New("pi's extension is not connected")
	}
	request := frame{Op: "prompt", Thread: thread, Text: input.Text}
	if input.Kind == mirror.InputKindInterrupt {
		request = frame{Op: "interrupt", Thread: thread}
	}
	answer, err := l.ask(ctx, request)
	if err != nil {
		return fmt.Errorf("pi's extension did not answer: %w", err)
	}
	if !answer.OK {
		if answer.Error == "" {
			return fmt.Errorf("pi's extension refused the %s", request.Op)
		}
		return errors.New(answer.Error)
	}
	return nil
}

// mirrorLetter shows a letter rewake delivered as the face draws one: the
// notification's summary after a dot colored by its status, in the form
// the face reads a letter from (docs/face.md, "The look"). pi hears the
// letter as a message of rewake's, which reaches no input handler.
func (r *run) mirrorLetter(message mail.Message) {
	if r.mirror == nil {
		return
	}
	status := "completed"
	switch mail.NoticeKind(message) {
	case mail.Stopped:
		status = "killed"
	case mail.Error:
		status = "failed"
	case mail.Interim:
		status = "running"
	}
	escape := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	text := "<task-notification>\n<status>" + status + "</status>\n<summary>" + escape.Replace(mail.Notice(message)) + "</summary>\n</task-notification>"
	r.mirror.Apply(mirror.Event{Kind: mirror.EventPrompt, ID: "letter-" + message.ID, Origin: "rewake", Text: text})
}
