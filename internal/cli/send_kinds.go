package cli

import (
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// messageKind is how send treats one kind of message. A kind lives in a file of
// its own and is listed in sendKinds; the flags, the refusal and the help come
// from here.
type messageKind struct {
	kind    inbox.Kind
	summary string
	// flag selects the kind. The default kind has none.
	flag Option
	// wait is how long send waits when --wait is not given.
	wait time.Duration
	// needsSession, when set, refuses a sender that is not a session, and says
	// why.
	needsSession string
	// after runs once the message is delivered or pending; without it send
	// prints the delivery result.
	after func(*Context, sent) error
}

// sent is what a kind knows once its message is on its way.
type sent struct {
	dir      string
	self     registry.Session
	epoch    string
	target   registry.Session
	model    sendModel
	deadline time.Time
}

// sendKinds lists every kind send can write, the default first.
var sendKinds = []messageKind{taskKind, questionKind, noteKind, errorKind, stoppedKind}

// chosenKind reads the kind from the flags: at most one may be given.
func chosenKind(call Call) (messageKind, error) {
	chosen := sendKinds[0]
	var flags []string
	for _, kind := range sendKinds {
		if kind.flag.Flag != "" && call.Switch(strings.TrimPrefix(kind.flag.Flag, "--")) {
			chosen = kind
			flags = append(flags, kind.flag.Flag)
		}
	}
	if len(flags) > 1 {
		return messageKind{}, &UsageError{
			Command: call.Command,
			Message: strings.Join(flags, " and ") + " exclude each other: a message is one kind.",
		}
	}
	return chosen, nil
}

// kindOptions are the flags that choose a kind, for the command table.
func kindOptions() []Option {
	var options []Option
	for _, kind := range sendKinds {
		if kind.flag.Flag != "" {
			options = append(options, kind.flag)
		}
	}
	return options
}

func kindSummary() string {
	var descriptions []string
	for _, kind := range sendKinds {
		descriptions = append(descriptions, string(kind.kind)+": "+kind.summary)
	}
	return strings.Join(descriptions, " ") + " finished: a successful final report, owing no reply. Answer tasks and questions by ending the turn with a final result, not by rewake send. Do not answer notify, finished or error messages."
}
