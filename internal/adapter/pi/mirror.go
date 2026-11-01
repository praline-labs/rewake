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

// mirrored hands a batch of the conversation to the model the face draws,
// in the feed's neutral words the extension mapped pi's events to.
func (r *run) mirrored(request frame) frame {
	if r.mirror == nil {
		return frame{Error: "rewake's face does not draw this run"}
	}
	// pi has no permission mode to say.
	r.mirror.Apply(mirror.Decode(request.Events, nil)...)
	return frame{OK: true}
}

// input carries an input of rewake's face to pi: a prompt typed there as
// the person's, which the extension mirrors once pi hears it as its input;
// the person's Esc, which stops the run as pi's own Esc does; or their
// shift+tab, which moves the thinking level on as pi's own does.
func (r *run) input(ctx context.Context, input mirror.Input) error {
	l, thread := r.connection()
	if l == nil {
		return errors.New("pi's extension is not connected")
	}
	request := frame{Op: "prompt", Thread: thread, Text: input.Text}
	switch input.Kind {
	case mirror.InputKindInterrupt:
		request = frame{Op: "interrupt", Thread: thread}
	case mirror.InputKindThinking:
		request = frame{Op: "thinking", Thread: thread}
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
