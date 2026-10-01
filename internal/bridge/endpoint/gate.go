package endpoint

import (
	"sync"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/inbox"
)

// Gate orders the acknowledgments of reads against the turn ends this
// wrapper captures (docs/mail-bridge-turns.md#a-turns-end-meets-its-calls).
// One mutex covers both: an acknowledgment checks for a noted end and
// registers as writing under it, and a capture notes its end under it. So an
// acknowledgment that reaches its check after the note writes nothing, and
// the one already writing hands the capture the snapshot it closes with,
// taken while it still holds the mailbox lock.
type Gate struct {
	mu       sync.Mutex
	noted    int64
	writing  *slot
	snapshot func() *inbox.ReadBoundary
	now      func() int64
}

// slot is where one writing acknowledgment leaves its closing snapshot. It is
// filled once, by its own acknowledgment, so no later one can stand in for it.
type slot struct {
	done     chan struct{}
	boundary *inbox.ReadBoundary
}

// NewGate makes the gate of a run whose read clock snapshot reads. A nil
// snapshot captures no boundary, as a run without a clock reports none.
func NewGate(snapshot func() *inbox.ReadBoundary) *Gate {
	return &Gate{snapshot: snapshot, now: boottime.Now}
}

func (g *Gate) sample() *inbox.ReadBoundary {
	if g.snapshot == nil {
		return nil
	}
	return g.snapshot()
}

// Enter is the acknowledgment's check, made holding the mailbox lock before
// its first write. It refuses when an end at or after calledBoot was noted.
// Otherwise the acknowledgment is registered as writing, and leave — called
// once whatever its writes did, before the mailbox lock goes — closes it with
// a snapshot of the read clock.
func (g *Gate) Enter(calledBoot int64) (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	// Two acknowledgments cannot both hold the mailbox lock; one found
	// writing is one that never closed, and nothing is added beside it.
	if g.noted >= calledBoot || g.writing != nil {
		return nil, false
	}
	own := &slot{done: make(chan struct{})}
	g.writing = own
	var once sync.Once
	return func() {
		once.Do(func() {
			boundary := g.sample()
			g.mu.Lock()
			own.boundary = boundary
			close(own.done)
			if g.writing == own {
				g.writing = nil
			}
			g.mu.Unlock()
		})
	}, true
}

// Capture notes a turn's end and returns its boundary with the moment it was
// noted on the boot clock. With an acknowledgment writing, the boundary is the
// snapshot that one closes with, and the capture waits for it without the
// mutex: the one wait of rule 9 without a bound of its own. With none, the
// clock is sampled before the mutex goes, so no acknowledgment can register
// in between.
func (g *Gate) Capture() (*inbox.ReadBoundary, int64) {
	g.mu.Lock()
	noted := g.now()
	g.noted = max(g.noted, noted)
	if own := g.writing; own != nil {
		g.mu.Unlock()
		<-own.done
		return own.boundary, noted
	}
	boundary := g.sample()
	g.mu.Unlock()
	return boundary, noted
}

// Stamp is the time a ticket is issued at, taken under the mutex: false when
// an end was noted at or after since, the moment the call was heard. A
// capture after the stamp notes a later moment, so the ticket's
// acknowledgment is refused at Enter; one before it refuses the ticket.
func (g *Gate) Stamp(since int64) (int64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.noted >= since {
		return 0, false
	}
	return g.now(), true
}

// Noted is the latest end this gate noted, zero before any.
func (g *Gate) Noted() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.noted
}
