/*
Package bridge holds what the CLI and the mail tool's server agree on
(docs/mail-bridge.md): the ticket that says which native call a CLI run answers,
the bounds of one tool result, and how a text is cut into parts that fit them.

The server runs the CLI once per tool call with the call's words. The CLI stays
the one implementation of the mail; this package only carries the terms both
sides must read the same way, so neither can drift from the other.
*/
package bridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"syscall"
	"time"
)

// TicketEnv names the variable holding the number of the inherited descriptor
// that carries the call's ticket. Its presence is what puts the CLI in bridge
// mode: the words then come from a tool call, not from a shell.
const TicketEnv = "REWAKE_BRIDGE_TICKET_FD"

// CapabilityEnv names the variable that hands the server the per-launch
// secret it presents to the wrapper's endpoint. The launch sets it in the
// server's own environment, never the harness's.
const CapabilityEnv = "REWAKE_BRIDGE_CAPABILITY"

// The bounds of one tool result, measured on both harnesses (probe 2 in
// docs/mail-bridge.md): a direct result arrived whole at about 47,000 bytes on
// one and 60,000 on the other, so 4 KiB leaves more than tenfold margin.
const (
	// BodyCap is the most letter or output text one result carries.
	BodyCap = 2048
	// ResultCap bounds the whole encoded result: text, escapes, stderr,
	// receipt and continuation. Whichever of the two binds first wins.
	ResultCap = 4096
	// resultOverhead is kept free for the server's own framing of a result
	// around stdout and stderr: the exit code, field names, brackets.
	resultOverhead = 256
	// MaxWords and MaxWordsBytes bound a call before anything else is read:
	// the longest allowed form is send with a name, a text and three flags.
	MaxWords      = 12
	MaxWordsBytes = 32 << 10
	// ticketMax bounds what is read from the ticket descriptor.
	ticketMax = 4096
)

// Ticket is what the wrapper issues for one native tool call and the server
// hands the CLI through an inherited descriptor. The CLI takes nothing it
// carries as authority until the wrapper has confirmed it (Validator).
type Ticket struct {
	// Capability is the per-launch secret the wrapper checks.
	Capability string `json:"capability"`
	// Conversation and Turn are the harness's own ids of the calling
	// conversation and turn; they scope the receipt.
	Conversation string `json:"conversation"`
	Turn         string `json:"turn"`
	// CallID is the native id of the tool call. It correlates observations;
	// it is not the retry key, since a retry gets a new one.
	CallID string `json:"callId"`
	// CalledBoot is when the call was made, on the boot clock.
	CalledBoot int64 `json:"calledBoot"`
	// DeadlineBoot is when the call's result stops being wanted.
	DeadlineBoot int64 `json:"deadlineBoot"`
	// WordsDigest is Digest of the normalized words the call carried.
	WordsDigest string `json:"wordsDigest"`
	// Transport names the harness path, such as "codex-mcp".
	Transport string `json:"transport"`
	// Nonce makes the ticket one-time: the wrapper confirms it once, for
	// one process, and never again.
	Nonce string `json:"nonce"`
	// ReadsOff, when set, is why this run's tool may not read the mail:
	// what it would show could reach the conversation other than direct
	// and whole (docs/mail-bridge-launch-codex.md#the-output-limit).
	ReadsOff string `json:"readsOff,omitempty"`
}

// The transports of the mail tool, as tickets and receipts name them.
const (
	CodexTransport  = "codex-mcp"
	ClaudeTransport = "claude-mcp"
)

// CallKey names one native call within its run: the hex SHA-256 of the
// transport, the conversation and the call id. It is the name of the call's
// binding (docs/mail-bridge-server.md#running-the-child), which the server
// and the wrapper both derive from the ticket.
func CallKey(transport, conversation, callID string) string {
	sum := sha256.Sum256([]byte(transport + "\x00" + conversation + "\x00" + callID))
	return hex.EncodeToString(sum[:])
}

// Validator asks the wrapper of the run whether it issued this ticket. The
// wrapper's context endpoint answers; until one is reachable nothing in
// bridge mode is authorized.
type Validator func(dir, name, epoch string, ticket Ticket) error

// ErrNoEndpoint is the answer while no wrapper endpoint validates tickets.
var ErrNoEndpoint = errors.New("the wrapper's context endpoint does not answer, so no tool call can be authorized")

// NoEndpoint is the validator of a build whose wrapper offers no endpoint yet:
// it refuses every ticket, so an invented one authorizes nothing.
func NoEndpoint(string, string, string, Ticket) error { return ErrNoEndpoint }

