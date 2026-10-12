package endpoint

import (
	"time"

	"github.com/praline-labs/rewake/internal/core/channel"
	"github.com/praline-labs/rewake/internal/tool"
)

// Config is what the wrapper of one run gives its endpoint.
type Config struct {
	// Dir, Name and Epoch name the run.
	Dir, Name, Epoch string
	// Transport is the harness path of this run's tool calls.
	Transport string
	// Capability is the per-launch secret a child presents from its ticket.
	Capability string
	// Span is how long after it is issued a ticket's answer is wanted.
	// Refusal, when set, is why this run's tool reads nothing: a deadline
	// too short to run a call in (DeadlineFor).
	Span    time.Duration
	Refusal string
	// Words normalizes the words a harness observed: the CLI's own check.
	Words func([]string) ([]string, error)
	// Check is the same check for a call a transport asks to run: the
	// normalized words, or the CLI's own refusal as a shell prints it.
	Check func([]string) ([]string, string)
	// Tools are the tools a transport offers, from the command table; a
	// call names one of them.
	Tools []tool.ToolDescriptor
	// Executable is the image a transport's call runs in, the wrapper's own
	// unless a test names another.
	Executable string
	// Acknowledge acknowledges a read's parts on the evidence of a result;
	// the CLI's AcknowledgeRead.
	Acknowledge func(dir, name, epoch, token string, evidence tool.Exposure, gate tool.EndGate) error
	// Gate orders acknowledgments against this run's captured ends.
	Gate *tool.Gate
	// Channel takes the channel events the endpoint observes; nil drops
	// them.
	Channel func(channel.Event)

	// Wait bounds a ticket request's wait for its observation; zero is the
	// two seconds of the rules. Tests shorten it.
	Wait time.Duration
	// SameBuild and Descends check a peer; nil takes the real checks.
	// Tests, whose server is another binary than the test, replace them.
	SameBuild func(pid int) error
	Descends  func(pid, ancestor int) error
}
