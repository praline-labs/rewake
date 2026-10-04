package endpoint

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/proc"
)

// What the wrapper matches (docs/mail-bridge-server.md#what-the-wrapper-matches).
// The harness's own event records a call first or second — neither harness
// orders the two — and the server's request waits for it, on its own
// channel, holding nothing the observation needs. One observation gives one
// ticket, and the ticket is confirmed once, for one process.

// observation is what the harness recorded of one call.
type observation struct {
	conversation, turn, digest string
	// prompt is a Claude Code call's prompt_id, from which its turn is
	// named when the ticket is issued.
	prompt string
	// refusal says why the call cannot be matched: a second observation
	// of the same call, a nested agent, words off the surface.
	refusal string
	// at is when the observation was heard, on the boot clock: a ticket is
	// issued only while no end was noted since.
	at int64
	// limits are what a Claude Code hook saw of the call's limits; nil
	// from a harness, or a hook, that sends none (limits.go).
	limits *HookLimits
}

// call is one native call, from the first thing heard of it until it ages
// out of the table.
type call struct {
	ready    chan struct{}
	observed *observation
	issued   *issued
	created  time.Time
	// completed says the harness's result of the call was handled: a
	// repeat of it is the same event, and is ignored whatever the first
	// one's acknowledgment did.
	completed bool
}

// issued is a ticket and who used it.
type issued struct {
	ticket bridge.Ticket
	used   bool
	pid    int
	start  uint64
}

// calls is the table, held only to record or take an entry.
type calls struct {
	mu      sync.Mutex
	byCall  map[string]*call
	order   []string
	byNonce map[string]*issued
	// open are the Codex turns of the primary thread seen started and not
	// completed, with when each started; firstSeen when each Claude Code
	// prompt was first heard.
	open      map[string]int64
	firstSeen map[string]int64
	prompts   []string
	// spent are the native calls given a ticket in this run. The table
	// forgets a call under pressure or with age; this set does not, so a
	// call heard again after that is never taken for a new one.
	spent map[string]bool
	// now is the clock calls age by.
	now func() time.Time
}

// The table's bounds: a call ages out after callLife, and the table keeps at
// most maxCalls; an older call's ticket is past its deadline long before.
// A run issues at most maxSpent tickets, the calls it can remember having
// served; past them, a call is refused rather than possibly served twice.
const (
	maxCalls   = 512
	callLife   = 2 * time.Minute
	maxTurns   = 64
	maxPrompts = 256
	maxSpent   = 1 << 16
)

func newCalls() *calls {
	return &calls{
		byCall: map[string]*call{}, byNonce: map[string]*issued{}, open: map[string]int64{},
		firstSeen: map[string]int64{}, spent: map[string]bool{}, now: time.Now,
	}
}

// entry returns the call of id, adding it; nil when the table is full of
// calls too young to drop. Called under c.mu.
func (c *calls) entry(id string, now time.Time) *call {
	if existing := c.byCall[id]; existing != nil {
		return existing
	}
	for len(c.order) > 0 {
		oldest := c.byCall[c.order[0]]
		if len(c.order) < maxCalls && now.Sub(oldest.created) < callLife {
			break
		}
		if now.Sub(oldest.created) < callLife && oldest.issued != nil && !oldest.issued.used {
			return nil
		}
		if oldest.issued != nil {
			delete(c.byNonce, oldest.issued.ticket.Nonce)
		}
		delete(c.byCall, c.order[0])
		c.order = c.order[1:]
	}
	fresh := &call{ready: make(chan struct{}), created: now}
	c.byCall[id] = fresh
	c.order = append(c.order, id)
	return fresh
}

// observe records the harness's own record of a call.
func (e *Endpoint) observe(id string, seen observation) {
	if id == "" {
		return
	}
	c := e.calls
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entry(id, c.now())
	if entry == nil {
		return
	}
	if entry.observed != nil {
		entry.observed.refusal = "the harness reported this call twice, so it cannot be told which one runs"
		return
	}
	if c.spent[id] {
		seen.refusal = "the harness reported this call again after its ticket was issued"
	}
	seen.at = boottime.Now()
	entry.observed = &seen
	close(entry.ready)
}

