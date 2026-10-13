/*
Package space keeps the record of each space a main's run opens: a worker that
main's wrapper hosts in a pseudo-terminal of its own, in a checkout of its own
(docs/v2/stage4-space.md).

A record is one JSON file per space under the room's spaces directory, named
by the space's id. It is written before the act it names is answered and
outlives the space: a closed record keeps the worker's session id for a later
restore, and a kept checkout is listed until it is gone, since rewake removes
none (docs/rules/space.md, SP2 and SP7). The core knows no harness: the record
names the worker's by its id.
*/
package space

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"
)

// State is where a space stands. The order below is the only order a record
// moves in; a final state never changes but for a kept checkout observed
// gone, which makes the space closed.
type State string

const (
	// Opening means the record is written, the worker not yet seated. What the
	// worker will be — harness, name, role, session id — is fixed here,
	// before anything starts.
	Opening State = "opening"
	// Open means the worker is seated in its checkout, its run recorded.
	Open State = "open"
	// Closing means the space is being shut down; nothing new is seated.
	Closing State = "closing"
	// Kept means closed, and its checkout was seen still there.
	Kept State = "kept"
	// Closed means closed, and no checkout of it is left.
	Closed State = "closed"
)

// Final reports whether the space is over: its worker's end was observed.
func (s State) Final() bool { return s == Kept || s == Closed }

// rank orders the states a record moves through; kept and closed share the
// last place, one reached from the other only by a checkout observed gone.
func (s State) rank() int {
	switch s {
	case Opening:
		return 1
	case Open:
		return 2
	case Closing:
		return 3
	case Kept, Closed:
		return 4
	}
	return 0
}

// Record is one space.
type Record struct {
	// ID names the space in its record's path and in the mod's protocol.
	ID string `json:"id"`
	// Name is what main and the person call it: unique among the room's
	// spaces that are not over, free again once its space is.
	Name  string `json:"name"`
	State State  `json:"state"`
	// Main is the run that opened the space and that it lives no longer
	// than.
	Main Run `json:"main"`
	// Worker is the session the space hosts.
	Worker Worker `json:"worker"`
	// Checkout is where the worker runs; empty until the keeper's checkout
	// is found, and always set before the worker starts.
	Checkout Checkout `json:"checkout"`
	// Keeper is the agent id of the subagent whose entry is the space's tab;
	// empty until its spawn answers, and for a space hosted without one.
	Keeper   string    `json:"keeper,omitempty"`
	OpenedAt time.Time `json:"openedAt"`
	// Closing is why the space began to close, once it did.
	Closing string `json:"closing,omitempty"`
	// End is what closing observed; set exactly when the state is final.
	End *End `json:"end,omitempty"`
}

// Run names one run of a session: its name and its epoch, which carries the
// wrapper's pid, start time and boot, so whether it still runs is a question
// a later reader can answer.
type Run struct {
	Name string `json:"name"`
	Run  string `json:"run"`
	// PIDNamespace is the pid namespace of that wrapper: only a reader in
	// the same one can tell from its pid whether it runs.
	PIDNamespace string `json:"pidNamespace,omitempty"`
}

// Worker is what the space starts, fixed before it starts.
type Worker struct {
	// Harness is the worker harness's adapter id.
	Harness string `json:"harness"`
	// Name is the prefix the worker is launched with, --name.
	Name string `json:"name"`
	Role string `json:"role"`
	// SessionID is the conversation id rewake chose before the launch;
	// empty for a harness that takes none, which the record so says.
	SessionID string `json:"sessionId,omitempty"`
	// Run is the worker's own run, once its wrapper is seen running.
	Run string `json:"run,omitempty"`
}

// Checkout is the worker's working tree.
type Checkout struct {
	Path   string `json:"path,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// End is what was observed when the space closed: never what was sent, only
// what was seen after (SP2).
type End struct {
	At time.Time `json:"at"`
	// Why the space closed, in words: closed by main, main ended without
	// closing, the open failed and at which step.
	Why    string    `json:"why"`
	Worker WorkerEnd `json:"worker"`
	// Checkout is how the checkout was found once the worker was gone.
	Checkout Seen `json:"checkout"`
	// Cleared is when a checkout left at the close was seen gone, which
	// ends its reminder.
	Cleared *time.Time `json:"cleared,omitempty"`
}

// WorkerEnd is how the worker was seen to end.
type WorkerEnd string

const (
	// WorkerEnded means its wrapper, by pid and start time, was seen gone.
	WorkerEnded WorkerEnd = "ended"
	// WorkerNeverStarted means no run of it was recorded, so its launch gate
	// never opened and nothing of its harness ran; the open stopped first.
	WorkerNeverStarted WorkerEnd = "never-started"
)

// Seen is a checkout as it was found.
type Seen struct {
	// Left says the checkout was still there; false, it was gone or never
	// made.
	Left bool `json:"left"`
	// Changes, Commits and Locked are what was found in a checkout left:
	// changes or untracked files, commits beyond its base, a Git lock such
	// as a dead keeper leaves.
	Changes bool `json:"changes,omitempty"`
	Commits bool `json:"commits,omitempty"`
	Locked  bool `json:"locked,omitempty"`
}

var (
	idShape        = regexp.MustCompile(`^[0-9a-f]{32}$`)
	sessionIDShape = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

// ValidID reports whether id can name a space record.
func ValidID(id string) bool { return idShape.MatchString(id) }

// NewID draws a space's id.
func NewID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("could not draw a space id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

// NewSessionID draws the worker's conversation id, a random UUID, chosen
// before the launch so the record holds it before the worker starts.
func NewSessionID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("could not draw a session id: %w", err)
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	text := hex.EncodeToString(raw[:])
	return text[0:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:], nil
}
