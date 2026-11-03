package host

import (
	"fmt"
	"os"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// A wrapper's image that could not adopt the run handed over to it keeps the
// harness running instead of exiting under it. The harness is the wrapper's
// child on the wrapper's terminal, and a wrapper that exited took it down
// with the terminal's hangup: main's Claude Code, and with it the shell and
// the room, on November 3, 2026 (docs/traps.md). Held, the run is served no
// more — no notice wakes it, none of its turns is reported — which its record
// says, its live set withdrawn so no sender's task is admitted, and main is
// told; its record, its lease and its files stay until the harness ends, as
// a run's do, and go then.

// Held is the error of an adoption that failed and held its harness to its
// end: the code Run answers beside it is the harness's own.
type Held struct{ Err error }

func (h *Held) Error() string { return h.Err.Error() }
func (h *Held) Unwrap() error { return h.Err }

// fail holds the harness until it ends and answers its exit code: before
// the harness is the run's own, nothing of the run serves on, and what of it
// the run has so far says so.
func (a adopting) fail(r *run, err error) (int, error) {
	h := a.h
	// Nothing half-adopted serves on: the mod's or the extension's next
	// request finds nobody, as when its run's wrapper is gone.
	if r.closeService != nil {
		r.closeService()
	}
	// What the record says the run serves goes: admission reads it, and a
	// task admitted now would wait for a report no turn's end can send. Before
	// the claim the record is the earlier image's, under the same epoch.
	live := r.live
	if live == nil && r.lease != nil {
		live = newLiveSet(r.request.Dir, h.Name, h.Epoch)
		defer live.stop()
	}
	if live != nil {
		live.withdraw()
	}
	if r.runDir != "" {
		(&reexec{runDir: r.runDir}).mark("not adopted: " + err.Error())
	}
	why := "could not be adopted: " + err.Error()
	_, _ = fmt.Fprintln(os.Stderr, "rewake: the run handed over "+why+"; its harness runs on unserved: no letter wakes it and none of its turns is reported. End it and start it again.")
	if name := r.session.Name; name != "" && role.Of(r.session.Role).ID != role.Main.ID {
		_ = tellMain(r.request.Dir, name, r.epoch, "Rewake: the run of "+name+" "+why+".\n"+
			"Its harness runs on, but rewake serves it no more: no letter wakes it and none of its turns is reported. Ask the person to end it in its pane, or close its space, and open it again.")
	}
	code, held := h.hold(r.signals)
	if !held {
		return 0, err
	}
	return code, &Held{Err: err}
}

// Hold keeps the harness an earlier image of this wrapper handed over
// running, serving nothing, and answers its exit code once it ends; false
// when it no longer runs. Its signals are passed on and its stops answered
// as a run's are.
func (h *Handover) Hold() (int, bool) { return h.hold(nil) }

// hold is Hold forwarding on signals, the run's catch, or on a catch of its
// own when nil.
func (h *Handover) hold(signals chan os.Signal) (int, bool) {
	// What the service handed over goes unserved: a listener kept open
	// would take connections nobody answers.
	for _, file := range h.files {
		_ = file.Close()
	}
	if h.model != nil {
		_ = h.model.Close()
	}
	harness, onStop, err := h.adoptHarness()
	if err != nil {
		return 0, false
	}
	if signals == nil {
		var restore func()
		signals, restore = takeSignals(h.Signals)
		defer restore()
	}
	go forward(signals, harness, func() bool { return proc.Alive(h.HarnessPID, h.HarnessStart) })
	return waitForHarness(h.HarnessPID, onStop), true
}

// tellMain leaves the room's main a note from the session.
func tellMain(dir, name, epoch, text string) error {
	sessions, err := registry.List(dir)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if role.Of(session.Role).ID == role.Main.ID && session.Name != name {
			return mail.Put(dir, mail.Message{
				ID: mail.NewID(), From: name, FromEpoch: epoch, To: session.Name, ToEpoch: session.Epoch(),
				Kind: mail.Note, Text: text, CreatedAt: time.Now(),
			})
		}
	}
	return nil
}
