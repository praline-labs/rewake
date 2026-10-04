package codex

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/control"
	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/harness/codex/gateway"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/sessionstate"
)

// lastObservedServerVersion is the native CLI the app-server transport was seen
// working against: two probes on 0.155.1, the ordinary path and steer, not a full
// re-verification. A mismatch only produces a note: an unobserved version is a reason
// to warn, not to refuse a launch the owner asked for. The test fixture repeats this
// string as its own literal on purpose, so a typo here fails a test instead of
// matching itself.
const lastObservedServerVersion = "codex-cli 0.155.1"

type serverSession struct {
	gatewayLog  *os.File
	capture     func() *inbox.ReadBoundary
	startupFork bool
	// intent is the conversation the launch asked to resume, which
	// deliveries wait for (gateway.LaunchIntent).
	intent                     gateway.LaunchIntent
	processStop                sync.Once
	reportVersion, reportSaved uint64
	reportOverflow             sync.Once
	path, upstream, epoch, cwd string
	args, env                  []string
	gitWrite                   bool
	// mailbox is the room's state directory, where the session's mail and
	// its grant journal are, and stateRoot the root above every room.
	mailbox, name, stateRoot string
	// grants is what this run granted and took back, guarded by mu: the
	// record revocation goes by (grantJournal).
	grants []grant.Entry
	// followed are the conversations whose grants from before a resume were
	// restored or taken back, guarded by mu (server_dirgrant_resume.go).
	followed map[string]bool
	// answers are what the mains said, by conversation, when asked to
	// confirm again the grants a resumed one had; guarded by mu
	// (server_dirgrant_resume.go).
	answers map[string]*resumeAnswers
	// legacyLandlock says why this session's commands may run in rewake's
	// own namespaces; set, it takes no grant.
	legacyLandlock  string
	program         string
	controlDir      string
	gateway         *gateway.Gateway
	proxy           *http.Server
	process         *exec.Cmd
	exited, stopped chan struct{}
	cancel          context.CancelFunc
	closeOnce       sync.Once
	mu              sync.Mutex
	outcomes        []harness.Completion
	wake            chan struct{}
	reportCancel    context.CancelFunc
	emit            func(context.Context, harness.Completion) error
	note            func(string)
	// tool is the mail tool's injection this run checks, nil without one
	// (server_mailtool.go); readsOff, once a check finds a read would not
	// reach the conversation whole, says why for the rest of the run.
	tool     *toolInjection
	readsOff atomic.Pointer[string]
}

func newServer(path string, args, env []string, cwd string) *serverSession {
	args = append([]string(nil), args...)
	for i, arg := range args {
		if arg == "unix://"+path {
			args[i] = arg + ".up"
		}
	}
	epoch := ""
	for _, value := range env {
		if strings.HasPrefix(value, "REWAKE_EPOCH=") {
			epoch = strings.TrimPrefix(value, "REWAKE_EPOCH=")
		}
	}
	return &serverSession{path: path, upstream: path + ".up", epoch: epoch, args: args, env: env, cwd: cwd, exited: make(chan struct{}), stopped: make(chan struct{}), wake: make(chan struct{}, 1)}
}

// executable is the program the server runs: the launch's --command, or codex.
func (s *serverSession) executable() string {
	if s.program != "" {
		return s.program
	}
	return "codex"
}

