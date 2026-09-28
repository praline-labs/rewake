package grantauth

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/proc"
)

// Keeper holds what was granted to one run of a session whose harness takes a
// grant through a hook it runs itself — Claude Code's permission hook
// (docs/grants.md#claude-code). The hook is a process below the wrapper and
// asks the wrapper what to answer; the journal lives here, in memory, and the
// file under grants/ is only its copy for rewake list.
type Keeper struct {
	// Self is the wrapper; the hook runs below it.
	Self int
	// Decide answers one hook call from the journal.
	Decide func(call json.RawMessage, entries []grant.Entry) Decision
	// Settled says whether a task no longer needs what was granted with it.
	Settled func(id string) bool
	// Mirror writes the copy rewake list shows.
	Mirror func([]grant.Entry)

	listener *net.UnixListener
	mu       sync.Mutex
	entries  []grant.Entry
	// added are the directories the hook put into the session: only those
	// need a hook call to take out again.
	added map[string]bool
	done  chan struct{}
	once  sync.Once
}

// Decision is the hook's answer to one call: what it prints, and which
// directories that answer adds to the session and takes out of it.
type Decision struct {
	Output  []byte
	Added   []string
	Removed []string
}

// Keep binds the run's keeper address. An address already bound is somebody
// else's: the session then takes no grant, and says so.
func Keep(address string, self int) (*Keeper, error) {
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: address, Net: "unix"})
	if err != nil {
		return nil, err
	}
	return &Keeper{Self: self, listener: listener, added: map[string]bool{}, done: make(chan struct{})}, nil
}

// Grant records the directories a message grants, once it is confirmed and
// about to be delivered. The same message again, as a retried delivery asks,
// changes nothing; past grant.MaxLive live directories the grant is refused
// rather than an earlier one forgotten.
func (k *Keeper) Grant(id string, dirs []string, at time.Time) error {
	return k.GrantFrom(grant.Entry{Message: id, At: at}, dirs, false)
}

// GrantFrom records a grant with where it came from — the message, the main
// run that sent it and the conversation it went into, as origin names them —
// so that a run resuming that conversation can find it
// (docs/grants.md#after-a-cold-resume). added says the session has the
// directories already: a resumed one was started with them, and taking them
// back then needs the hook, as for one the hook added.
func (k *Keeper) GrantFrom(origin grant.Entry, dirs []string, added bool) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	id := origin.Message
	if slices.ContainsFunc(k.entries, func(entry grant.Entry) bool { return entry.Message == id }) {
		return nil
	}
	live := 0
	for _, entry := range k.entries {
		if entry.Live() || entry.Revoking() {
			live++
		}
	}
	if live+len(dirs) > grant.MaxLive {
		return fmt.Errorf("this session holds %d granted directories, and %d more would pass the %d rewake keeps; wait for earlier tasks to be reported on", live, len(dirs), grant.MaxLive)
	}
	for _, dir := range dirs {
		entry := origin
		entry.Path, entry.Outcome, entry.EndedAt = dir, grant.Granted, nil
		k.entries = append(k.entries, entry)
		if added {
			k.added[dir] = true
		}
	}
	k.mirror()
	return nil
}

// Entries is a copy of the journal.
func (k *Keeper) Entries() []grant.Entry {
	k.mu.Lock()
	defer k.mu.Unlock()
	return slices.Clone(k.entries)
}

// Serve answers until the context ends or Close is called.
func (k *Keeper) Serve(ctx context.Context) {
	acceptLoop(ctx, k.listener, k.done, k.Close, k.answer)
}

// Close stops answering. What it held goes with it: the harness forgets a
// directory its hook added when the session ends (docs/grants.md#claude-code).
func (k *Keeper) Close() {
	k.once.Do(func() {
		close(k.done)
		_ = k.listener.Close()
	})
}

