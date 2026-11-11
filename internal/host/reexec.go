package host

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mirror"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/workmode"
	"github.com/praline-labs/rewake/internal/host/delivery"
	"github.com/praline-labs/rewake/internal/host/switching"
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

// HandoverVersion is the version of Handover; an image adopts only its own,
// and a switch of the room admits only a build that takes the same.
const HandoverVersion = 7

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
	Mode          workmode.Mode   `json:"mode"`
	ModeText      string          `json:"modeText,omitempty"`
	Modes         workmode.Config `json:"modes"`
	HarnessPID    int             `json:"harnessPid"`
	HarnessStart  uint64          `json:"harnessStart"`
	Service       json.RawMessage `json:"service"`
	// Files are the descriptors the service handed over, inherited under
	// these numbers, in its order.
	Files []int `json:"files"`
	// Count is how many execs this run has been through.
	Count int `json:"count"`
	// Park is set on an image a switch parked (host/switching): it holds no
	// lease until its leader says go or resume.
	Park *switching.Park `json:"park,omitempty"`
	// Mirror says the run serves rewake's face; Model is the descriptor of
	// the memory file its model is handed over in (startMirror), 0 for none.
	Mirror bool `json:"mirror,omitempty"`
	Model  int  `json:"model,omitempty"`
	// Signals are those a parked image caught and did not pass on, which
	// the run of the next image forwards first (takeSignals).
	Signals []int `json:"signals,omitempty"`
	// Shell says the run is main's in rewake's shell, which its launch read
	// from an environment it then cleared (Request.Shell).
	Shell bool `json:"shell,omitempty"`
	// Notices are main's notice state (notices_handover.go).
	Notices *Notices `json:"notices,omitempty"`
	// Delivery is the delivery server's reminder state (delivery.Memory).
	Delivery *delivery.Memory `json:"delivery,omitempty"`

	files []*os.File
	model *os.File
}

// ReadHandover reads the handover an earlier image left in the environment
// and takes its descriptors.
func ReadHandover(spec string) (*Handover, error) {
	var handover Handover
	if err := json.Unmarshal([]byte(spec), &handover); err != nil {
		return nil, fmt.Errorf("the wrapper's handover cannot be read: %w", err)
	}
	if handover.Version != HandoverVersion {
		return nil, fmt.Errorf("the wrapper's handover is version %d; this build takes %d", handover.Version, HandoverVersion)
	}
	for _, fd := range handover.Files {
		handover.files = append(handover.files, os.NewFile(uintptr(fd), "handover"))
	}
	if handover.Model > 0 {
		handover.model = os.NewFile(uintptr(handover.Model), "model")
	}
	return &handover, nil
}

// Request is the launch the handover continues; the caller adds its own
// callbacks.
func (h *Handover) Request(harness adapter.Harness) Request {
	mode := h.Mode
	mode.Text = h.ModeText
	return Request{
		Harness: harness, Dir: h.Dir, Name: h.Name, Args: h.Args, Intro: h.Intro, Command: h.Command,
		Role: role.Of(h.Role), RoleReason: h.RoleReason, Template: h.Template,
		WorktreeEntry: h.WorktreeEntry, Mode: mode, Modes: h.Modes, Mirror: h.Mirror, Shell: h.Shell, Handover: h,
	}
}

// Continue execs executable with the handover, unparked: a parked image's
// go, to the staged copy of the switch's target. It returns only on failure.
func (h *Handover) Continue(executable string) error {
	next := *h
	next.Park = nil
	// What the parked image caught goes with the run: the handler that
	// holds it does not outlive the exec.
	next.Signals = append(slices.Clone(h.Signals), caughtSignals()...)
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	env := slices.DeleteFunc(os.Environ(), func(entry string) bool { return strings.HasPrefix(entry, HandoverEnv+"=") })
	return syscall.Exec(executable, os.Args, append(env, HandoverEnv+"="+string(raw)))
}

// reexec is what a run needs to hand itself over.
type reexec struct {
	request Request
	session registry.Session
	runDir  string
	service adapter.Service
	harness *os.Process
	start   uint64
	// model is the face's model, nil for a run the face does not draw.
	model *mirror.Model
	// notices hands main's notice state over (notices_handover.go).
	notices *noticeHand
	// count is how many execs the run has been through before this image.
	count int
	// handoff asks the delivery server to stop; resume serves again after
	// an exec that failed.
	handoff chan chan<- delivery.Stop
	resume  chan struct{}
	ctx     context.Context
}

// watch re-executes on each SIGUSR2, and answers every switch of the room
// (host/switching), until ctx ends.
func (r *reexec) watch(ctx context.Context) func() {
	r.ctx = ctx
	stopSwitching := switching.Watch(ctx, r.request.Dir, r)
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
	return func() { signal.Stop(signals); stopSwitching() }
}