// observedWords normalizes the words a harness recorded, or says why they
// cannot run: the request carries the same words and is refused for them.
func (e *Endpoint) observedWords(words []string, ok bool) (string, string) {
	if !ok {
		return "", "the call's arguments are not one words array of strings"
	}
	normalized, err := e.cfg.Words(words)
	if err != nil {
		return "", "the call's words do not run through the tool"
	}
	return bridge.Digest(normalized), ""
}

// issue matches a server's request with the harness's observation of the same
// call, and issues its one ticket.
func (e *Endpoint) issue(asked TicketRequest) (bridge.Ticket, error) {
	if e.cfg.Refusal != "" && e.cfg.Transport != bridge.ClaudeTransport {
		return bridge.Ticket{}, errors.New(e.cfg.Refusal)
	}
	if asked.Transport != e.cfg.Transport || asked.CallID == "" {
		return bridge.Ticket{}, errors.New("the call names another transport or no native call")
	}
	c := e.calls
	c.mu.Lock()
	entry := c.entry(asked.CallID, c.now())
	c.mu.Unlock()
	if entry == nil {
		return bridge.Ticket{}, errors.New("too many calls are waiting for their tickets")
	}
	timer := time.NewTimer(e.cfg.Wait)
	defer timer.Stop()
	select {
	case <-entry.ready:
	case <-timer.C:
		// The observer is gone: every call is refused until it returns.
		e.tell(channel.Event{Kind: channel.NotObserved})
		return bridge.Ticket{}, fmt.Errorf("the harness did not report this call within %s", e.cfg.Wait)
	case <-e.done:
		return bridge.Ticket{}, errors.New("the wrapper is closing")
	}
	c.mu.Lock()
	seen := *entry.observed
	first := c.firstSeen[seen.prompt]
	c.mu.Unlock()
	deadline, refusal := e.deadline(seen)
	if refusal != "" {
		return bridge.Ticket{}, errors.New(refusal)
	}
	turn, err := e.turnOf(asked, seen, first)
	if err != nil {
		return bridge.Ticket{}, err
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return bridge.Ticket{}, err
	}
	readsOff := e.readsOffNow()
	c.mu.Lock()
	defer c.mu.Unlock()
	started, open := c.open[seen.turn]
	switch {
	case entry.observed.refusal != "":
		return bridge.Ticket{}, errors.New(entry.observed.refusal)
	case entry.issued != nil || c.spent[asked.CallID]:
		return bridge.Ticket{}, errors.New("this call already has its ticket")
	case len(c.spent) >= maxSpent:
		return bridge.Ticket{}, errors.New("this run has issued all the tickets it can remember")
	case seen.digest != asked.Digest:
		return bridge.Ticket{}, errors.New("the harness recorded other words for this call")
	case e.cfg.Transport == bridge.CodexTransport && !open:
		return bridge.Ticket{}, errors.New("the call's turn is completed or was never seen to start")
	}
	// The ticket's time says the call belongs to a turn still open: so it
	// is taken under the gate, and only while no end was noted since the
	// call was heard — or, on Codex, since its turn started. An end
	// captured before turn/completed reaches the table closes the turn
	// already (docs/mail-bridge-turns.md#a-turns-end-meets-its-calls).
	since := seen.at
	if e.cfg.Transport == bridge.CodexTransport {
		since = min(since, started)
	}
	now, ok := e.cfg.Gate.Stamp(since)
	if !ok {
		return bridge.Ticket{}, errors.New("the call's turn ended before its ticket was issued")
	}
	ticket := bridge.Ticket{
		Capability: e.cfg.Capability, Conversation: seen.conversation, Turn: turn, CallID: asked.CallID,
		CalledBoot: now, DeadlineBoot: now + int64(deadline), WordsDigest: seen.digest,
		Transport: e.cfg.Transport, Nonce: hex.EncodeToString(nonce), ReadsOff: readsOff,
	}
	entry.issued = &issued{ticket: ticket}
	c.byNonce[ticket.Nonce] = entry.issued
	c.spent[asked.CallID] = true
	return ticket, nil
}

