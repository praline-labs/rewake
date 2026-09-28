package claude

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// A Claude Code session passes every line on its inbox socket through an
// inbound gate, which may accept it, hold it until somebody releases it, or
// refuse it (docs/research-launch.md). A write that succeeded says nothing
// about which. The session tells only a sender that asked: a line naming a
// reply socket in "from" and a UUID in "msg_id" gets a receipt on that socket
// for every step but a plain accept.
//
// The receiver checks that the reply socket is listened on by the very process
// that wrote the line — same pid, uid and start time — so the listener has to
// live where the writer does: in the wrapper, whose inbox server writes every
// notice of this session. `rewake send` could not do it; it only puts a file in
// a mailbox and is gone long before a hold ends.

// receiptWindow is how long Deliver waits for a first receipt. An accepted line
// gets none, so every accepted delivery waits this long; a held one was
// answered within 30 ms live on 2.1.280. Past the window the delivery counts
// as accepted, as it did before receipts — but only for now: a machine under
// load may answer later, so the line is kept for inbox.LateWordWindow, and a
// late word goes to the inbox server, which takes the delivery back.
const receiptWindow = 300 * time.Millisecond

// openingLimit is how long the first notice waits for the session to draw its
// status line, which is when it stops holding what arrives (telemetry.Drawn).
// The status line never comes when the tap is not installed — the settings
// could not be merged, or a managed policy owns the status line — and then
// mail goes out after this.
const openingLimit = 3 * time.Second

// lane delivers notices with a reply address and routes what comes back.
type lane struct {
	reply string
	// ownDir says the reply socket's directory is rewake's own, to be made if
	// the session has not made it yet; beside a socket the caller named it is
	// left as it is.
	ownDir bool
	drawn  <-chan struct{}
	// marks, when set, says who interrupted the last turn.
	marks  interruptMarks
	window time.Duration
	limit  time.Duration

	mu       sync.Mutex
	listener *net.UnixListener
	// sent maps a line's msg_id to what Deliver is waiting on, or to a held
	// notice whose later words go to the inbox server.
	sent map[string]*sentNotice
	// silent lists lines counted delivered from silence, oldest first, so they
	// can be forgotten by age and by number.
	silent []string
	// queue holds words for the inbox server in the order they came. It is
	// drained by one goroutine so that neither Deliver, which runs on the
	// server's own goroutine, nor the reader ever blocks on the server.
	queue    []inbox.Receipt
	queued   chan struct{}
	receipts chan inbox.Receipt
	opened   chan struct{}
	done     chan struct{}
	once     sync.Once
}

// sentNotice is one line waiting for its receipts.
type sentNotice struct {
	// id is the announcement's id, which the inbox server knows it by.
	id    string
	msgID string
	// first carries the first word to Deliver while it waits.
	first chan inbox.Result
	// answered says the first word has been passed to Deliver.
	answered bool
	// claimed says Deliver returned held: every further word is the server's.
	claimed bool
	// backlog keeps words that came after the first and before the claim.
	backlog []inbox.Result
	// at is when a line counted delivered from silence was counted so.
	at time.Time
}

// newLane listens at reply once started; drawn, when not nil, opens the gate on
// the first notice.
func newLane(reply string, ownDir bool, drawn <-chan struct{}, marks interruptMarks) *lane {
	return &lane{
		reply:    reply,
		ownDir:   ownDir,
		drawn:    drawn,
		marks:    marks,
		window:   receiptWindow,
		limit:    openingLimit,
		sent:     map[string]*sentNotice{},
		queued:   make(chan struct{}, 1),
		receipts: make(chan inbox.Receipt),
		opened:   make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (l *lane) Receipts() <-chan inbox.Receipt { return l.receipts }

func (l *lane) Opened() <-chan struct{} { return l.opened }

// Start opens the gate on time and listens for receipts. The gate opens even
// when listening fails: that costs the receipts, not delivery.
func (l *lane) Start(ctx context.Context) error {
	go l.open(ctx)
	go l.forward()
	if l.reply == "" {
		return errors.New("not hearing back about held messages: no reply socket for this session")
	}
	if l.ownDir {
		// The session makes this directory for its own socket only after
		// the lane has to listen.
		if err := state.EnsureSubdir(filepath.Dir(l.reply)); err != nil {
			return fmt.Errorf("not hearing back about held messages: %w", err)
		}
		sweepReplies(filepath.Dir(l.reply))
	}
	if info, err := os.Lstat(l.reply); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("not hearing back about held messages: %s is taken by something that is not a socket", l.reply)
		}
		// The path names this run, so a leftover belongs to a wrapper that died.
		_ = os.Remove(l.reply)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: l.reply, Net: "unix"})
	if err != nil {
		return fmt.Errorf("not hearing back about held messages: %w", err)
	}
	if err := os.Chmod(l.reply, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(l.reply)
		return fmt.Errorf("not hearing back about held messages: %w", err)
	}
	l.mu.Lock()
	l.listener = listener
	l.mu.Unlock()
	go l.accept(listener)
	return nil
}

func (l *lane) open(ctx context.Context) {
	timer := time.NewTimer(l.limit)
	defer timer.Stop()
	select {
	case <-l.drawn:
	case <-timer.C:
	case <-ctx.Done():
	case <-l.done:
	}
	close(l.opened)
}

// Close stops listening and removes the reply socket.
func (l *lane) Close() {
	l.once.Do(func() {
		close(l.done)
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.listener != nil {
			_ = l.listener.Close()
			_ = os.Remove(l.reply)
		}
	})
}

