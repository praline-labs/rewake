package workflow

// The shim that plays a Claude Code session.
//
// It is the other column's fixture, and it is a different animal from the
// Codex one. There is no protocol here and nothing to answer: the wrapper
// dials a unix socket, writes one line, and closes. The session learns that
// mail is waiting and nothing else — no turn, no acknowledgement, no
// conversation — which is why several observations that Codex supports are
// declared absent on this column rather than quietly dropped.
//
// What it must be strict about is the one thing it does receive. A fixture
// that accepted any line would let a scenario pass while the envelope was
// mangled on the way, so the envelope is checked field by field and anything
// else is refused. Stricter than the real harness is allowed and intended;
// looser is not.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// runClaudeShim is the entry point when the test binary is re-executed as
// `claude`. It parses what the wrapper passed, listens where it was told, and
// then behaves as a session does: reads its mail when told there is some, and
// reports the end of each turn through the hook the wrapper configured.
func runClaudeShim(args []string) int {
	launch, err := parseClaudeLaunch(harnessArgs(args))
	if err != nil {
		fmt.Fprintf(os.Stderr, "claude-shim: %v\n", err)
		// Written where the case keeps its evidence, because a session's own
		// output is not kept: a refused launch otherwise leaves a wrapper that
		// exited 2 and nothing saying why.
		if mark := os.Getenv(shimReadyFile); mark != "" {
			_ = os.WriteFile(mark+".refused", []byte(err.Error()+"\n"+strings.Join(args, "\n")+"\n"), 0o600)
		}
		return 2
	}
	if err := insideACase(); err != nil {
		fmt.Fprintf(os.Stderr, "claude-shim: %v\n", err)
		return 1
	}
	listener, err := net.Listen("unix", launch.socket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "claude-shim: listen %s: %v\n", launch.socket, err)
		return 3
	}
	defer func() { _ = listener.Close() }()
	// The socket exists now. A sender on this column waits for this file
	// rather than for telemetry, which this harness does not have: mail sent
	// before the socket exists waits in the mailbox and is folded into the
	// first delivery, which would make a scenario about the collection window
	// measure the launch instead.
	if mark := os.Getenv(shimReadyFile); mark != "" {
		_ = os.WriteFile(mark, []byte(launch.socket), 0o600)
	}

	session := &claudeSession{launch: launch}
	go session.serve(listener)
	if os.Getenv(shimTelemetryFile) != "" {
		go session.playTelemetry()
	}
	// A sender sends with its own rewake here too, and it waits for its
	// recipient first — so it runs beside the listener rather than before it:
	// a session that sent before it could receive would be unable to answer
	// the report it is about to be sent.
	go sendAsAsked()
	return session.waitToBeStopped()
}

// harnessArgs are the arguments the wrapper gave the harness: everything after
// the "--" the stand-in script puts between the test binary's own flags and
// them. Without that cut the strict parser read the test binary's path as an
// unknown option and refused every launch — which is how it was found.
func harnessArgs(args []string) []string {
	for index, arg := range args {
		if arg == "--" {
			return args[index+1:]
		}
	}
	// No separator means this was not started by the stand-in script, and an
	// empty list is then refused for its missing socket rather than guessed at.
	return nil
}

// claudeLaunch is what the wrapper passed, read back. Every field of it is
// something the adapter promises to send, so a missing one is a defect in the
// adapter rather than a shape this fixture may improvise around.
type claudeLaunch struct {
	socket   string
	intro    string
	settings claudeSettings
	tools    string
}

// claudeLaunchFlags are the flags rewake passes to this harness, each with a
// value. Nothing else is accepted: the real Claude Code refuses a flag it
// does not know ("claude -m x" answers "unknown option '-m'", see
// docs/research-launch.md), so a fixture that let one through would be looser
// than the harness and would pass a launch the harness would refuse. The model
// and the effort are here because rewake adds them when a launch default is
// configured; the isolation of a case sets none, and a launch that carried
// them anyway would still be one rewake can produce.
var claudeLaunchFlags = map[string]bool{
	"--messaging-socket-path": true,
	"--append-system-prompt":  true,
	"--settings":              true,
	"--allowedTools":          true,
	"--model":                 true,
	"--effort":                true,
}