// Active says whether this process runs a tool call.
func Active() bool { return os.Getenv(TicketEnv) != "" }

// ReadTicket reads the ticket from the descriptor TicketEnv names. The
// descriptor is read once, to its end, within a second: a writer that never
// closes it must not hold the call.
func ReadTicket() (Ticket, error) {
	raw := os.Getenv(TicketEnv)
	fd, err := strconv.Atoi(raw)
	if err != nil || fd < 3 {
		return Ticket{}, fmt.Errorf("%s holds %q, not an inherited descriptor", TicketEnv, raw)
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		return Ticket{}, fmt.Errorf("the ticket descriptor %d is not open: %w", fd, err)
	}
	file := os.NewFile(uintptr(fd), "ticket")
	defer func() { _ = file.Close() }()
	_ = file.SetReadDeadline(time.Now().Add(time.Second))
	data, err := io.ReadAll(io.LimitReader(file, ticketMax+1))
	if err != nil {
		return Ticket{}, fmt.Errorf("could not read the ticket: %w", err)
	}
	return ParseTicket(data)
}

// ParseTicket reads a ticket's bytes and refuses one missing what scopes it.
func ParseTicket(data []byte) (Ticket, error) {
	if len(data) > ticketMax {
		return Ticket{}, errors.New("the ticket is longer than a ticket can be")
	}
	var ticket Ticket
	if err := json.Unmarshal(data, &ticket); err != nil {
		return Ticket{}, fmt.Errorf("the ticket is not readable: %w", err)
	}
	switch {
	case ticket.Conversation == "", ticket.Turn == "":
		return Ticket{}, errors.New("the ticket names no conversation or turn, so the call cannot be scoped")
	case ticket.CallID == "":
		return Ticket{}, errors.New("the ticket names no native call")
	case ticket.WordsDigest == "":
		return Ticket{}, errors.New("the ticket carries no digest of the words")
	case ticket.Nonce == "":
		return Ticket{}, errors.New("the ticket carries no nonce, so it could be used twice")
	case ticket.CalledBoot <= 0 || ticket.DeadlineBoot <= ticket.CalledBoot:
		return Ticket{}, errors.New("the ticket's call time or deadline is missing")
	}
	return ticket, nil
}

// Digest identifies a call's normalized words: the same words, the same
// digest, whichever side computes it.
func Digest(words []string) string {
	encoded, _ := json.Marshal(words)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// Exposure is what the wrapper observed of one tool result reaching the model,
// the evidence a read part needs before it counts as shown.
type Exposure struct {
	// CallID is the native call the result answered.
	CallID string
	// Direct says the model called the tool itself, not from inside a script
	// whose own output nobody bounded.
	Direct bool
	// Succeeded says the harness took the result as a success.
	Succeeded bool
	// ResultBytes is the size of the result as it was handed on.
	ResultBytes int
	// Shortened says the harness cut, persisted or previewed the result.
	Shortened bool
	// Answer is the first content item of the result the harness recorded,
	// decoded from its JSON string and nothing else; nil when that item is
	// not text. A part counts as shown only when its digest is the answer
	// the call recorded before printing it (AnswerDigest).
	Answer []byte
}

// AnswerDigest names the whole text a read call prints: the hex SHA-256 of
// its stdout, which the call records with each part before printing it.
func AnswerDigest(text []byte) string {
	sum := sha256.Sum256(text)
	return hex.EncodeToString(sum[:])
}

// EndGate orders a read's acknowledgment against the turn ends the wrapper
// captures (docs/mail-bridge-turns.md#a-turns-end-meets-its-calls). The
// acknowledgment calls Enter holding the mailbox lock, before its first
// write: false says an end at or after calledBoot was noted, and nothing may
// be written. Otherwise it is registered as writing, and calls leave once,
// whatever its writes did, before it releases the mailbox lock: leave takes
// the read clock's snapshot that an end captured meanwhile takes as its
// boundary.
type EndGate interface {
	Enter(calledBoot int64) (leave func(), ok bool)
}

// Whole says whether the evidence proves the model got the entire result:
// a direct, successful call whose result is inside the calibrated bound and
// was not shortened. Anything less acknowledges nothing.
func (e Exposure) Whole() bool {
	return e.CallID != "" && e.Direct && e.Succeeded && !e.Shortened && e.ResultBytes > 0 && e.ResultBytes <= ResultCap
}
