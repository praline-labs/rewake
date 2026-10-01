package bridge

import (
	"encoding/json"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

func ticketBytes(t *testing.T, change func(*Ticket)) []byte {
	t.Helper()
	ticket := Ticket{Conversation: "c", Turn: "t", CallID: "call", CalledBoot: 10, DeadlineBoot: 20, WordsDigest: "d", Transport: "test"}
	change(&ticket)
	encoded, err := json.Marshal(ticket)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// A ticket that cannot scope its call authorizes nothing, whatever else it
// carries.
func TestATicketMissingItsScopeIsRefused(t *testing.T) {
	if _, err := ParseTicket(ticketBytes(t, func(*Ticket) {})); err != nil {
		t.Fatalf("a whole ticket is refused: %v", err)
	}
	for name, change := range map[string]func(*Ticket){
		"no conversation": func(ticket *Ticket) { ticket.Conversation = "" },
		"no turn":         func(ticket *Ticket) { ticket.Turn = "" },
		"no call":         func(ticket *Ticket) { ticket.CallID = "" },
		"no digest":       func(ticket *Ticket) { ticket.WordsDigest = "" },
		"no call time":    func(ticket *Ticket) { ticket.CalledBoot = 0 },
		"deadline first":  func(ticket *Ticket) { ticket.DeadlineBoot = ticket.CalledBoot },
	} {
		if _, err := ParseTicket(ticketBytes(t, change)); err == nil {
			t.Errorf("%s: the ticket is taken", name)
		}
	}
	if _, err := ParseTicket([]byte(strings.Repeat(" ", ticketMax+1))); err == nil {
		t.Error("an oversized ticket is taken")
	}
}

// The ticket comes from an inherited descriptor, never from a word: an
// environment naming stdin or no number is refused before anything is read.
func TestTheTicketIsReadFromItsDescriptor(t *testing.T) {
	reader, writer := rawPipe(t)
	if _, err := syscall.Write(writer, ticketBytes(t, func(*Ticket) {})); err != nil {
		t.Fatal(err)
	}
	_ = syscall.Close(writer)
	t.Setenv(TicketEnv, strconv.Itoa(reader))
	if !Active() {
		t.Fatal("bridge mode is not active with the variable set")
	}
	if ticket, err := ReadTicket(); err != nil || ticket.CallID != "call" {
		t.Fatalf("read %+v, %v", ticket, err)
	}
	for _, raw := range []string{"0", "x"} {
		t.Setenv(TicketEnv, raw)
		if _, err := ReadTicket(); err == nil {
			t.Errorf("%q gave a ticket", raw)
		}
	}
}

// A writer that never closes the descriptor does not hold the call.
func TestAnOpenTicketDescriptorDoesNotHang(t *testing.T) {
	reader, writer := rawPipe(t)
	defer func() { _ = syscall.Close(writer) }()
	t.Setenv(TicketEnv, strconv.Itoa(reader))
	began := time.Now()
	if _, err := ReadTicket(); err == nil {
		t.Fatal("an unfinished ticket was taken")
	}
	if waited := time.Since(began); waited > 3*time.Second {
		t.Fatalf("the read waited %s", waited)
	}
}

// rawPipe is a pipe whose descriptors no os.File holds, as an inherited
// descriptor arrives.
func rawPipe(t *testing.T) (int, int) {
	t.Helper()
	var fds [2]int
	if err := syscall.Pipe(fds[:]); err != nil {
		t.Fatal(err)
	}
	return fds[0], fds[1]
}

func TestNoEndpointRefusesEveryTicket(t *testing.T) {
	if err := NoEndpoint("dir", "name", "epoch", Ticket{}); err != ErrNoEndpoint {
		t.Fatalf("NoEndpoint = %v", err)
	}
}

// Only a direct, successful, unshortened result inside the bound is evidence
// of a whole read.
func TestOnlyAWholeResultIsEvidence(t *testing.T) {
	whole := Exposure{CallID: "call", Direct: true, Succeeded: true, ResultBytes: 900}
	if !whole.Whole() {
		t.Fatal("a whole result is not evidence")
	}
	for name, change := range map[string]func(*Exposure){
		"no call":     func(e *Exposure) { e.CallID = "" },
		"in a script": func(e *Exposure) { e.Direct = false },
		"failed":      func(e *Exposure) { e.Succeeded = false },
		"shortened":   func(e *Exposure) { e.Shortened = true },
		"empty":       func(e *Exposure) { e.ResultBytes = 0 },
		"over bound":  func(e *Exposure) { e.ResultBytes = ResultCap + 1 },
	} {
		evidence := whole
		change(&evidence)
		if evidence.Whole() {
			t.Errorf("%s: counted as whole", name)
		}
	}
}

// Parts never split a character, never pass BodyCap, and hold the caller's
// bound, JSON escapes included.
func TestCutKeepsCharactersAndBounds(t *testing.T) {
	text := strings.Repeat("ж\"é😀x\n", 2000)
	for start := 0; start < len(text); {
		end := Cut(text, start, func(end int) bool { return Fits(text[start:end], "") })
		if end <= start {
			t.Fatalf("no progress at %d", start)
		}
		if end-start > BodyCap {
			t.Fatalf("part %d–%d passes BodyCap", start, end)
		}
		if !utf8.ValidString(text[start:end]) {
			t.Fatalf("part %d–%d splits a character", start, end)
		}
		if !Fits(text[start:end], "") {
			t.Fatalf("part %d–%d does not fit", start, end)
		}
		start = end
	}
	if Cut("abc", 0, func(int) bool { return false }) != 0 {
		t.Error("a text where nothing fits was cut anyway")
	}
	// A plain text is cut at BodyCap exactly: the result bound is not what
	// binds it.
	plain := strings.Repeat("a", 3*BodyCap)
	if end := Cut(plain, 0, func(end int) bool { return Fits(plain[:end], "") }); end != BodyCap {
		t.Errorf("plain text cut at %d, want %d", end, BodyCap)
	}
}

// The digest is over the words as a list: no joining makes two calls alike.
func TestTheDigestSeparatesWords(t *testing.T) {
	if Digest([]string{"send", "a b"}) == Digest([]string{"send", "a", "b"}) {
		t.Fatal("two different calls digest alike")
	}
	words := []string{"inbox"}
	if Digest(words) != Digest(append([]string{}, words...)) {
		t.Fatal("the same words digest differently")
	}
}
