package hostmode

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

// run feeds the reads at one instant and then lets every wait pass, and
// says what reached the program and which commands ran.
func run(reads ...string) (string, string) {
	var in input
	now := time.Unix(1000, 0)
	var sent bytes.Buffer
	var commands []byte
	collect := func(actions []action) {
		for _, a := range actions {
			sent.Write(a.send)
			if a.command != 0 {
				commands = append(commands, a.command)
			}
		}
	}
	for _, r := range reads {
		collect(in.feed([]byte(r), now))
	}
	collect(in.tick(now.Add(keyHold)))
	return sent.String(), string(commands)
}

func TestThePrefixRunsItsCommandAndSendsNothing(t *testing.T) {
	for _, prefix := range []string{"\x1c", "\x1b[92;5u", "\x1b[27;5;92~"} {
		sent, commands := run("ab" + prefix + "n" + "cd")
		if sent != "abcd" || commands != "n" {
			t.Errorf("prefix %q: sent %q, commands %q; want abcd and n", prefix, sent, commands)
		}
	}
}

// Every split of the same keys gives the same result: a read cut inside the
// prefix, inside an escape sequence or between the prefix and its command
// changes nothing.
func TestASplitAtAnyOffsetChangesNothing(t *testing.T) {
	stream := "x\x1b[A\x1b[27;5;92~2y\x1bOC\x1b[<0;5;40M\x1c\x1cz"
	wantSent, wantCommands := run(stream)
	if wantSent != "x\x1b[Ay\x1bOC\x1b[<0;5;40M\x1cz" || wantCommands != "2" {
		t.Fatalf("whole: sent %q, commands %q", wantSent, wantCommands)
	}
	for cut := 1; cut < len(stream); cut++ {
		sent, commands := run(stream[:cut], stream[cut:])
		if sent != wantSent || commands != wantCommands {
			t.Errorf("cut at %d: sent %q, commands %q", cut, sent, commands)
		}
	}
}

func TestAPasteIsNeverReadForThePrefix(t *testing.T) {
	sent, commands := run("\x1b[200~a\x1cnb\x1b[201~", "\x1cm")
	if sent != "\x1b[200~a\x1cnb\x1b[201~" || commands != "m" {
		t.Errorf("sent %q, commands %q", sent, commands)
	}
}

// A lone Escape goes out once keyHold passes, not before, and whole.
func TestALoneEscapeWaitsThenGoesAlone(t *testing.T) {
	var in input
	now := time.Unix(1000, 0)
	if got := in.feed([]byte("\x1b"), now); len(got) != 0 {
		t.Fatalf("sent at once: %q", got)
	}
	if at := in.deadline(); !at.Equal(now.Add(keyHold)) {
		t.Fatalf("deadline %v, want %v", at, now.Add(keyHold))
	}
	if got := in.tick(now.Add(keyHold - time.Millisecond)); len(got) != 0 {
		t.Fatalf("sent early: %q", got)
	}
	got := in.tick(now.Add(keyHold))
	if len(got) != 1 || string(got[0].send) != "\x1b" {
		t.Fatalf("got %q, want one escape", fmt.Sprint(got))
	}
}

// A sequence cut by a read is sent in one piece once its rest arrives.
func TestAKeyIsNeverSentInTwoWrites(t *testing.T) {
	var in input
	now := time.Unix(1000, 0)
	if got := in.feed([]byte("\x1b[1;5"), now); len(got) != 0 {
		t.Fatalf("sent a part: %q", got)
	}
	got := in.feed([]byte("C"), now.Add(10*time.Millisecond))
	if len(got) != 1 || string(got[0].send) != "\x1b[1;5C" {
		t.Fatalf("got %v", got)
	}
}

func TestAPendingPrefixLapses(t *testing.T) {
	var in input
	now := time.Unix(1000, 0)
	in.feed([]byte("\x1c"), now)
	if at := in.deadline(); !at.Equal(now.Add(prefixLapse)) {
		t.Fatalf("deadline %v", at)
	}
	in.tick(now.Add(prefixLapse))
	got := in.feed([]byte("n"), now.Add(prefixLapse+time.Millisecond))
	if len(got) != 1 || string(got[0].send) != "n" {
		t.Fatalf("after the lapse n was %v, want it sent", got)
	}
}

func TestAnUnknownKeyCancelsThePrefixAndMouseDoesNot(t *testing.T) {
	sent, commands := run("\x1cqa")
	if sent != "a" || commands != "" {
		t.Errorf("unknown key: sent %q, commands %q", sent, commands)
	}
	sent, commands = run("\x1c\x1b[<35;3;4M", "p")
	if sent != "\x1b[<35;3;4M" || commands != "p" {
		t.Errorf("mouse between: sent %q, commands %q", sent, commands)
	}
}

func TestMouseReportsCarryTheirRow(t *testing.T) {
	for token, row := range map[string]int{
		"\x1b[<0;12;24M":                      24,
		"\x1b[<64;1;3m":                       3,
		"\x1b[M" + string([]byte{32, 40, 57}): 25,
		"\x1b[A":                              0,
		"\x1b[<0;1M":                          0,
	} {
		if got := mouseRow([]byte(token)); got != row {
			t.Errorf("%q: row %d, want %d", token, got, row)
		}
	}
}
