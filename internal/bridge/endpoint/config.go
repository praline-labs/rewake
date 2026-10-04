package endpoint

import (
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/channel"
)

// Config is what the wrapper of one run gives its endpoint.
type Config struct {
	// Dir, Name and Epoch name the run.
	Dir, Name, Epoch string
	// Transport is the harness path of this run's tool calls.
	Transport string
	// Capability is the per-launch secret a server and a child present.
	Capability string
	// Span is how long after it is issued a ticket's answer is wanted.
	// Refusal, when set, is why this run's tool reads nothing: a deadline
	// too short to run a call in (DeadlineFor).
	Span    time.Duration
	Refusal string
	// Words normalizes the words a harness observed: the CLI's own check.
	Words func([]string) ([]string, error)
	// Acknowledge acknowledges a read's parts on the evidence of a result;
	// the CLI's AcknowledgeRead.
	Acknowledge func(dir, name, epoch, token string, evidence bridge.Exposure, gate bridge.EndGate) error
	// Conversation is the conversation the run's collector holds, on Claude
	// Code; "" while it has seen none.
	Conversation func() string
	// Gate orders acknowledgments against this run's captured ends.
	Gate *Gate
	// LimitsProven says gate G8 closed: the limits a Claude Code hook sees
	// at PreToolUse are the ones the harness applies to that call's result.
	// Until then no Claude Code call acknowledges a read.
	LimitsProven bool
	// OutputBound is the smallest MAX_MCP_OUTPUT_TOKENS L5 calibrated for
	// the launch's version, zero for none: a call whose limit is set
	// acknowledges a read only when it is a whole number at or above it.
	OutputBound int64
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
