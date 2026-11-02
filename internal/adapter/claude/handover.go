package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
)

// The run across an exec of its wrapper (adapter.Handover,
// docs/claude.md#handing-the-run-over). The mod's endpoint hands over its
// listener and its order of loads, so the loaded mod's next request is
// answered as before the exec; everything else that lives only in this
// image's memory keeps the exec from going until it is gone, which an idle
// session makes it: a control request or an ask the mod waits on, an end the
// core has not taken (modhost), a notice waiting for its receipts or for a
// late word (the lane). The reply socket is bound again by the next image,
// which is the same process the session checks a receipt's listener against.
// The mod's files stay as the first image wrote them: the loaded mod is that
// build's, and the switch admitted only a build of the same protocol.

// handoverVersion is the version of handedState; an image adopts only its
// own, and a switch of the room admits only a build that takes the same
// (Contracts).
const handoverVersion = 1

type handedState struct {
	Version int            `json:"version"`
	Server  modhost.Handed `json:"server"`
}

// Busy answers what keeps the run from being handed over now.
func (r *modRun) Busy() string {
	if why := r.server.Busy(); why != "" {
		return why
	}
	return r.busy()
}

// HandOver answers the endpoint's order of loads and a copy of its listener.
func (r *modRun) HandOver() (adapter.HandedOver, error) {
	if why := r.busy(); why != "" {
		return adapter.HandedOver{}, errors.New(why)
	}
	handed, listener, err := r.server.HandOver()
	if err != nil {
		return adapter.HandedOver{}, err
	}
	raw, err := json.Marshal(handedState{Version: handoverVersion, Server: handed})
	if err != nil {
		_ = listener.Close()
		r.server.Thaw()
		return adapter.HandedOver{}, err
	}
	return adapter.HandedOver{State: raw, Files: []*os.File{listener}, Resume: r.server.Thaw}, nil
}

// Adopt runs the service on what an earlier image of the run handed over, in
// place of Start: the endpoint on its listener, the lane listening again with
// its gate open, since the session drew its interface long ago.
func (r *modRun) Adopt(ctx context.Context, note func(string), handed adapter.HandedOver) error {
	var got handedState
	if err := json.Unmarshal(handed.State, &got); err != nil {
		closeAll(handed.Files)
		return fmt.Errorf("the Claude Code run's handover cannot be read: %w", err)
	}
	if got.Version != handoverVersion || len(handed.Files) != 1 {
		closeAll(handed.Files)
		return fmt.Errorf("the Claude Code run was handed over as version %d with %d files; this build takes version %d with 1", got.Version, len(handed.Files), handoverVersion)
	}
	// The lane's set first: the endpoint reports a live generation proven
	// as it adopts it, a transition the plain set must not follow.
	r.reporting.Lock()
	if r.live != nil {
		r.report(false)
	}
	r.reporting.Unlock()
	if err := r.server.Adopt(got.Server, handed.Files[0]); err != nil {
		return err
	}
	r.server.StartControl(ctx)
	r.limit = 0
	return r.lane.Start(ctx, note)
}

// busy answers what of the lane lives only in this image: a word not yet
// passed to the delivery server, a notice waiting for its receipts, or one
// counted delivered whose late word may still take it back.
func (l *lane) busy() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Deliver alone prunes; an idle lane would keep what aged out.
	l.pruneSilent(time.Now())
	switch {
	case len(l.queue) > 0:
		return "a word about a notice is not yet passed to the delivery server"
	case len(l.sent) > 0:
		return "a notice waits for its receipts or a late word, which only this image would hear"
	}
	return ""
}

func closeAll(files []*os.File) {
	for _, file := range files {
		_ = file.Close()
	}
}

// WorkersOnly: main's run is not handed over yet (adapter.WorkersOnly).
func (r *modRun) WorkersOnly() {}