func (s *serverSession) Start(ctx context.Context, handler harness.CompletionHandler, note func(string)) error {
	s.emit, s.note = handler.Publish, note
	s.capture = handler.Capture
	if err := s.restoreOutcomes(); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	versionCtx, stopVersion := context.WithTimeout(ctx, 2*time.Second)
	version := exec.CommandContext(versionCtx, s.executable(), "--version")
	version.Env = s.env
	// A wrapper that does not end in exec leaves the harness as its child,
	// holding stdout after the wrapper is killed; without a delay Output
	// would wait for that grandchild for ever.
	version.WaitDelay = time.Second
	raw, err := version.Output()
	stopVersion()
	if err != nil || string(raw) != lastObservedServerVersion+"\n" {
		s.note("server transport was last observed working with " + lastObservedServerVersion + "; installed version differs or could not be read")
	}
	log, err := os.OpenFile(s.upstream+".log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		cancel()
		return err
	}
	command := exec.Command(s.executable(), s.args...)
	command.Env = s.env
	command.Dir = s.cwd
	command.Stderr = log
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if err := command.Start(); err != nil {
		_ = log.Close()
		cancel()
		return err
	}
	s.process = command
	go func() { _ = command.Wait(); _ = log.Close(); close(s.exited); cancel() }()
	connectCtx, stopConnect := context.WithTimeout(runCtx, 8*time.Second)
	err = s.probe(connectCtx)
	stopConnect()
	if err != nil {
		s.stopProcess()
		cancel()
		return fmt.Errorf("app-server startup failed (log %s): %w", s.upstream+".log", err)
	}
	if err := s.checkTool(runCtx); err != nil {
		s.stopProcess()
		cancel()
		return err
	}
	listener, err := net.Listen("unix", s.path)
	if err != nil {
		s.stopProcess()
		cancel()
		return err
	}
	_ = os.Chmod(s.path, 0o600)
	s.gatewayLog, err = os.OpenFile(s.path+".gateway.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		_ = listener.Close()
		s.stopProcess()
		cancel()
		return err
	}
	var readSequence func() uint64
	if s.capture != nil {
		readSequence = func() uint64 { return s.capture().Through }
	}
	var endCapture func() (uint64, int64)
	if handler.EndCapture != nil && s.capture != nil {
		endCapture = func() (uint64, int64) {
			boundary, noted := handler.EndCapture()
			if boundary == nil {
				return s.capture().Through, noted
			}
			return boundary.Through, noted
		}
	}
	admit, err := s.holdMail()
	if err != nil {
		_ = listener.Close()
		s.stopProcess()
		cancel()
		return err
	}
	s.gateway = gateway.New(gateway.Config{Upstream: s.upstream, Epoch: s.epoch, StartupFork: s.startupFork, Intent: s.intent, Name: s.name, Admit: admit, ReadSequence: readSequence, EndCapture: endCapture, ToolEvent: handler.ToolEvent, ThreadCheck: s.threadCheck(runCtx, handler.Channel), Closed: func(info gateway.CloseInfo) {
		_, _ = fmt.Fprintf(s.gatewayLog, "connection=%d generation=%d direction=%s reason=%s error=%q bytes=%d requests=%d responses=%d sizeStage=%s messageBytes=%d limitBytes=%d\n", info.Connection, info.Generation, info.Direction, info.Reason, info.Error, info.Bytes, info.Requests, info.Responses, info.SizeStage, info.MessageBytes, info.LimitBytes)
	}, Complete: func(result gateway.Completion) {
		value := harness.Completion{ID: result.PublicationID(), Thread: result.Thread, Kind: inbox.Kind(result.Kind), Text: result.Text, Started: result.Started, Ended: result.Ended}
		if s.capture != nil && result.ReadThrough != nil {
			value.Boundary = s.capture()
			value.Boundary.Through = *result.ReadThrough
		}
		s.queueCompletion(value)
	}})
	s.proxy = &http.Server{Handler: s.gateway, ReadHeaderTimeout: 3 * time.Second, MaxHeaderBytes: 8192}
	reportCtx, reportCancel := context.WithCancel(context.Background())
	s.reportCancel = reportCancel
	go s.report(reportCtx)
	if s.controlDir != "" {
		go control.Serve(runCtx, s.controlDir, controlPoll, s.steer, s.withdrawn)
	}
	go func() { _ = s.proxy.Serve(listener); cancel() }()
	go func() { <-runCtx.Done(); s.stopProcess() }()
	return nil
}

// holdMail closes the run's mail before the terminal starts, when the launch
// asked to resume a conversation: until it is resumed or the person accepts
// another, rewake inbox reads nothing (sessionstate.MailHeld). It returns what
// opens the mail again, which the gateway calls before the hold ends.
func (s *serverSession) holdMail() (func() error, error) {
	if !s.intent.Resume || s.mailbox == "" {
		return nil, nil
	}
	intended := s.intent.Thread
	if intended == "" {
		intended = "a conversation"
	}
	detail := "the launch asked to resume " + intended + ", and it has not been resumed, nor has the person accepted the conversation selected instead"
	if err := sessionstate.HoldMail(s.mailbox, s.name, s.epoch, detail); err != nil {
		return nil, fmt.Errorf("could not record that the session's mail waits for its conversation: %w", err)
	}
	return func() error { return sessionstate.AdmitMail(s.mailbox, s.name, s.epoch) }, nil
}

// Startup probing initializes and closes a temporary client; it never chooses or resumes a root.
func (s *serverSession) probe(ctx context.Context) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		client, err := connectRPC(ctx, s.upstream, nil)
		if err == nil {
			client.close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *serverSession) Done() <-chan struct{} { return s.exited }

// ProcessID is the app-server's pid, zero before it started: the mail tool's
// server runs below it, not below the terminal.
func (s *serverSession) ProcessID() int {
	if s.process == nil || s.process.Process == nil {
		return 0
	}
	return s.process.Process.Pid
}

func (s *serverSession) Thread() (string, error) {
	if s.gateway != nil {
		binding := s.gateway.Binding()
		if binding.Ready {
			return binding.Thread, nil
		}
	}
	return "", inbox.ErrThreadUnavailable
}

// stopProcess ends the native process group: SIGTERM, then SIGKILL two seconds
// later. Those two seconds are one stage of an ordinary shutdown, and the
// workflow suite's termination budget (test/workflow) is the sum of those
// stages, so a change here has to be reflected there.
func (s *serverSession) stopProcess() {
	if s.process == nil {
		return
	}
	s.processStop.Do(func() {
		select {
		case <-s.exited:
			return
		default:
		}
		_ = syscall.Kill(-s.process.Process.Pid, syscall.SIGTERM)
		select {
		case <-s.exited:
		case <-time.After(2 * time.Second):
			_ = syscall.Kill(-s.process.Process.Pid, syscall.SIGKILL)
			<-s.exited
		}
	})
}

func (s *serverSession) Close() {
	s.closeOnce.Do(func() {
		if s.cancel == nil {
			return
		}
		if s.proxy != nil {
			_ = s.proxy.Close()
		}
		if s.gateway != nil {
			s.gateway.Close()
		}
		if s.gatewayLog != nil {
			_ = s.gatewayLog.Close()
		}
		s.cancel()
		s.stopProcess()
		if s.reportCancel != nil {
			s.reportCancel()
			<-s.stopped
		}
		_ = os.Remove(s.path)
		_ = os.Remove(s.upstream)
	})
}

func (s *serverSession) SessionState() sessionstate.Snapshot {
	if s.gateway == nil {
		return sessionstate.Unknown(s.epoch)
	}
	return s.gateway.SessionState()
}