func parseClaudeLaunch(args []string) (claudeLaunch, error) {
	var launch claudeLaunch
	var settings string
	seen := map[string]bool{}
	for index := 0; index < len(args); index++ {
		flag := args[index]
		if !claudeLaunchFlags[flag] {
			// Positional words too: rewake passes none in a case, and one here
			// would mean the launch carried a prompt nobody asked for.
			return launch, fmt.Errorf("unknown option %q; rewake does not pass it", flag)
		}
		if seen[flag] {
			return launch, fmt.Errorf("%s given twice; rewake passes it once", flag)
		}
		seen[flag] = true
		if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
			return launch, fmt.Errorf("%s without a value", flag)
		}
		index++
		switch flag {
		case "--messaging-socket-path":
			launch.socket = args[index]
		case "--append-system-prompt":
			launch.intro = args[index]
		case "--settings":
			settings = args[index]
		case "--allowedTools":
			launch.tools = args[index]
		}
	}
	if launch.socket == "" {
		return launch, errors.New("no --messaging-socket-path; the wrapper always names one")
	}
	if launch.intro == "" {
		return launch, errors.New("no --append-system-prompt; a session launched without its briefing is not one")
	}
	if launch.tools == "" {
		return launch, errors.New("no --allowedTools; without it a session cannot answer without a person")
	}
	parsed, err := parseClaudeSettings(settings)
	if err != nil {
		return launch, err
	}
	launch.settings = parsed
	return launch, nil
}

// claudeSession is the fixture's session: what it was launched with, and what
// it has seen.
type claudeSession struct {
	launch claudeLaunch
	mu     sync.Mutex
	// deliveries counts the notifications that arrived, which is this
	// column's equivalent of a turn number — the socket has no turns.
	deliveries int
}

func (s *claudeSession) serve(listener net.Listener) {
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		s.handle(connection)
	}
}

// handle reads the one line a delivery is, checks it, and works a turn on it.
//
// One line and then closed, the way the adapter writes it. A connection that
// carries more than one line is refused rather than drained: the adapter sends
// one, and a fixture that accepted a stream would be modeling a protocol
// nobody has.
func (s *claudeSession) handle(connection net.Conn) {
	defer func() { _ = connection.Close() }()
	_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(connection)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintf(os.Stderr, "claude-shim: reading a delivery: %v\n", err)
		return
	}
	notice, err := checkedEnvelope([]byte(line))
	if err != nil {
		fmt.Fprintf(os.Stderr, "claude-shim: %v\n", err)
		return
	}
	if rest, _ := reader.ReadString('\n'); strings.TrimSpace(rest) != "" {
		fmt.Fprintf(os.Stderr, "claude-shim: a delivery carried more than one line\n")
		return
	}
	s.mu.Lock()
	s.deliveries++
	turn := fmt.Sprintf("delivery-%d", s.deliveries)
	s.mu.Unlock()
	s.workTurn(turn, notice)
}

// envelopeShape is what one line of the session inbox protocol may look like.
// The table is closed at every depth, so a field nobody agreed to is refused
// by name rather than ignored — the same rule the other column's fixture
// follows, and for the same reason.
var envelopeShape = served{kind: "object", fields: map[string]served{
	"type": {kind: "string"},
	"message": {kind: "object", fields: map[string]served{
		"role":    {kind: "string"},
		"content": {kind: "string"},
	}, required: []string{"role", "content"}},
	"priority": {kind: "string"},
}, required: []string{"type", "message", "priority"}}

