package cli

import (
	"bytes"
	"errors"
	"io"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/state"
)

// Bridge mode: the CLI running one tool call (docs/mail-bridge.md). The words
// are checked against the surface again, the call's ticket is confirmed with
// the run's wrapper, and the whole answer is bounded before it leaves: a
// result the harness would cut is a result the model did not get.

// validateTicket asks the wrapper of the run whether it issued a ticket. The
// wrapper's context endpoint comes with the mail tool's server; until then
// every ticket is refused, so bridge mode authorizes nothing by itself.
var validateTicket bridge.Validator = bridge.NoEndpoint

// callScope is a validated tool call.
type callScope struct {
	ticket bridge.Ticket
	words  []string
	digest string
}

// deadline is how long the call's answer is still wanted.
func (s *callScope) remaining() time.Duration {
	return time.Duration(s.ticket.DeadlineBoot - boottime.Now())
}

// runBridge runs one tool call and prints its bounded answer.
func runBridge(argv []string, stdout, stderr io.Writer) int {
	var out, errOut bytes.Buffer
	ctx := &Context{Stdout: &out, Stderr: &errOut}
	code := bridgeCall(ctx, argv)
	text, diagnostic := boundedAnswer(ctx, out.String(), errOut.String())
	_, _ = io.WriteString(stdout, text)
	_, _ = io.WriteString(stderr, diagnostic)
	return code
}

func bridgeCall(ctx *Context, argv []string) int {
	result, err := toolCall(argv)
	if err != nil {
		return report(ctx, err)
	}
	scope, err := authorize(result)
	if err != nil {
		return report(ctx, err)
	}
	ctx.scope = scope
	ctx.JSON = result.Call.Switch("json")
	if result.Help {
		printText(ctx, formatCommandHelp(result.Call.Command, true))
		return ExitOK
	}
	if err := result.Call.Command.Handler(ctx, result.Call); err != nil {
		return report(ctx, err)
	}
	return ExitOK
}

// authorize reads the call's ticket and has the wrapper confirm it. Nothing
// the model supplied — words, ids, times — is authority on its own: the
// ticket must have been issued for exactly these words, by this run's wrapper,
// and its deadline must not have passed.
func authorize(result parsed) (*callScope, error) {
	ticket, err := bridge.ReadTicket()
	if err != nil {
		return nil, failf("Rewake: this tool call carries no usable ticket (%v), so nothing ran; run the same words in the shell: rewake <words>", err)
	}
	words := normalized(result)
	digest := bridge.Digest(words)
	if ticket.WordsDigest != digest {
		return nil, failf("Rewake: the ticket of this tool call was issued for other words, so nothing ran.")
	}
	dir, err := state.Dir()
	if err != nil {
		return nil, failf("Rewake: %v; nothing ran.", err)
	}
	self, epoch, err := ownRun(dir)
	if errors.Is(err, errUpgraded) {
		return nil, refuseUpgraded(dir, self)
	}
	if err != nil {
		return nil, failf("Rewake: %v; nothing ran.", err)
	}
	if err := validateTicket(dir, self.Name, epoch, ticket); err != nil {
		return nil, failf("Rewake: the wrapper did not confirm this tool call (%v), so nothing ran; run the same words in the shell: rewake <words>", err)
	}
	state.Step("confirmed")
	scope := &callScope{ticket: ticket, words: words, digest: digest}
	if scope.remaining() <= 0 {
		return nil, failf("Rewake: this tool call's deadline passed before it started, so nothing ran; call it again.")
	}
	return scope, nil
}

// diagnosticCap bounds the encoded size of what a refusal or failure prints
// under the tool: the full help page is a shell's to print, and --help through
// the tool gives it in parts.
const diagnosticCap = 1024

// boundedAnswer fits an answer into one result. It is the one place the cap
// is enforced, on the final encoded bytes, whatever branch produced them: a
// diagnostic is cut first, with the way to its full text; an output still too
// long is frozen and its first part printed, with the words that fetch the
// next; and whatever does not fit after that is replaced by one line that
// does.
func boundedAnswer(ctx *Context, out, errOut string) (string, string) {
	if bridge.Fits(out, errOut) {
		return out, errOut
	}
	errOut = cutDiagnostic(errOut)
	if !bridge.Fits(out, errOut) {
		first, err := freezeOutput(ctx, out)
		if err != nil {
			out, errOut = "", cutDiagnostic(errOut+"Rewake: the answer is longer than one tool result and could not be kept for continuing ("+err.Error()+"); run the same words in the shell.\n")
		} else {
			out = first
		}
	}
	if !bridge.Fits(out, errOut) {
		return "", unfitting
	}
	return out, errOut
}

// unfitting is what an answer becomes that nothing else could fit.
const unfitting = "Rewake: the answer did not fit one tool result; run the same words in the shell.\n"

// cutDiagnostic cuts a diagnostic to diagnosticCap encoded bytes, escapes
// counted: a byte of text may take six once encoded.
func cutDiagnostic(text string) string {
	const tail = "\n… cut here; the full refusal: run the same words in the shell.\n"
	if encodedString(text) <= diagnosticCap {
		return text
	}
	budget := diagnosticCap - encodedString(tail)
	cut := bridge.Cut(text, 0, func(end int) bool { return encodedString(text[:end]) <= budget })
	return text[:cut] + tail
}

// encodedString is the size of text as a JSON string.
func encodedString(text string) int {
	return bridge.EncodedSize(text, "") - bridge.EncodedSize("", "")
}
