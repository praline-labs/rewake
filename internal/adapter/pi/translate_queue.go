package pi

import "github.com/praline-labs/rewake/internal/core/mirror"

// The prompts pi holds while it works (docs/face.md, "The look"): one
// steered into a working run waits in pi's queue until pi takes it after the
// run's current tool batch, its user message beginning then, or until pi
// gives it back — Esc puts the queue back into pi's editor, alt+up takes it
// there without stopping the run — or an input handler took it whole or
// changed its text, which no message can then be told to be. The face shows
// it waiting, as pi's own screen does under its chat, until the take settles
// it where pi took it.

// queueFollowed bounds the prompts followed: past it the oldest is no longer
// followed and, left queued, ends a gap with the run.
const queueFollowed = 32

type queued struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Missed bool   `json:"missed,omitempty"`
}

// add follows a prompt pi was given to queue.
func (t *translator) add(id, text string) {
	t.Queue = append(t.Queue, queued{ID: id, Text: text})
	if len(t.Queue) > queueFollowed {
		t.Queue = t.Queue[1:]
	}
}

// took is a user message pi began while prompts wait: pi took the one of
// its text, the first of them where two are the same. A message of no
// waiting text is none of them: an input handler may have taken one prompt
// whole and changed the next, so the oldest is no evidence; what waits is
// given back once pi's queue is found empty, or as the run ends.
func (t *translator) took(text string) {
	for i, w := range t.Queue {
		if w.Text == text {
			t.Queue = append(t.Queue[:i:i], t.Queue[i+1:]...)
			t.push(mirror.Event{Kind: mirror.EventTaken, ID: w.ID})
			return
		}
	}
}

// check holds what waits against whether pi's queue holds anything: one
// found missing twice running was given back or taken whole by an input
// handler. Twice, since a prompt is followed from pi's input event, a moment
// before pi puts it in its queue.
func (t *translator) check(pending bool) {
	if pending {
		for i := range t.Queue {
			t.Queue[i].Missed = false
		}
		return
	}
	var kept []queued
	for _, w := range t.Queue {
		if w.Missed {
			t.push(mirror.Event{Kind: mirror.EventTaken, ID: w.ID, Status: mirror.ItemStatusRefused})
			continue
		}
		w.Missed = true
		kept = append(kept, w)
	}
	t.Queue = kept
}

// giveBack gives back what is left as the run ends, before its end: pi
// takes its whole queue within a run, and Esc puts it back in its editor.
func (t *translator) giveBack() {
	for _, w := range t.Queue {
		t.push(mirror.Event{Kind: mirror.EventTaken, ID: w.ID, Status: mirror.ItemStatusRefused})
	}
	t.Queue = nil
}