// claudeNotice is the notification as it arrives: the tagged block the adapter
// builds. Its fields are what a session on this column can know about the mail
// it was told about — an id for the announcement, a status, a count and a
// preview. The member ids are deliberately not among them, and that absence is
// what several observations on this column rest on.
type claudeNotice struct {
	TaskID  string
	Status  string
	Summary string
	Count   int
	Preview string
}

// checkedEnvelope answers what a delivery must look like on this column, and
// refuses everything else. The table is closed: a field the adapter does not
// send is a field this fixture does not accept, which is the check that would
// have caught an envelope growing a shape nobody agreed to.
func checkedEnvelope(line []byte) (claudeNotice, error) {
	var notice claudeNotice
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return notice, errors.New("an empty delivery")
	}
	if wrong := singleNames("the delivery", json.RawMessage(trimmed)); wrong != "" {
		return notice, errors.New(wrong)
	}
	if wrong := unserved("the delivery", json.RawMessage(trimmed), envelopeShape); wrong != "" {
		return notice, errors.New(wrong)
	}
	var envelope struct {
		Type    string `json:"type"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Priority string `json:"priority"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return notice, fmt.Errorf("unreadable delivery: %w", err)
	}
	if envelope.Type != "user" || envelope.Message.Role != "user" {
		return notice, fmt.Errorf("a delivery addressed as %q/%q, not as a user message", envelope.Type, envelope.Message.Role)
	}
	if envelope.Priority != "next" {
		// "next" is what puts the message after the tool call in flight and
		// wakes an idle session; anything else would be a different promise.
		return notice, fmt.Errorf("a delivery with priority %q", envelope.Priority)
	}
	return parseNotification(envelope.Message.Content)
}

// parseNotification reads the tagged block the adapter builds. A session sees
// exactly this text, so what the scenario may observe on this column is
// exactly what can be read out of it.
func parseNotification(content string) (claudeNotice, error) {
	var notice claudeNotice
	notice.TaskID = between(content, "<task-id>", "</task-id>")
	notice.Status = between(content, "<status>", "</status>")
	notice.Summary = strings.TrimSpace(unescapeNotice(between(content, "<summary>", "</summary>")))
	if notice.TaskID == "" || notice.Status == "" || notice.Summary == "" {
		return notice, fmt.Errorf("a notification without an id, a status or a summary: %s", firstLine(content))
	}
	first, preview, _ := strings.Cut(notice.Summary, "\n")
	notice.Preview = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(preview), "↳"))
	count, err := announcedCount(first)
	if err != nil {
		return notice, err
	}
	notice.Count = count
	return notice, nil
}

// announcedCount reads how many messages the notice says are waiting. It is
// the only thing this column knows about the membership of a group: the
// adapter names the group, not its members.
func announcedCount(line string) (int, error) {
	fields := strings.Fields(line)
	for index, field := range fields {
		if index+1 < len(fields) && strings.HasPrefix(fields[index+1], "new") {
			count, err := strconv.Atoi(field)
			if err == nil {
				return count, nil
			}
		}
	}
	return 0, fmt.Errorf("a notice that does not say how many messages are waiting: %q", line)
}

func between(text, opening, closing string) string {
	_, rest, found := strings.Cut(text, opening)
	if !found {
		return ""
	}
	inner, _, found := strings.Cut(rest, closing)
	if !found {
		return ""
	}
	return inner
}

// unescapeNotice undoes what the adapter escapes so a sender name cannot close
// a tag early.
func unescapeNotice(text string) string {
	return strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&").Replace(text)
}

// waitToBeStopped keeps the session alive the way the Codex client half does:
// until it is asked to stop, or until its own ceiling.
func (s *claudeSession) waitToBeStopped() int {
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(os.Getenv(shimExitFile)); err == nil {
			return 0
		}
		if os.Getenv(shimExitAfterTurn) != "" && workedATurn() {
			return 0
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0
}