// Deliver writes the notice with a reply address and waits a moment for the
// first word about it. Held is returned as held; a refusal as failed; silence
// as delivered, which is what the gate says by saying nothing.
func (l *lane) Deliver(ctx context.Context, session registry.Session, message inbox.Message) inbox.Result {
	l.mu.Lock()
	listening := l.listener != nil
	l.mu.Unlock()
	interrupter, told := l.interruption()
	notice := newEnvelope(message, interrupter)
	if !listening {
		return told(writeNotice(ctx, session, notice))
	}
	notice.From, notice.MsgID = "uds:"+l.reply, newMsgID()
	waiting := &sentNotice{id: message.ID, msgID: notice.MsgID, first: make(chan inbox.Result, 1)}
	l.mu.Lock()
	l.sent[notice.MsgID] = waiting
	l.mu.Unlock()

	result := told(writeNotice(ctx, session, notice))
	if result.State != inbox.Delivered {
		l.forget(notice.MsgID)
		return result
	}
	timer := time.NewTimer(l.window)
	defer timer.Stop()
	select {
	case word := <-waiting.first:
		return l.answered(waiting, word)
	case <-timer.C:
	case <-ctx.Done():
	}
	l.mu.Lock()
	if waiting.answered {
		// The word came as the window closed.
		l.mu.Unlock()
		return l.answered(waiting, <-waiting.first)
	}
	now := time.Now()
	waiting.claimed, waiting.at = true, now
	l.silent = append(l.silent, notice.MsgID)
	l.pruneSilent(now)
	l.mu.Unlock()
	return result
}

// answered returns the first word Deliver got. After a hold the later words are
// the server's; after anything else the line is done.
func (l *lane) answered(waiting *sentNotice, word inbox.Result) inbox.Result {
	if word.State == inbox.Held {
		l.claim(waiting)
		return word
	}
	l.forget(waiting.msgID)
	return word
}

// pruneSilent forgets lines counted delivered from silence once they are older
// than the window the server keeps them for, or beyond its number. The caller
// holds the lock.
func (l *lane) pruneSilent(now time.Time) {
	keep := l.silent[:0]
	for index, msgID := range l.silent {
		waiting, ok := l.sent[msgID]
		switch {
		case !ok || waiting.at.IsZero():
			// Settled, or held after all and waiting for its last word.
		case now.Sub(waiting.at) > inbox.LateWordWindow || len(l.silent)-index > inbox.LateWordKeep:
			delete(l.sent, msgID)
		default:
			keep = append(keep, msgID)
		}
	}
	l.silent = keep
}

// claim hands the notice's later words to the server, starting with those that
// came while Deliver was returning.
func (l *lane) claim(waiting *sentNotice) {
	l.mu.Lock()
	defer l.mu.Unlock()
	waiting.claimed = true
	for _, word := range waiting.backlog {
		l.enqueue(inbox.Receipt{ID: waiting.id, Result: word})
		if word.State != inbox.Held {
			delete(l.sent, waiting.msgID)
		}
	}
	waiting.backlog = nil
}

func (l *lane) forget(msgID string) {
	l.mu.Lock()
	delete(l.sent, msgID)
	l.mu.Unlock()
}

// route passes one word to whoever waits for it. The caller holds no lock.
//
// Each receipt comes on a connection of its own and is read on a goroutine of
// its own, so two that follow closely may be routed in either order. A final
// word wins either way: once one is routed the line is forgotten, and a "held"
// read after it goes nowhere.
func (l *lane) route(msgID string, word inbox.Result) {
	l.mu.Lock()
	defer l.mu.Unlock()
	waiting, ok := l.sent[msgID]
	if !ok {
		return
	}
	if !waiting.at.IsZero() && time.Since(waiting.at) > inbox.LateWordWindow {
		// Too late for the server too; the next prune would have dropped it.
		delete(l.sent, msgID)
		return
	}
	switch {
	case waiting.claimed:
		l.enqueue(inbox.Receipt{ID: waiting.id, Result: word})
		if word.State != inbox.Held {
			delete(l.sent, msgID)
		} else {
			// A hold outlives the window for late words: its end may be minutes off.
			waiting.at = time.Time{}
		}
	case !waiting.answered:
		// Deliver returns this one, and forgets the id unless it is held.
		waiting.answered = true
		waiting.first <- word
	default:
		waiting.backlog = append(waiting.backlog, word)
	}
}

// enqueue adds a word for the server. The caller holds the lock.
func (l *lane) enqueue(receipt inbox.Receipt) {
	l.queue = append(l.queue, receipt)
	select {
	case l.queued <- struct{}{}:
	default:
	}
}

// forward hands queued words to the server one at a time, in order.
func (l *lane) forward() {
	for {
		l.mu.Lock()
		var next *inbox.Receipt
		if len(l.queue) > 0 {
			next = &l.queue[0]
			l.queue = l.queue[1:]
		}
		l.mu.Unlock()
		if next == nil {
			select {
			case <-l.queued:
				continue
			case <-l.done:
				return
			}
		}
		select {
		case l.receipts <- *next:
		case <-l.done:
			return
		}
	}
}

// newMsgID is a random UUID, the only form the receiver quotes back.
func newMsgID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// replyPath is where this run hears back about its notices. It has to be a
// .sock in the same directory as the session's own socket, the only place
// Claude Code sends a receipt to (docs/research-launch.md). Beside a socket of
// rewake's own it is that socket's name with .reply; beside one the caller
// named, or where that name would be too long, a name of this run's own.
func replyPath(request harness.LaunchRequest, socket string, owns bool) string {
	if socket == "" {
		return ""
	}
	if own := strings.TrimSuffix(socket, ".sock") + replySuffix; owns && len(own) <= maxSocketPath {
		return own
	}
	sum := sha256.Sum256([]byte(request.Name + "\x00" + request.Epoch))
	return filepath.Join(filepath.Dir(socket), fmt.Sprintf("rewake-%x.reply.sock", sum[:8]))
}
