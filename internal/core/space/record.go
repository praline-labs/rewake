/*
Package space keeps the record of each space a main's run opens: a worker that
a detached space host runs in a pseudo-terminal of its own, in main's room, and
closes once main's run has ended (docs/rules/space.md).

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
	"strings"
	"time"
)

// State is where a space stands. The order below is the only order a record
// moves in; a final state never changes but for a kept checkout observed
// gone, which makes the space closed.
type State string

const (
	// Opening means the record is written, the worker not yet started. What
	// the worker will be — harness, name, role, session id — is fixed here,
	// before anything starts.
	Opening State = "opening"
	// Open means the worker's wrapper was started, its run recorded.
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
	// Checkout is the worker's one worktree: the one its launch made, or the
	// one it entered mid-session (enter_worktree = true). Empty for a worker
	// without one, and until its session is seen to claim it.
	Checkout Checkout `json:"checkout"`
	// Host is the space host's run, pid.start.boot as an epoch: the process
	// that holds the worker's terminal and closes it. Set before the worker
	// starts.
	Host     string    `json:"host,omitempty"`
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
	// Name is the prefix the worker is launched with, --name: its session is
	// <name>-<harness>.
	Name string `json:"name"`
	Role string `json:"role"`
	// Type is the worker type the space was opened with, its name only (the
	// text travels in the spec); optional, absent from records of before the
	// field and of a worker without a type.
	Type string `json:"type,omitempty"`
	// Label is the template's name for its model (launch.label), shown when
	// the harness gives none; optional, absent from older records.
	Label string `json:"label,omitempty"`
	// SessionID is the conversation id rewake chose before the launch;
	// empty for a harness that takes none, which the record so says.
	SessionID string `json:"sessionId,omitempty"`
	// Run is the worker's own run, once its wrapper is started.
	Run string `json:"run,omitempty"`
	// Dev says the worker was raised with space open --dev, drawn by
	// rewake's own face beside its native one (docs/face.md); optional,
	// absent from older records and from a worker without it.
	Dev bool `json:"dev,omitempty"`
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
}

// staysMark joins to an end's reason why the worker's checkout was not
// removed at the close: the reason stays one field of the record, and the
// views split it again.
const staysMark = "; its checkout stays: "

// WithStays is an end's reason with why its checkout stays, when it does.
func WithStays(why, stays string) string {
	if stays == "" {
		return why
	}
	return why + staysMark + stays
}

// Reasons are the end's reason without why its checkout stays, and that
// part, empty for a checkout removed or never made.
func (e End) Reasons() (why, stays string) {
	why, stays, _ = strings.Cut(e.Why, staysMark)
	return why, stays
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

// Seen is a checkout as closing found it.
type Seen struct {
	// Presence is what closing saw of the checkout: there, gone or never
	// made (gone, with no checkout recorded), or unknown when it could not be looked at. It is always set, so
	// an observation nobody made never reads as a checkout seen gone (SP2).
	Presence Presence `json:"presence"`
	// Changes, Commits and Locked are what was found in a checkout there:
	// changes or untracked files, commits beyond its base, a Git lock.
	Changes bool `json:"changes,omitempty"`
	Commits bool `json:"commits,omitempty"`
	Locked  bool `json:"locked,omitempty"`
}

// Presence is whether a checkout is there, as closing found it or as it is
// looked at later.
type Presence string

// The presences of a checkout: there, gone, or not to be told.
const (
	There Presence = "there"
	Gone  Presence = "gone"
	// Unknown is a checkout whose presence cannot be told; it is kept and
	// reminded as if there, since a kept checkout is never forgotten (SP7).
	Unknown Presence = "unknown"
)

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