// mark writes the outcome of a request where its sender looks.
func (r *reexec) mark(outcome string) {
	_ = state.WriteFile(filepath.Join(r.runDir, reexecMark), []byte(outcome+"\n"), 0o600)
}

// refusal is why this run can never be handed over, or "".
func (r *reexec) refusal() string {
	// A checkout the launch made goes over too: the adopted image finds it
	// by its record and settles it at the run's end (cli.adoptedCheckout).
	if _, ok := r.service.(adapter.Handover); !ok {
		return "this harness's adapter cannot hand its run over"
	}
	return ""
}

// Who names the run to a switch's leader.
func (r *reexec) Who() string { return "session " + r.session.Name }

// Answer is whether the run can be handed over now.
func (r *reexec) Answer() (string, string) {
	if why := r.refusal(); why != "" {
		return switching.Cannot, why
	}
	if why := r.service.(adapter.Handover).Busy(); why != "" {
		return switching.Busy, why
	}
	return switching.Ready, ""
}

// Drop hands the run over to its own image, parked for the switch.
func (r *reexec) Drop(park switching.Park) string {
	if kind, why := r.Answer(); kind != switching.Ready {
		return why
	}
	return r.handOff(r.ctx, &park)
}

// try hands the run over and execs; it returns only when it did not.
func (r *reexec) try(ctx context.Context) string {
	if kind, why := r.Answer(); kind != switching.Ready {
		return why
	}
	return r.handOff(ctx, nil)
}

// handOff stops the delivery server and execs, parked when park is set; it
// returns only when it did not, and the server serves again.
func (r *reexec) handOff(ctx context.Context, park *switching.Park) string {
	handing := r.service.(adapter.Handover)
	reply := make(chan delivery.Stop, 1)
	select {
	case r.handoff <- reply:
	case <-ctx.Done():
		return "the run is ending"
	case <-time.After(5 * time.Second):
		return "the delivery server did not take the request"
	}
	stop := <-reply
	if stop.Why != "" {
		return stop.Why
	}
	// The server has stopped: no notice goes out until an image serves again.
	why := r.exec(handing, park, stop.Memory)
	r.resume <- struct{}{}
	return why
}

// exec replaces the image with its own; it returns only on a failure, and
// changed nothing.
func (r *reexec) exec(handing adapter.Handover, park *switching.Park, memory delivery.Memory) string {
	handed, err := handing.HandOver()
	if err != nil {
		return err.Error()
	}
	// Reached only when the exec did not happen.
	defer func() {
		for _, file := range handed.Files {
			_ = file.Close()
		}
		if handed.Resume != nil {
			handed.Resume()
		}
	}()
	notices, why := r.takeNotices()
	if why != "" {
		return why
	}
	defer r.giveNotices()
	h := Handover{
		Version: HandoverVersion, Harness: r.request.Harness.ID(), Dir: r.request.Dir, Name: r.session.Name, Epoch: r.session.Epoch(),
		Args: r.request.Args, Intro: r.request.Intro, Command: r.request.Command, Role: r.session.Role, RoleReason: r.session.RoleReason,
		Template: r.request.Template, WorktreeEntry: r.request.WorktreeEntry, Mode: r.request.Mode, ModeText: r.request.Mode.Text, Modes: r.request.Modes,
		HarnessPID: r.harness.Pid, HarnessStart: r.start, Service: handed.State, Park: park, Mirror: r.request.Mirror,
		Shell: r.request.Shell, Notices: notices, Delivery: &memory,
	}
	// The model after the service's handover, which stopped what feeds it.
	if r.model != nil {
		model, err := handModel(r.model)
		if err != nil {
			return "the face's model could not be handed over: " + err.Error()
		}
		defer func() { _ = model.Close() }()
		if why := inherited(model); why != "" {
			return why
		}
		h.Model = int(model.Fd())
	}
	h.Count = r.count + 1
	for _, file := range handed.Files {
		if why := inherited(file); why != "" {
			return why
		}
		h.Files = append(h.Files, int(file.Fd()))
	}
	raw, err := json.Marshal(h)
	if err != nil {
		return err.Error()
	}
	env := slices.DeleteFunc(os.Environ(), func(entry string) bool { return strings.HasPrefix(entry, HandoverEnv+"=") })
	env = append(env, HandoverEnv+"="+string(raw))
	// The image this process runs, which the install's path may no longer
	// hold: an upgrade replaces that file under running sessions.
	return "the exec failed: " + syscall.Exec("/proc/self/exe", os.Args, env).Error()
}

// inherited clears a descriptor's close-on-exec, which every one Go opens
// carries, so the next image has it.
func inherited(file *os.File) string {
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, file.Fd(), syscall.F_SETFD, 0); errno != 0 {
		return "a descriptor could not be kept across the exec: " + errno.Error()
	}
	return ""
}
