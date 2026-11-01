package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/worktype"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A worker's wrapper replaces its own image with the same build on SIGUSR2
// and goes on as the same run: its pid, its epoch, its harness child and the
// files its service hands over stay, and the new image adopts them instead of
// launching. The probe of step 1 of "Updating without losing the sessions",
// on a branch only; what it hands over, what it rebuilds and what it refuses
// is .scratch/reexec-probe.md.

// HandoverEnv carries the handover to the new image; the CLI takes it before
// any argument (cli.Run) and removes it, so no child inherits it.
const HandoverEnv = "REWAKE_HANDOVER"

// handoverVersion is the version of Handover; an image adopts only its own.
const handoverVersion = 1

// reexecMark is the file of the run's directory where the outcome of the last
// request to re-execute is written, for whoever sent the signal.
const reexecMark = "reexec"

// Handover is the run as an image of its wrapper hands it to the next: the
// launch it was asked for, its identity, its harness and its service.
type Handover struct {
	Version       int             `json:"version"`
	Harness       string          `json:"harness"`
	Dir           string          `json:"dir"`
	Name          string          `json:"name"`
	Epoch         string          `json:"epoch"`
	Args          []string        `json:"args"`
	Intro         bool            `json:"intro"`
	Command       string          `json:"command,omitempty"`
	Role          string          `json:"role"`
	RoleReason    string          `json:"roleReason,omitempty"`
	Template      string          `json:"template,omitempty"`
	WorktreeEntry bool            `json:"worktreeEntry,omitempty"`
	WorkType      worktype.Type   `json:"workType"`
	WorkTypeText  string          `json:"workTypeText,omitempty"`
	HarnessPID    int             `json:"harnessPid"`
	HarnessStart  uint64          `json:"harnessStart"`
	Service       json.RawMessage `json:"service"`
	// Files are the descriptors the service handed over, inherited under
	// these numbers, in its order.
	Files []int `json:"files"`
	// Count is how many execs this run has been through.
	Count int `json:"count"`

	files []*os.File
}

// ReadHandover reads the handover an earlier image left in the environment
// and takes its descriptors.
func ReadHandover(spec string) (*Handover, error) {
	var handover Handover
	if err := json.Unmarshal([]byte(spec), &handover); err != nil {
		return nil, fmt.Errorf("the wrapper's handover cannot be read: %w", err)
	}
	if handover.Version != handoverVersion {
		return nil, fmt.Errorf("the wrapper's handover is version %d; this build takes %d", handover.Version, handoverVersion)
	}
	for _, fd := range handover.Files {
		handover.files = append(handover.files, os.NewFile(uintptr(fd), "handover"))
	}
	return &handover, nil
}

// Request is the launch the handover continues; the caller adds its own
// callbacks.
func (h *Handover) Request(harness adapter.Harness) Request {
	workType := h.WorkType
	workType.Text = h.WorkTypeText
	return Request{
		Harness: harness, Dir: h.Dir, Name: h.Name, Args: h.Args, Intro: h.Intro, Command: h.Command,
		Role: role.Of(h.Role), RoleReason: h.RoleReason, Template: h.Template,
		WorktreeEntry: h.WorktreeEntry, WorkType: workType, Handover: h,
	}
}

// claimOrAdopt claims the run's name, or takes the record an earlier image of
// this process claimed: same pid, same start, same epoch.
func claimOrAdopt(request Request, self int, selfStart uint64, boot, cwd string) (registry.Session, error) {
	h := request.Handover
	if h == nil {
		return claimName(request, self, selfStart, boot, cwd)
	}
	session, err := registry.Load(request.Dir, h.Name)
	if err != nil {
		return registry.Session{}, err
	}
	if session.ServicePID != self || session.ServiceStart != selfStart || session.Epoch() != h.Epoch {
		return registry.Session{}, errors.New("the record of the run handed over is not this process's any more")
	}
	return session, nil
}

// adoptHarness takes the harness an earlier image started, which is this
// process's child still.
func adoptHarness(request Request) (*os.Process, func(int), error) {
	h := request.Handover
	if start, err := proc.StartTime(h.HarnessPID); err != nil || start != h.HarnessStart {
		return nil, nil, errors.New("the harness handed over no longer runs")
	}
	process, err := os.FindProcess(h.HarnessPID)
	if request.Shell {
		return process, continueHarness, err
	}
	return process, followJob, err
}

