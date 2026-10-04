// Package channel is the record of how a run's mail travels — through the
// mail tool, through the shell, or neither — folded from events the wrapper
// observes (docs/mail-bridge-channel.md). It holds no clock, no file and no
// process: the wrapper feeds it events with their own times, and it answers
// what to show and what to tell.
package channel

import (
	"slices"
	"time"

	"github.com/praline-labs/rewake/internal/receipt"
)

// The tool observation's states.
const (
	ToolStarting  = "starting"
	ToolConnected = "connected"
	ToolWorking   = "working"
	ToolFailing   = "failing"
	ToolNone      = "no tool"
)

// The transport failure classes: a closed list, each from the endpoint's own
// evidence or its timer, never from a harness's error text.
const (
	ClassServerGone    = "server gone"
	ClassServerRefused = "server refused"
	ClassCannotStart   = "command cannot start"
	ClassNotObserved   = "calls not observed"
	ClassNoHello       = "no hello observed"
)

// The shell failure classes, by what reaching the state met: the CLI's,
// which writes them (receipt.ShellClass).
const (
	ShellReadOnly   = receipt.ShellReadOnly
	ShellNotAllowed = receipt.ShellNotAllowed
	ShellIOError    = receipt.ShellIOError
)

// The hello timer's bounds: chosen with room over probe 2's cold starts, not
// measured norms.
const (
	HelloAtStart = 15 * time.Second
	HelloAtCall  = 10 * time.Second
)

// Harness says how the run's harness starts its servers, which decides the
// timer and what a lost connection means.
type Harness string

// The harnesses the channel knows.
const (
	Codex  Harness = "codex"
	Claude Harness = "claude"
)

// Stamp is an event's time: the boot clock orders, the wall clock is shown.
type Stamp struct {
	Boot int64     `json:"boot"`
	Wall time.Time `json:"wall"`
}

// Shell is the newest shell observation.
type Shell struct {
	OK    bool   `json:"ok"`
	Class string `json:"class,omitempty"`
	At    Stamp  `json:"at"`
}

// Record is the channel record of a run. The state shown is derived from it
// (Display) and never stored apart.
type Record struct {
	Harness Harness `json:"harness"`
	Tool    string  `json:"tool"`
	// Class is the failure class shown while an interval is open, the latest
	// failure's by event time, which ClassAt keeps; Reason, for no tool, is
	// why, in the words a launch note allows.
	Class   string `json:"class,omitempty"`
	ClassAt Stamp  `json:"class_at"`
	Reason  string `json:"reason,omitempty"`
	// Interval is when the open failure interval began; zero while none.
	Interval Stamp `json:"interval"`
	// Worked is the last validated ticket; Reconnected, a hello during an
	// open interval.
	Worked      Stamp `json:"worked"`
	Reconnected Stamp `json:"reconnected"`
	// Live are the generations of the live connections; Generation the
	// newest hello's.
	Live       []uint64 `json:"live,omitempty"`
	Generation uint64   `json:"generation,omitempty"`
	// Timer is when the hello timer passes; zero while none runs.
	Timer int64 `json:"timer,omitempty"`
	// Conversation is the thread the gateway last selected on Codex, whose
	// connections the tool observation counts; "" before the first.
	Conversation string `json:"conversation,omitempty"`
	Shell        *Shell `json:"shell,omitempty"`
	// Block is the latest denial's time while the block is set; zero while
	// none. Issued is the newest validated ticket's issue time: a denial
	// before it was lifted by it, whenever either is folded.
	Block  Stamp `json:"block"`
	Issued int64 `json:"issued,omitempty"`
	// Frozen: the harness exited or its turn-end shutdown began.
	Frozen bool `json:"frozen,omitempty"`

	// h is what the tool observation is derived from (history.go); only
	// folding needs it.
	h history
}

// Open says whether a failure interval is open.
func (r *Record) Open() bool { return r.Interval.Boot != 0 }

// Blocked says whether the policy block is set.
func (r *Record) Blocked() bool { return r.Block.Boot != 0 }

// Working says whether mail goes through the tool: the CLI writes a shell
// observation only while it does not.
func (r *Record) Working() bool {
	return r.Tool == ToolWorking && !r.Open() && !r.Blocked()
}

// New is the record at launch: the tool injected, or not with its reason,
// whose interval opens at the launch.
func New(harness Harness, injected bool, reason string, at Stamp) Record {
	if injected {
		return Record{Harness: harness, Tool: ToolStarting}
	}
	return Record{Harness: harness, Tool: ToolNone, Reason: reason, Interval: at}
}

func (r *Record) live(generation uint64) bool { return slices.Contains(r.Live, generation) }

// View is a copy of the record for a reader: what it shows, without the
// history it is derived from.
func (r Record) View() Record {
	r.Live = slices.Clone(r.Live)
	if r.Shell != nil {
		shell := *r.Shell
		r.Shell = &shell
	}
	r.h = history{}
	return r
}
