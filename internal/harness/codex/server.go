package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

type serverSession struct {
	observerSubscriptions map[observerSubscription]observerLease
	gitWrite              bool
	subscribedClient      *rpcClient
	subscribedThread      string
	subscriptionWake      chan struct{}
	statusSequence        uint64
	observationSequence   uint64
	observation           *turnObservation
	observations          []*turnObservation
	closeOnce             sync.Once
	generation            uint64
	hintSequence          uint64
	dirty                 bool
	discoveryErr          error
	discoverGate          chan struct{}
	discoverWake          chan struct{}
	ignored               map[string]bool
	delayed               []serverNotice
	scope                 context.Context
	path                  string
	args                  []string
	env                   []string
	cwd                   string
	mu                    sync.Mutex
	client                *rpcClient
	current               string
	changed               chan struct{}
	messages              map[string]string
	outcomes              []harness.Completion
	wake                  chan struct{}
	process               *exec.Cmd
	exited                chan struct{}
	stopped               chan struct{}
	cancel                context.CancelFunc
	emit                  func(harness.Completion) error
	note                  func(string)
}

func newServer(path string, args, env []string, cwd string) *serverSession {
	return &serverSession{subscriptionWake: make(chan struct{}, 1), path: path, args: args, env: env, cwd: cwd, changed: make(chan struct{}), discoverGate: make(chan struct{}, 1), discoverWake: make(chan struct{}, 1), ignored: make(map[string]bool), messages: make(map[string]string), wake: make(chan struct{}, 1), exited: make(chan struct{}), stopped: make(chan struct{})}
}

func (s *serverSession) Start(ctx context.Context, emit func(harness.Completion) error, note func(string)) error {
	s.emit, s.note = emit, note
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.scope = runCtx
	versionCtx, stopVersion := context.WithTimeout(ctx, 2*time.Second)
	version := exec.CommandContext(versionCtx, "codex", "--version")
	version.Env = s.env
	raw, err := version.Output()
	stopVersion()
	if err != nil || string(raw) != "codex-cli 0.154.0\n" {
		s.note("server transport was verified with codex-cli 0.154.0; installed version differs or could not be read")
	}
	log, err := os.OpenFile(s.path+".log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		cancel()
		return err
	}
	command := exec.Command("codex", s.args...)
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
	client, err := s.connect(connectCtx)
	stopConnect()
	if err != nil {
		s.stopProcess()
		cancel()
		return fmt.Errorf("app-server startup failed (log %s): %w", s.path+".log", err)
	}
	s.mu.Lock()
	s.client = client
	s.signal()
	s.mu.Unlock()
	go s.report(runCtx)
	go s.discover(runCtx)
	go s.subscribe(runCtx)
	go s.maintain(runCtx, client)
	return nil
}

func (s *serverSession) connect(ctx context.Context) (*rpcClient, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		client, err := connectRPC(ctx, s.path, s.event)
		if err == nil {
			return client, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *serverSession) maintain(ctx context.Context, client *rpcClient) {
	defer close(s.stopped)
	for {
		select {
		case <-ctx.Done():
			client.close()
			return
		case <-client.done:
		}
		if ctx.Err() != nil {
			client.close()
			return
		}
		s.mu.Lock()
		s.client = nil
		s.generation++
		s.ignored = make(map[string]bool)
		s.signal()
		s.mu.Unlock()
		client.close()
		s.note("app-server connection lost; reconnecting without treating it as a turn result")
		for {
			if ctx.Err() != nil {
				return
			}
			reconnectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			next, err := s.connect(reconnectCtx)
			if err == nil {
				err = s.restore(reconnectCtx, next)
			}
			cancel()
			if err == nil {
				client = next
				break
			}
			if next != nil {
				next.close()
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(250 * time.Millisecond):
			}
		}
		s.mu.Lock()
		s.client = client
		s.signal()
		s.mu.Unlock()
		s.note("app-server connection restored")
		s.wakeDiscovery()
	}
}

func (s *serverSession) signal()               { close(s.changed); s.changed = make(chan struct{}) }
func (s *serverSession) Done() <-chan struct{} { return s.exited }
func (s *serverSession) Thread() (string, error) {
	scope := s.scope
	if scope == nil {
		scope = context.Background()
	}
	ctx, cancel := context.WithTimeout(scope, 2*time.Second)
	defer cancel()
	if err := s.ensureThread(ctx); err != nil {
		return "", fmt.Errorf("%w: %v", inbox.ErrThreadUnavailable, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil || s.current == "" || s.dirty || s.discoveryErr != nil {
		return "", fmt.Errorf("%w: thread identity changed during discovery", inbox.ErrThreadUnavailable)
	}
	return s.current, nil
}

func (s *serverSession) stopProcess() {
	if s.process == nil {
		return
	}
	_ = syscall.Kill(-s.process.Process.Pid, syscall.SIGTERM)
	select {
	case <-s.exited:
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(-s.process.Process.Pid, syscall.SIGKILL)
		<-s.exited
	}
}

func (s *serverSession) Close() {
	s.closeOnce.Do(func() {
		if s.cancel == nil {
			return
		}
		s.cancel()
		s.stopProcess()
		select {
		case <-s.stopped:
		case <-time.After(time.Second):
		}
	})
}
