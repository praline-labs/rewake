package modhost

import (
	"time"

	asking "github.com/praline-labs/rewake/internal/core/ask"
	"github.com/praline-labs/rewake/internal/core/mirror"
)

// Config is what a run's mod endpoint serves.
type Config struct {
	// Socket is the run's mod socket, in the room's sock/ directory.
	Socket     string
	ControlDir string
	Epoch      string
	// Role is the run's role, as the role catalog names it, which a hello
	// is answered with.
	Role string
	// Record is where the last admitted hello is written for the run, empty
	// for nowhere: what a person or a live check reads of the mod's load.
	Record string
	// Wait bounds how long a request waits for the host to name the
	// harness's process; a request that comes before is held, then refused.
	Wait time.Duration
	// Live hears each transition of what the mod proves: a hello that opens
	// a generation, once it is bound and recorded and before it is
	// answered, so the set is live by the time the mod acts on the answer;
	// and a lapse. Each carries the next number of the run's one sequence,
	// so a report heard after a later one is known for what it is
	// (alive.go); nil hears nothing.
	Live func(transition uint64, proven bool)
	// Hold and Lapse are the heartbeat's bounds, the protocol's
	// aliveHoldSeconds and aliveLapseSeconds when zero.
	Hold, Lapse time.Duration
	// Ended says a run, by its epoch, is proven ended — of another boot, or
	// its wrapper gone — for mod/ended; nil proves none ended.
	Ended func(epoch string) bool
	// Asked tells main of a worker's request the mod opened, by its id, what
	// it shows and its deadline (asks.go); nil for a run main does not
	// answer for, whose requests stay the person's at the screen.
	Asked func(id string, offer asking.Offer, deadline time.Time) error
	// Dialog tells main once that a worker waits at a dialog on its screen
	// that no request carries, with what the harness said of it; nil for a
	// run main does not answer for.
	Dialog func(message string) error
	// Effort is the reasoning effort the launch passed, empty when it passed
	// none: shown, marked, until the mod sees a request of the main loop.
	Effort string
	// Window is the context window the launch passed, in tokens, 0 when it
	// passed none or auto: the model's own window is then the limit.
	Window int64
	// Closing hears the mod say the harness has begun to end its process
	// (closing.go); nil hears nothing.
	Closing func()
	// Mirror is the model of the conversation rewake's own face draws, fed
	// by mirror/events (mirror.go); nil for a run the face does not draw,
	// whose load is never asked for its conversation.
	Mirror *mirror.Model
}
