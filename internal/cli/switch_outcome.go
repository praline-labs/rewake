package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// switchOutcome is what a deferred switch came to: the view, and the
// refusal or loss the command fails with.
type switchOutcome struct {
	View switchView `json:"view"`
	Fail string     `json:"fail,omitempty"`
	// Gone says main's run ended first: no note, its successor asked nothing.
	Gone bool `json:"gone,omitempty"`
}

// line is the outcome as the command and main's note say it.
func (o switchOutcome) line() string {
	switch {
	case o.Fail != "":
		return o.Fail
	case o.View.From == o.View.To:
		return "nothing to switch; the room runs the installed rewake, build " + build.Short(o.View.To) + "."
	}
	return "the room switched from build " + build.Short(o.View.From) + " to " + build.Short(o.View.To) + "; adopted: " + listOrNone(o.View.Adopted) + "."
}

// outcomeSpec is the leader's last image's work.
type outcomeSpec struct {
	Lead    leadSpec      `json:"lead"`
	Outcome switchOutcome `json:"outcome"`
}

// deliver hands the outcome to the room's build, which may no longer be
// this image's: a leader's lease is revoked at the commit, and only the new
// build's process writes in the room after it. It returns only when the exec
// failed, the outcome then said on the pipe alone.
func deliver(spec leadSpec, out switchOutcome, pipe *os.File) int {
	raw, err := json.Marshal(outcomeSpec{Lead: spec, Outcome: out})
	if err == nil {
		env := append(withoutEnv(os.Environ(), switchLeadEnv, switchOutcomeEnv), switchOutcomeEnv+"="+string(raw))
		path := build.Pinned(spec.Room)
		if _, statErr := os.Stat(path); statErr != nil {
			path = "/proc/self/exe"
		}
		// The request and the pipe are the descriptors the leader inherited,
		// which Go never marked close-on-exec.
		err = syscall.Exec(path, []string{path}, env)
	}
	out.Fail = strings.TrimSpace(out.Fail + "; the outcome could not be delivered to main: " + err.Error())
	answer(pipe, out)
	return ExitFailed
}

// deliverOutcome is the leader's last image: under a lease of the room's
// build as a writer, the note to main's run the switch was asked for, the
// answer on the pipe, and the request let go.
func deliverOutcome(raw string) int {
	var spec outcomeSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return ExitFailed
	}
	request := os.NewFile(requestFD, "switch-request")
	var pipe *os.File
	if spec.Lead.Pipe {
		pipe = os.NewFile(pipeFD, "switch-outcome")
	}
	out := spec.Outcome
	lease, err := build.Acquire(spec.Lead.Base, filepath.Base(spec.Lead.Room))
	if err != nil {
		out.Fail = strings.TrimSpace(out.Fail + "; the outcome could not be delivered to main: " + err.Error())
		answer(pipe, out)
		return ExitFailed
	}
	defer lease.Release()
	if !out.Gone && spec.Lead.Main != "" {
		if err := putSwitchNote(spec.Lead, out); err != nil {
			out.Fail = strings.TrimSpace(out.Fail + "; the note to main could not be put: " + err.Error())
		}
	}
	answer(pipe, out)
	_ = state.EndSwitchRequest(spec.Lead.Room, request)
	return ExitOK
}

// answer writes the outcome to a command waiting on the pipe; one that went
// leaves a broken pipe, which is nobody's loss.
func answer(pipe *os.File, out switchOutcome) {
	if pipe == nil {
		return
	}
	data, _ := json.Marshal(out)
	_, _ = pipe.Write(append(data, '\n'))
	_ = pipe.Close()
}

// putSwitchNote puts the outcome in main's mailbox, only while main's run is
// the one the switch was asked for: looked up again under its mailbox lock.
func putSwitchNote(spec leadSpec, out switchOutcome) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return state.WithMailboxLock(ctx, spec.Room, spec.Main, func() error {
		target, err := registry.LookupReadOnly(spec.Room, spec.Main)
		if err != nil || target.Epoch() != spec.MainEpoch || target.Role != role.Main.ID {
			return nil
		}
		return mail.PutOnce(spec.Room, switchNote(spec, out))
	})
}

// switchNote is the note of one deferred switch: its id sorts by its time,
// as every message's does, and names the request it answers.
func switchNote(spec leadSpec, out switchOutcome) mail.Message {
	sum := sha256.Sum256([]byte("switch\x00" + spec.Main + "\x00" + spec.MainEpoch + "\x00" + fmt.Sprint(spec.Asked)))
	now := time.Now()
	from, fromEpoch := spec.Asker, spec.AskerEpoch
	if from == "" {
		from = "shell"
	}
	text := "rewake restart rewake: " + out.line()
	return mail.Message{ID: fmt.Sprintf("%019d-%x", now.UnixNano(), sum[:12]), From: from, FromEpoch: fromEpoch, To: spec.Main, ToEpoch: spec.MainEpoch, Kind: mail.Note, CreatedAt: now, Text: text}
}