func (k *Keeper) answer(conn *net.UnixConn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(exchangeLimit))
	line, err := bufio.NewReader(io.LimitReader(conn, maxLine)).ReadBytes('\n')
	if err != nil {
		return
	}
	var asked request
	var answer response
	switch {
	case json.Unmarshal(line, &asked) != nil:
		answer.Error = "a request that does not parse"
	case asked.Op != opDecide:
		answer.Error = "an operation this wrapper does not know: " + asked.Op
	default:
		output, err := k.decide(conn, asked.Call)
		if err != nil {
			answer.Error = err.Error()
		} else {
			answer.Output = output
		}
	}
	_ = json.NewEncoder(conn).Encode(answer)
}

// decide answers a hook call from a process below this wrapper and in its
// namespaces: this session's own harness. Anything else is told nothing.
func (k *Keeper) decide(conn *net.UnixConn, call json.RawMessage) (json.RawMessage, error) {
	peer, err := peerOf(conn)
	if err != nil {
		return nil, fmt.Errorf("cannot tell who asked: %v", err)
	}
	if int(peer.Uid) != os.Getuid() {
		return nil, errors.New("asked by another user")
	}
	if err := proc.Default.Descends(int(peer.Pid), k.Self); err != nil {
		return nil, fmt.Errorf("only this session's harness asks about its grants: %v", err)
	}
	// The harness's own hook runs beside it; a command it started in a
	// sandbox of its own is not the hook, even below this wrapper.
	if err := sameNamespaces(int(peer.Pid)); err != nil {
		return nil, fmt.Errorf("a command in a sandbox of its own is told nothing about the grants: %v", err)
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	changed := k.settle()
	var decided Decision
	if k.Decide != nil {
		decided = k.Decide(call, slices.Clone(k.entries))
	}
	for _, path := range decided.Added {
		k.added[path] = true
	}
	now := time.Now()
	for index := range k.entries {
		entry := &k.entries[index]
		if entry.Revoking() && slices.Contains(decided.Removed, entry.Path) {
			entry.Outcome, entry.EndedAt = grant.Revoked, &now
			delete(k.added, entry.Path)
			changed = true
		}
	}
	if changed {
		k.mirror()
	}
	return decided.Output, nil
}

// settle marks what settled tasks were given as being taken back, and ends
// at once a grant the session never wrote in: nothing was added for it, so
// there is nothing to take out, and the hook need not force a question to do
// it. A directory another live task holds stays as it is. Call with the lock
// held.
func (k *Keeper) settle() bool {
	if k.Settled == nil {
		return false
	}
	known := map[string]bool{}
	changed := false
	now := time.Now()
	for index := range k.entries {
		entry := &k.entries[index]
		if !entry.Live() {
			continue
		}
		settled, ok := known[entry.Message]
		if !ok {
			settled = k.Settled(entry.Message)
			known[entry.Message] = settled
		}
		if !settled {
			continue
		}
		changed = true
		if k.added[entry.Path] {
			entry.Outcome = grant.Revoking
		} else {
			entry.Outcome, entry.EndedAt = grant.Revoked, &now
		}
	}
	return changed
}

// mirror writes the copy. Call with the lock held.
func (k *Keeper) mirror() {
	if k.Mirror != nil {
		k.Mirror(slices.Clone(k.entries))
	}
}

// Ask is the hook's side: it hands one call to its session's wrapper and
// returns what to answer the harness. The answer counts only from the
// wrapper the session's run names, alive as it started.
func Ask(address string, expect Expect, call json.RawMessage) ([]byte, error) {
	conn, err := dial(address)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	if err := expect.servedBy(conn); err != nil {
		return nil, err
	}
	answer, err := exchange(conn, request{Op: opDecide, Call: call})
	if err != nil {
		return nil, err
	}
	if answer.Error != "" {
		return nil, fmt.Errorf("%w: %s", ErrNotConfirmed, answer.Error)
	}
	return answer.Output, nil
}

// servedBy checks that the listener at the other end of conn is the wrapper
// expected: this user's, that pid, alive as it started.
func (e Expect) servedBy(conn *net.UnixConn) error {
	peer, err := peerOf(conn)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if int(peer.Uid) != os.Getuid() || int(peer.Pid) != e.PID || e.PID <= 0 || !proc.Alive(e.PID, e.Start) {
		return fmt.Errorf("%w: the socket is served by process %d, not this session's wrapper %d", ErrNotConfirmed, peer.Pid, e.PID)
	}
	return nil
}
