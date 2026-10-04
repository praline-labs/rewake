package channel

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"
)

// Notices (docs/mail-bridge-channel.md#notices): which category entered is
// told to whom, the windows that suppress a repeat, and the identity of each
// publication, fixed before its first attempt.

// The roles a notice goes to.
const (
	ToWorker = "worker"
	ToMain   = "main"
)

// The first lines, fixed per category and class: at most 55 cells, so a
// grouped notice's "<sender> notify: " prefix with a 32-character sender
// keeps them whole in the 96-cell preview.
const (
	FirstWorkerFailing = "rewake tool failed; new calls in the shell, no repeats."
	FirstMainFailing   = "rewake tool failed; shell not yet confirmed."
	FirstMainShell     = "mail goes through the shell."
	FirstMainNone      = "no mail channel works; see whoami there."
	FirstMainDenied    = "rewake tool denied by the person's policy."
	FirstMainWorks     = "rewake tool works again."
	FirstMainNoTool    = "started without the rewake tool; shell only."
)

// MainReconnect is main's second line when Claude Code's server is gone:
// the harness starts it again only when the person reconnects it.
const MainReconnect = "the person can reconnect it with /mcp in that session"

// WorkerAdvice is the worker's second line: a call whose outcome is unknown
// is continued, never repeated by its words.
const WorkerAdvice = "a call whose outcome is unknown: rewake retry <token>; never its words in the shell."

// The windows: one notice per key per ten minutes, six per hour from one run
// to one recipient. Both chosen, not measured; both count landed notices.
const (
	KeyWindow  = 10 * time.Minute
	HourWindow = time.Hour
	HourLimit  = 6
)

// Recipient is who a notice goes to: a role and the run that holds it.
type Recipient struct {
	Role, Name, Epoch string
}

func (r Recipient) id() string { return r.Role + "\x00" + r.Name + "\x00" + r.Epoch }

// The states of a publication.
const (
	Pending = "pending"
	Landed  = "landed"
	Dropped = "dropped"
)

// Publication is one notice, fixed before its first attempt: its number,
// recipient and whole body never change, so a retry lands it once.
type Publication struct {
	Seq   uint64    `json:"seq"`
	To    Recipient `json:"to"`
	Key   string    `json:"key"`
	Body  string    `json:"body"`
	Fixed Stamp     `json:"fixed"`
	// Advice says it advises the shell: one that has not landed when the
	// block is set is dropped, never published.
	Advice bool   `json:"advice,omitempty"`
	State  string `json:"state"`
}

// ID is the publication's message ID: the run, the recipient, its epoch and
// the number, after the fixed time so a mailbox sorts it.
func (p Publication) ID(runName, runEpoch string) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "channel\x00%s\x00%s\x00%s\x00%s\x00%d", runName, runEpoch, p.To.Name, p.To.Epoch, p.Seq))
	return fmt.Sprintf("%019d-%x", p.Fixed.Wall.UnixNano(), sum[:12])
}

// Notices is a run's notice record.
type Notices struct {
	Seq          uint64               `json:"seq"`
	Told         map[string]string    `json:"told,omitempty"`
	Landings     map[string][]landing `json:"landings,omitempty"`
	Publications []Publication        `json:"publications,omitempty"`
}

// landing is a landed notice, for the windows.
type landing struct {
	Key string `json:"key"`
	At  int64  `json:"at"`
}

// due is what a recipient should be told of a record now: a key and a
// body, or "" for nothing. reset says the recipient's view moved somewhere
// that tells it nothing, so a later return is told again.
func due(r *Record, to Recipient, told string) (key, body string, advice, reset bool) {
	category := r.Category()
	if to.Role == ToWorker {
		if category != CategoryFailing {
			return "", "", false, true
		}
		key = string(category) + "/" + r.Class
		return key, FirstWorkerFailing + "\n" + WorkerAdvice + "\n" + Label(r), true, false
	}
	var first string
	key = string(category)
	switch category {
	case CategoryFailing:
		first, key = FirstMainFailing, key+"/"+r.Class
		if r.Harness == Claude && r.Class == ClassServerGone {
			first += "\n" + MainReconnect
		}
	case CategoryShell:
		first, key = FirstMainShell, key+"/"+r.toolPart()
	case CategoryNoChannel:
		first, key = FirstMainNone, key+"/"+r.toolPart()+"/"+r.Shell.Class
	case CategoryDenied:
		first = FirstMainDenied
	case CategoryNoTool:
		first, key = FirstMainNoTool, key+"/"+r.Reason
	case CategoryTool:
		// Only to a main that was told of a failure or a denial.
		if told == "" || told == string(CategoryTool) {
			return "", "", false, false
		}
		first = FirstMainWorks
	default:
		return "", "", false, false
	}
	return key, first + "\n" + Label(r), false, false
}

