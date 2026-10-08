package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// testTransport is a transport of no harness: what a ticket's transport means
// to the CLI is only what its binding declares.
const testTransport = "test-transport"

// toolCaller runs words the way the mail tool's server will: the ticket on an
// inherited descriptor, the variable naming it set, and the wrapper's
// confirmation stood in for, since its endpoint comes with the server.
type toolCaller struct {
	t            *testing.T
	conversation string
	turn         string
	calls        int
	// forge, when set, is what the ticket's digest is taken over instead of
	// the words the call carries.
	forge []string
	// late shifts the call's deadline into the past.
	late bool
	// reused withholds the transport's declaration that its turn ids are
	// never reused; by default the tickets carry it.
	reused bool
	// tickets are the tickets the stand-in confirmed, in order.
	tickets []bridge.Ticket
}

func newToolCaller(t *testing.T) *toolCaller {
	t.Helper()
	caller := &toolCaller{t: t, conversation: "conversation-1", turn: "turn-1"}
	previous := validateTicket
	validateTicket = func(_, _, _ string, ticket bridge.Ticket) error {
		caller.tickets = append(caller.tickets, ticket)
		return nil
	}
	t.Cleanup(func() { validateTicket = previous })
	return caller
}

// toolRun is one call's answer and its native id.
type toolRun struct {
	code   int
	out    string
	errOut string
	callID string
}

func (c *toolCaller) run(words ...string) toolRun {
	c.t.Helper()
	c.calls++
	callID := fmt.Sprintf("call-%d", c.calls)
	digested := c.forge
	if digested == nil {
		digested = words
	}
	digest := "refused"
	if normal, err := ToolWords(digested); err == nil {
		digest = bridge.Digest(normal)
	}
	now := boottime.Now()
	ticket := bridge.Ticket{
		Capability: "capability", Conversation: c.conversation, Turn: c.turn, CallID: callID,
		CalledBoot: now, DeadlineBoot: now + int64(30*time.Second), WordsDigest: digest, Transport: testTransport, Nonce: "nonce-" + callID,
		TurnsNeverReused: !c.reused,
	}
	if c.late {
		ticket.CalledBoot, ticket.DeadlineBoot = now-int64(2*time.Second), now-int64(time.Second)
	}
	encoded, _ := json.Marshal(ticket)
	var fds [2]int
	if err := syscall.Pipe(fds[:]); err != nil {
		c.t.Fatal(err)
	}
	if _, err := syscall.Write(fds[1], encoded); err != nil {
		c.t.Fatal(err)
	}
	_ = syscall.Close(fds[1])
	c.t.Setenv(bridge.TicketEnv, strconv.Itoa(fds[0]))
	defer func() { _ = os.Unsetenv(bridge.TicketEnv) }()
	code, out, errOut := run(words...)
	return toolRun{code: code, out: out, errOut: errOut, callID: callID}
}

// toolSession is api, the session the tool runs for, with web as a live peer.
func toolSession(t *testing.T) (string, registry.Session, registry.Session) {
	t.Helper()
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	self, err := registry.Lookup(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(epochEnv, self.Epoch())
	return dir, self, web
}

var nextLine = regexp.MustCompile(`Rewake: (?:the next|part \d+ of \d+ of this output; the next): rewake (inbox --next \S+)`)

// nextFrom is the words a human-form answer names for its continuation.
func nextFrom(out string) []string {
	found := nextLine.FindStringSubmatch(out)
	if found == nil {
		return nil
	}
	return strings.Fields(found[1])
}

func unreadIDs(t *testing.T, dir string, self registry.Session) map[string]bool {
	t.Helper()
	messages, err := inbox.PeekUnread(dir, self.Name, self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, message := range messages {
		ids[message.ID] = true
	}
	return ids
}

// The tool runs the mail and nothing else, and two spellings of one call are
// one call to the journal.
func TestTheToolSurface(t *testing.T) {
	for _, words := range [][]string{
		{"inbox"},
		{"inbox", "--json"},
		{"inbox", "--peek"},
		{"inbox", "--message", "1780000000000000000-012345abcdef"},
		{"inbox", "--owed"},
		{"inbox", "--awaited"},
		{"inbox", "--next", "0123456789abcdef01234567.0.1"},
		{"pending", "the suite runs"},
		{"send", "web", "look", "--notify"},
		{"send", "web", "look", "--notify", "--wait", "5"},
		{"whoami"},
		{"whoami", "--json"},
		{"retry", "0123456789abcdef01234567"},
		{"inbox", "--help"},
		{"send", "--help"},
	} {
		if _, err := ToolWords(words); err != nil {
			t.Errorf("%q refused: %v", words, err)
		}
	}
	for _, words := range [][]string{
		{},
		{"list"},
		{"claude"},
		{"--version"},
		{"withdraw", "x"},
		{"send", "web", "look"},
		{"send", "web", "look", "--question"},
		{"send", "web", "-", "--notify"},
		{"send", "web", " ", "--notify"},
		{"send", "web", "look", "--notify", "--wait", "6"},
		{"send", "web", "look", "--notify", "--to", "x"},
		{"inbox", "--peek", "--owed"},
		{"inbox", "--next", "a/b"},
		{"pending"},
		{"pending", ""},
		{"retry"},
		{"inbox", "--peek=yes"},
		{"whoami", "extra"},
		strings.Fields(strings.Repeat("inbox ", bridge.MaxWords+1)),
	} {
		if _, err := ToolWords(words); err == nil {
			t.Errorf("%q allowed", words)
		}
	}
	first, _ := ToolWords([]string{"send", "web", "look", "--notify", "--wait", "2"})
	second, _ := ToolWords([]string{"send", "--wait=2", "--notify", "web", "look"})
	if strings.Join(first, "\x00") != strings.Join(second, "\x00") {
		t.Errorf("one call normalizes as %q and %q", first, second)
	}
	other, _ := ToolWords([]string{"send", "web", "look again", "--notify", "--wait", "2"})
	if bridge.Digest(first) == bridge.Digest(other) {
		t.Error("two texts digest alike")
	}
}

// Nothing runs under the tool that the wrapper did not confirm, that was
// issued for other words, or whose answer is no longer wanted; the refused
// read leaves the letter as it was.
func TestAToolCallWithoutAValidTicketRunsNothing(t *testing.T) {
	dir, self, web := toolSession(t)
	id := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "text": "rerun"})
	tool := newToolCaller(t)

	validateTicket = bridge.NoEndpoint
	if answer := tool.run("inbox"); answer.code != ExitFailed || answer.out != "" || !strings.Contains(answer.errOut, "did not confirm") {
		t.Fatalf("with no endpoint: %+v", answer)
	}
	validateTicket = func(string, string, string, bridge.Ticket) error { return nil }
	tool.forge = []string{"inbox", "--peek"}
	if answer := tool.run("inbox"); answer.code != ExitFailed || !strings.Contains(answer.errOut, "other words") {
		t.Fatalf("for other words: %+v", answer)
	}
	tool.forge, tool.late = nil, true
	if answer := tool.run("inbox"); answer.code != ExitFailed || !strings.Contains(answer.errOut, "deadline") {
		t.Fatalf("past its deadline: %+v", answer)
	}
	tool.late = false
	if answer := tool.run("list"); answer.code != ExitUsage || !strings.Contains(answer.errOut, "does not run through the rewake tool") {
		t.Fatalf("list through the tool: %+v", answer)
	}
	if !unreadIDs(t, dir, self)[id] {
		t.Fatal("a refused call touched the letter")
	}
	if _, claimed := inbox.ReadClaimed(dir, "api", id); claimed {
		t.Fatal("a refused call claimed the letter")
	}
}

