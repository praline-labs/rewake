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
// more — no notice wakes it, none of its turns is reported — which main is
// told; its record, its lease and its files stay until the harness ends, as
// a run's do, and go then.

// unadopted is how Run fails before the harness is its own.
type unadopted struct {
	request Request
	// Known once the run has them: what main is told, and where the
	// sender of a re-execution looks for its outcome.
	name, epoch, part, runDir string
	// close closes the plan's service, once the plan exists.
	close func()
}

// fail returns err: at once for a launch, which started nothing, and for an
// adoption once the harness it holds has ended, with its exit code.
func (u *unadopted) fail(err error) (int, error) {
	h := u.request.Handover
	if h == nil {
		return 0, err
	}
	// Nothing half-adopted serves on: the mod's or the extension's next
	// request finds nobody, as when its run's wrapper is gone.
	if u.close != nil {
		u.close()
	}
	if u.runDir != "" {
		(&reexec{runDir: u.runDir}).mark("not adopted: " + err.Error())
	}
	why := "could not be adopted: " + err.Error()
	_, _ = fmt.Fprintln(os.Stderr, "rewake: the run handed over "+why+"; its harness runs on unserved: no letter wakes it and none of its turns is reported. End it and start it again.")
	if u.name != "" && role.Of(u.part).ID != role.Main.ID {
		_ = tellMain(u.request.Dir, u.name, u.epoch, "Rewake: the run of "+u.name+" "+why+".\n"+
			"Its harness runs on, but rewake serves it no more: no letter wakes it and none of its turns is reported. Ask the person to end it in its pane, or close its space, and open it again.")
	}
	code, _ := h.Hold()
	return code, err
}

// Hold keeps the harness an earlier image of this wrapper handed over
// running, serving nothing, and answers its exit code once it ends; false
// when it no longer runs. Its signals are passed on and its stops answered
// as a run's are.
func (h *Handover) Hold() (int, bool) {
	// What the service handed over goes unserved: a listener kept open
	// would take connections nobody answers.
	for _, file := range h.files {
		_ = file.Close()
	}
	if h.model != nil {
		_ = h.model.Close()
	}
	harness, onStop, err := adoptHarness(Request{Handover: h, Shell: h.Shell})
	if err != nil {
		return 0, false
	}
	signals, restore := takeSignals(h)
	defer restore()
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