// startService starts the plan's service, or adopts what the handover holds.
func startService(ctx context.Context, request Request, service adapter.Service, note func(string)) error {
	h := request.Handover
	if h == nil {
		return service.Start(ctx, note)
	}
	adopter, ok := service.(adapter.Handover)
	if !ok {
		return errors.New("the run was handed over to an adapter that cannot adopt it")
	}
	return adopter.Adopt(ctx, note, adapter.HandedOver{State: h.Service, Files: h.files})
}

// reexec is what a run needs to hand itself over.
type reexec struct {
	request Request
	session registry.Session
	runDir  string
	service adapter.Service
	harness *os.Process
	start   uint64
	// handoff asks the delivery server to stop; resume serves again after
	// an exec that failed.
	handoff chan chan<- string
	resume  chan struct{}
}

// watch re-executes on each SIGUSR2 until ctx ends.
func (r *reexec) watch(ctx context.Context) func() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGUSR2)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-signals:
				if why := r.try(ctx); why != "" {
					r.mark("refused: " + why)
					_, _ = fmt.Fprintln(os.Stderr, "rewake: not re-executed: "+why)
				}
			}
		}
	}()
	return func() { signal.Stop(signals) }
}

// mark writes the outcome of a request where its sender looks.
func (r *reexec) mark(outcome string) {
	_ = state.WriteFile(filepath.Join(r.runDir, reexecMark), []byte(outcome+"\n"), 0o600)
}

// try hands the run over and execs; it returns only when it did not.
func (r *reexec) try(ctx context.Context) string {
	handing, ok := r.service.(adapter.Handover)
	switch {
	case !ok:
		return "this harness's adapter cannot hand its run over"
	case r.session.Role == role.Main.ID:
		return "main's wrapper is not part of the probe"
	case r.request.Mirror:
		return "the run serves a mirror, which is not handed over"
	case r.request.OnClaimed != nil:
		return "the run made a checkout, which its launch settles at its end"
	}
	if why := handing.Busy(); why != "" {
		return why
	}
	reply := make(chan string, 1)
	select {
	case r.handoff <- reply:
	case <-ctx.Done():
		return "the run is ending"
	case <-time.After(5 * time.Second):
		return "the delivery server did not take the request"
	}
	if why := <-reply; why != "" {
		return why
	}
	// The server has stopped: no notice goes out until an image serves again.
	why := r.exec(handing)
	r.resume <- struct{}{}
	return why
}

// exec replaces the image; it returns only on a failure, and changed nothing.
func (r *reexec) exec(handing adapter.Handover) string {
	handed, err := handing.HandOver()
	if err != nil {
		return err.Error()
	}
	defer func() {
		for _, file := range handed.Files {
			_ = file.Close()
		}
	}()
	h := Handover{
		Version: handoverVersion, Harness: r.request.Harness.ID(), Dir: r.request.Dir, Name: r.session.Name, Epoch: r.session.Epoch(),
		Args: r.request.Args, Intro: r.request.Intro, Command: r.request.Command, Role: r.session.Role, RoleReason: r.session.RoleReason,
		Template: r.request.Template, WorktreeEntry: r.request.WorktreeEntry, WorkType: r.request.WorkType, WorkTypeText: r.request.WorkType.Text,
		HarnessPID: r.harness.Pid, HarnessStart: r.start, Service: handed.State,
	}
	if r.request.Handover != nil {
		h.Count = r.request.Handover.Count
	}
	h.Count++
	for _, file := range handed.Files {
		fd := file.Fd()
		// The copies carry close-on-exec, as every descriptor Go opens.
		if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_SETFD, 0); errno != 0 {
			return "a descriptor could not be kept across the exec: " + errno.Error()
		}
		h.Files = append(h.Files, int(fd))
	}
	raw, err := json.Marshal(h)
	if err != nil {
		return err.Error()
	}
	env := slices.DeleteFunc(os.Environ(), func(entry string) bool { return strings.HasPrefix(entry, HandoverEnv+"=") })
	env = append(env, HandoverEnv+"="+string(raw))
	executable, err := os.Executable()
	if err != nil {
		return err.Error()
	}
	return "the exec failed: " + syscall.Exec(executable, os.Args, env).Error()
}

// adopted tells the sender that the new image runs the run.
func adopted(request Request, runDir string) {
	if request.Handover != nil {
		(&reexec{runDir: runDir}).mark(fmt.Sprintf("adopted %d", request.Handover.Count))
	}
}