// whoami says which channel the call came through.
func TestWhoamiNamesTheMailChannel(t *testing.T) {
	toolSession(t)
	tool := newToolCaller(t)
	channel := func(out string) string {
		var model whoamiModel
		_ = json.Unmarshal([]byte(out), &model)
		return model.MailChannel
	}
	if answer := tool.run("whoami", "--json"); answer.code != ExitOK || channel(answer.out) != "tool" {
		t.Fatalf("through the tool: %+v", answer)
	}
	if answer := tool.run("whoami"); !strings.Contains(answer.out, "came through the rewake tool") {
		t.Fatalf("through the tool, as text: %+v", answer)
	}
	if _, out, _ := run("whoami", "--json"); channel(out) != "shell" {
		t.Fatalf("from the shell: %s", out)
	}
	if _, out, _ := run("whoami"); strings.Contains(out, "rewake tool") {
		t.Fatalf("the shell's whoami speaks of the tool: %s", out)
	}
}

// An answer longer than one result is kept and handed out in parts, each
// inside the bound and each a whole envelope, that a shell can continue as
// well as the tool; put together they are the answer.
func TestALongAnswerComesInParts(t *testing.T) {
	toolSession(t)
	tool := newToolCaller(t)
	full := formatCommandHelp(lookupCommand(t, "send"), true)
	answer := tool.run("send", "--help", "--json")
	var text strings.Builder
	parts := 0
	for ; ; parts++ {
		if answer.code != ExitOK || !bridge.Fits(answer.out, answer.errOut) {
			t.Fatalf("part %d: %d, %d bytes: %s", parts, answer.code, bridge.EncodedSize(answer.out, answer.errOut), answer.errOut)
		}
		var part outputPartModel
		if err := json.Unmarshal([]byte(answer.out), &part); err != nil {
			t.Fatalf("part %d is no whole envelope: %v: %s", parts, err, answer.out)
		}
		text.WriteString(part.Text)
		if part.NextWords == nil {
			break
		}
		if parts == 1 {
			// The shell picks up where the tool left off.
			code, out, errOut := run(part.NextWords...)
			answer = toolRun{code: code, out: out, errOut: errOut}
			continue
		}
		answer = tool.run(part.NextWords...)
	}
	if parts < 2 || strings.TrimRight(text.String(), "\n") != strings.TrimRight(full, "\n") {
		t.Fatalf("%d-byte help came in %d parts as %d bytes", len(full), parts+1, text.Len())
	}
	// The human form names its continuation too.
	if answer := tool.run("send", "--help"); nextFrom(answer.out) == nil {
		t.Fatalf("the text form names no next part: %s", answer.out)
	}
}

func lookupCommand(t *testing.T, name string) *Command {
	t.Helper()
	for _, group := range Groups() {
		for _, command := range group.Commands {
			if command.Name == name {
				return command
			}
		}
	}
	t.Fatalf("no command %s", name)
	return nil
}
