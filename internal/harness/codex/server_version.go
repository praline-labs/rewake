package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/proc"
)

// The version, confirmed at start
// (docs/mail-bridge-launch-codex.md#the-version-confirmed-at-start): the
// launch chose the tool with the version its one --version read gave; the
// owned app-server's initialize answer names its version too, and a version
// is proven only when both name it. The server's start runs nothing more to
// learn it.

// lastObservedServerVersion is the native CLI the app-server transport was seen
// working against: two probes on 0.155.1, the ordinary path and steer, not a full
// re-verification. A mismatch only produces a note: an unobserved version is a reason
// to warn, not to refuse a launch the owner asked for. The test fixture repeats this
// string as its own literal on purpose, so a typo here fails a test instead of
// matching itself.
const lastObservedServerVersion = "0.155.1"

// notConfirmed is why a withdrawn tool is out: the two reads disagreed and
// the choice made again without a version leaves it out.
const notConfirmed = "harness version not confirmed"

// transportNote is the compatibility note, from the launch's read.
func (s *serverSession) transportNote() {
	if s.version.Value != lastObservedServerVersion {
		s.note("server transport was last observed working with codex-cli " + lastObservedServerVersion + "; installed version differs or could not be read")
	}
}

// spawn starts the app-server with args. Its exit ends the run through
// cancel, unless it was retired first: a server stopped to be started again
// without the tool is not the run's end.
func (s *serverSession) spawn(args []string, cancel context.CancelFunc) (*atomic.Bool, error) {
	log, err := os.OpenFile(s.upstream+".log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	command := exec.Command(s.executable(), args...)
	command.Env = s.env
	command.Dir = s.cwd
	command.Stderr = log
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if err := command.Start(); err != nil {
		_ = log.Close()
		return nil, err
	}
	exited := make(chan struct{})
	retired := &atomic.Bool{}
	s.process, s.exited = command, exited
	go func() {
		_ = command.Wait()
		_ = log.Close()
		close(exited)
		if !retired.Load() {
			cancel()
		}
	}()
	return retired, nil
}

// retireGrace bounds each stage of a retirement: the group's own end after
// SIGTERM, then its end after SIGKILL. A variable so a test can shorten it.
var retireGrace = 2 * time.Second

// retire ends a server nothing has connected to yet and proves its whole
// process group ended: a --command wrapper may end before the app-server it
// started, so the leader's exit proves nothing of the rest. SIGTERM to the
// group, SIGKILL to what is left of it two seconds later, then a bounded wait
// until no member but a zombie remains and the leader is reaped. Past that
// bound the second start would run beside a server of the first, so it is
// refused; the group is not forgotten, its id is in the error.
func (s *serverSession) retire(retired *atomic.Bool) error {
	retired.Store(true)
	pid := s.process.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	if !groupEnds(pid, retireGrace) {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		if !groupEnds(pid, retireGrace) {
			return fmt.Errorf("the first app-server's process group %d did not end within %s of SIGKILL, so the server is not started again without the tool", pid, retireGrace)
		}
	}
	select {
	case <-s.exited:
		return nil
	case <-time.After(retireGrace):
		return fmt.Errorf("the first app-server %d was not reaped within %s of its group's end", pid, retireGrace)
	}
}

// groupAlive is proc.GroupAlive, a variable so a test can hold a group
// that does not end.
var groupAlive = proc.GroupAlive

// groupEnds waits, within bound, until no process of the group but a zombie
// is left.
func groupEnds(pgid int, bound time.Duration) bool {
	deadline := time.Now().Add(bound)
	for groupAlive(pgid) {
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return true
}

// startUpstream starts the owned app-server and probes it, then confirms
// the version for a run with the tool: kept, or withdrawn by starting the
// server again without our leaves, probed as a launch without the tool.
func (s *serverSession) startUpstream(ctx context.Context, cancel context.CancelFunc) error {
	retired, err := s.spawn(s.args, cancel)
	if err != nil {
		return err
	}
	agent, err := s.probeFor(ctx)
	if err != nil {
		return err
	}
	if s.tool == nil || s.confirm(agent) {
		return nil
	}
	if err := s.retire(retired); err != nil {
		return err
	}
	// The ended group's socket is still on disk, and a server may refuse to
	// listen where one exists; nothing of that group is left to listen on it.
	_ = os.Remove(s.upstream)
	s.tool, s.toolWithdrawn = nil, notConfirmed
	if _, err := s.spawn(s.plainArgs, cancel); err != nil {
		return err
	}
	_, err = s.probeFor(ctx)
	return err
}

// probeFor is the startup probe within its bound, failing as the start
// fails.
func (s *serverSession) probeFor(ctx context.Context) (string, error) {
	connectCtx, stopConnect := context.WithTimeout(ctx, 8*time.Second)
	defer stopConnect()
	agent, err := s.probe(connectCtx)
	if err != nil {
		return "", fmt.Errorf("app-server startup failed (log %s): %w", s.upstream+".log", err)
	}
	return agent, nil
}

// confirm applies the table to a run with the tool, answering whether it
// keeps it. Both reads naming one version keep the choice. An unknown read
// was chosen without a version, so initialize is not consulted. Otherwise
// the choice is made again with no version: kept when it still injects —
// every gate it needs assumed — with the gates of no version, which carry
// no L5 bound; else the tool is withdrawn.
func (s *serverSession) confirm(agent string) bool {
	if s.version.Value == "" || agentVersion(agent) == s.version.Value {
		return true
	}
	gates := harness.ResolveGates("codex", "", s.tool.gates.Assumed())
	if gatesLeaveOut(gates, harness.Version{Unknown: harness.VersionNotRead}) != "" {
		return false
	}
	s.tool.gates = gates
	return true
}

// agentVersion is the version in initialize's userAgent, "<client
// name>/<version> (…)" (docs/research-mail-tool.md), "" where it names none.
func agentVersion(agent string) string {
	client, _, _ := strings.Cut(agent, " ")
	slash := strings.LastIndex(client, "/")
	if slash < 0 {
		return ""
	}
	return client[slash+1:]
}

// userAgent is initialize's userAgent, "" when the answer carries none.
func userAgent(answered json.RawMessage) string {
	var reply struct {
		UserAgent string `json:"userAgent"`
	}
	_ = json.Unmarshal(answered, &reply)
	return reply.UserAgent
}

var _ harness.ToolWithdrawer = (*serverSession)(nil)

// ToolWithdrawn says why a run chosen with the tool started without it, ""
// when it was not withdrawn.
func (s *serverSession) ToolWithdrawn() string { return s.toolWithdrawn }