// turnOf names the call's turn as the ticket carries it
// (docs/mail-bridge-turns.md#the-turn-a-call-belongs-to).
func (e *Endpoint) turnOf(asked TicketRequest, seen observation, first int64) (string, error) {
	if seen.refusal != "" {
		return "", errors.New(seen.refusal)
	}
	if e.cfg.Transport == bridge.CodexTransport {
		if asked.Conversation != seen.conversation || asked.Turn != seen.turn || seen.turn == "" {
			return "", errors.New("the call's thread or turn is not the one the harness reported")
		}
		return seen.turn, nil
	}
	conversation := ""
	if e.cfg.Conversation != nil {
		conversation = e.cfg.Conversation()
	}
	if conversation == "" || seen.conversation != conversation {
		return "", errors.New("the call is not in the conversation this run holds")
	}
	if seen.prompt == "" || first == 0 {
		return "", errors.New("the call names no prompt")
	}
	// A prompt's identity outlives an end another hook blocked after ours
	// published: the turn goes on, under a name that says which end it
	// follows. An end before the prompt was first seen is an earlier
	// prompt's, however late its journal.
	ended, err := inbox.LatestEnd(e.cfg.Dir, e.cfg.Name, e.cfg.Epoch)
	if err != nil {
		return "", fmt.Errorf("the turn journals cannot be read (%v), so the call's turn is unknown", err)
	}
	if ended >= first {
		return seen.prompt + "@" + strconv.FormatInt(ended, 10), nil
	}
	return seen.prompt, nil
}

// confirm validates a ticket for the child presenting it, once.
func (e *Endpoint) confirm(conn *net.UnixConn, ticket bridge.Ticket) error {
	peer, err := peerOf(conn)
	if err != nil {
		return err
	}
	start, _ := proc.StartTime(int(peer.Pid))
	c := e.calls
	c.mu.Lock()
	defer c.mu.Unlock()
	held := c.byNonce[ticket.Nonce]
	switch {
	case held == nil:
		return errors.New("a ticket this wrapper did not issue, or one too old to keep")
	case held.ticket != ticket:
		return errors.New("the ticket differs from the one issued")
	case held.used:
		return fmt.Errorf("the ticket was already used, by process %d", held.pid)
	case boottime.Now() >= ticket.DeadlineBoot:
		return errors.New("the ticket's deadline passed")
	}
	held.used, held.pid, held.start = true, int(peer.Pid), start
	// The tool's one evidence: a child that validated its ticket, whatever
	// its words. Told under the lock is fine: the wrapper only queues it.
	e.tell(channel.Event{Kind: channel.Validated, Issued: ticket.CalledBoot})
	return nil
}

// turnStarted and turnEnded follow the Codex turns of the primary thread.
func (e *Endpoint) turnStarted(turn string) {
	c := e.calls
	c.mu.Lock()
	defer c.mu.Unlock()
	if turn == "" || len(c.open) >= maxTurns {
		return
	}
	if _, seen := c.open[turn]; !seen {
		c.open[turn] = boottime.Now()
	}
}

func (e *Endpoint) turnEnded(turn string) {
	c := e.calls
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.open, turn)
}

// promptSeen keeps when a Claude Code prompt was first heard.
func (e *Endpoint) promptSeen(prompt string) {
	c := e.calls
	c.mu.Lock()
	defer c.mu.Unlock()
	if prompt == "" || c.firstSeen[prompt] != 0 {
		return
	}
	if len(c.prompts) >= maxPrompts {
		delete(c.firstSeen, c.prompts[0])
		c.prompts = c.prompts[1:]
	}
	c.firstSeen[prompt] = boottime.Now()
	c.prompts = append(c.prompts, prompt)
}
