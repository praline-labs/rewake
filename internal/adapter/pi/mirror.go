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

// mirrored hands a batch of pi's own events to the model the face draws,
// translated here (translate.go).
func (r *run) mirrored(request frame) frame {
	if r.mirror == nil {
		return frame{Error: "rewake's face does not draw this run"}
	}
	r.mirror.Apply(r.translate.take(request.Events)...)
	return frame{OK: true}
}

// mirrorLevel tells the face the thinking level a sample read, which the
// strip shows and its Alt+T moves; the model says only a change,
// and pi's level names are the model's.
func (r *run) mirrorLevel(level string) {
	if r.mirror != nil && level != "" {
		r.mirror.Apply(mirror.Decode([]mirror.WireEvent{{Kind: mirror.EventLevel, Level: mirror.ThinkingLevel(level)}}, nil)...)
	}
}

func (r *run) mirrorWorkMode(mode string) {
	if r.mirror != nil {
		r.mirror.Apply(mirror.Event{Kind: mirror.EventWorkMode, WorkMode: mirror.WorkMode(mode)})
	}
}

// input carries an input of rewake's face to pi: a prompt typed there as
// the person's, which the face shows once pi hears it as its input;
// the person's Esc; Shift+Tab for runtime mode; or Alt+T for thinking,
// through the levels read here off the last sample's model.
func (r *run) input(ctx context.Context, input mirror.Input) error {
	l, thread := r.connection()
	if l == nil {
		return errors.New("pi's extension is not connected")
	}
	request := frame{Op: "prompt", Thread: thread, Text: input.Text}
	switch input.Kind {
	case mirror.InputKindPrompt:
		// Heard back as pi's input from an extension, it is the face's.
		r.translate.sent(input.Text)
	case mirror.InputKindInterrupt:
		request = frame{Op: "interrupt", Thread: thread}
	case mirror.InputKindWorkmode:
		request = frame{Op: "mode-key", Thread: thread}
	case mirror.InputKindThinking:
		r.mu.Lock()
		model := r.observed.model
		r.mu.Unlock()
		if model == nil || !model.Reasoning {
			return errors.New("the current model does not support thinking")
		}
		request = frame{Op: "thinking", Thread: thread, Levels: levelsOf(model)}
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
