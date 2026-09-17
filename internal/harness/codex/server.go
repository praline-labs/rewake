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
	closeOnce  sync.Once
	generation uint64
	path       string
	args       []string
	env        []string
	cwd        string
	mu         sync.Mutex
	client     *rpcClient
	current    string
	changed    chan struct{}
	messages   map[string]string
	outcomes   []harness.Completion
	wake       chan struct{}
	process    *exec.Cmd
	exited     chan struct{}
	stopped    chan struct{}
	cancel     context.CancelFunc
	emit       func(harness.Completion) error
	note       func(string)
}

func newServer(path string, args, env []string, cwd string) *serverSession {
	return &serverSession{path: path, args: args, env: env, cwd: cwd, changed: make(chan struct{}), messages: make(map[string]string), wake: make(chan struct{}, 1), exited: make(chan struct{}), stopped: make(chan struct{})}
}

func (s *serverSession) Start(ctx context.Context, emit func(harness.Completion) error, note func(string)) error {
	s.emit, s.note = emit, note
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
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
	}
}

func (s *serverSession) signal()               { close(s.changed); s.changed = make(chan struct{}) }
func (s *serverSession) Done() <-chan struct{} { return s.exited }
func (s *serverSession) Thread() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == "" || s.client == nil {
		return "", fmt.Errorf("%w: app-server has no ready TUI thread; wait for the TUI or connection to recover", inbox.ErrThreadUnavailable)
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