// Plan fixes the notice due to a recipient now, if the windows allow one,
// and answers it; false when nothing is due. A recipient whose category is
// suppressed is asked again later: when its window ends, the current
// category goes if it differs from the last one told.
//
// A recipient has at most one notice fixed and not yet settled. The windows
// count landings, and notices fixed while a mailbox cannot be written would
// otherwise land together once it can, past both bounds; held back instead,
// the change is told at the first heartbeat after the one in flight settles,
// by the current category against the last one told.
func (n *Notices) Plan(r *Record, to Recipient, now Stamp) (Publication, bool) {
	if r.Frozen {
		return Publication{}, false
	}
	if n.Told == nil {
		n.Told, n.Landings = map[string]string{}, map[string][]landing{}
	}
	told := n.Told[to.id()]
	key, body, advice, reset := due(r, to, told)
	if reset {
		n.Told[to.id()] = ""
	}
	if key == "" || key == told || n.inFlight(to) || !n.allowed(to, key, now.Boot) {
		return Publication{}, false
	}
	n.Seq++
	fixed := Publication{Seq: n.Seq, To: to, Key: key, Body: body, Fixed: now, Advice: advice, State: Pending}
	n.Publications = append(n.Publications, fixed)
	n.Told[to.id()] = key
	return fixed, true
}

// inFlight says whether a notice to the recipient is fixed and unsettled.
func (n *Notices) inFlight(to Recipient) bool {
	for _, publication := range n.Publications {
		if publication.State == Pending && publication.To.id() == to.id() {
			return true
		}
	}
	return false
}

// ErrNotWritable is a fixed notice that may not be written now, and never
// will be: the record froze, or the block was set over shell advice.
var ErrNotWritable = errors.New("the notice may no longer be written")

// Writable is the check under the recipient's mailbox lock, just before a
// fixed notice is written: nothing is written once the record froze, however
// early the notice was fixed, and no shell advice once the block is set. A
// notice that was written before stays: its retry is recognized by its ID
// before this check is asked.
func (n *Notices) Writable(r *Record, p Publication) error {
	if r.Frozen || p.Advice && r.Blocked() {
		return ErrNotWritable
	}
	return nil
}

// allowed applies the windows, counting landed notices only.
func (n *Notices) allowed(to Recipient, key string, now int64) bool {
	recent := 0
	for _, landed := range n.Landings[to.id()] {
		if landed.Key == key && now-landed.At < int64(KeyWindow) {
			return false
		}
		if now-landed.At < int64(HourWindow) {
			recent++
		}
	}
	return recent < HourLimit
}

// Unsent are the publications still to attempt, in their order.
func (n *Notices) Unsent() []Publication {
	var unsent []Publication
	for _, publication := range n.Publications {
		if publication.State == Pending {
			unsent = append(unsent, publication)
		}
	}
	return unsent
}

// Settle records what an attempt did: landed at a time, or dropped.
func (n *Notices) Settle(seq uint64, state string, at int64) {
	for i := range n.Publications {
		publication := &n.Publications[i]
		if publication.Seq != seq || publication.State != Pending {
			continue
		}
		publication.State = state
		if state == Landed {
			id := publication.To.id()
			n.Landings[id] = append(n.Landings[id], landing{Key: publication.Key, At: at})
		}
	}
	n.prune()
}

// prune forgets settled publications and landings past every window, so
// the record stays small for the life of a run.
func (n *Notices) prune() {
	kept := n.Publications[:0]
	for _, publication := range n.Publications {
		if publication.State == Pending {
			kept = append(kept, publication)
		}
	}
	n.Publications = kept
	for id, landings := range n.Landings {
		if len(landings) > HourLimit*2 {
			n.Landings[id] = landings[len(landings)-HourLimit*2:]
		}
	}
}
