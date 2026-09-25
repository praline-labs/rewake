package wrap

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/control"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// A worker that leaves while its command still asks — during the pickup,
// with its answer still to come — leaves main's record alone: the command
// holds it, and its answer, a refusal as not answering say, closes it without
// a letter. A letter sent meanwhile would contradict that answer.
func TestARecordItsCommandHoldsIsLeftAlone(t *testing.T) {
	saved := letterWait
	letterWait = 50 * time.Millisecond
	t.Cleanup(func() { letterWait = saved })
	dir, main, peer, observer := lettersFixture(t)
	release, err := control.Remember(dir, main.Name, control.Pending{ID: letterA, Worker: peer, AskerEpoch: main.Epoch(), AskedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Remove(dir, peer.Name); err != nil {
		t.Fatal(err)
	}
	scanTimes(t, observer, dir, main, 1)
	time.Sleep(2 * letterWait)
	scanTimes(t, observer, dir, main, 2)
	if got := compactionLetters(t, dir, main.Name); len(got) != 0 {
		t.Fatalf("a letter while the command still asks: %q", got)
	}
	// The command's answer was final: it closes the record and lets go.
	if err := control.Forget(dir, main.Name, letterA); err != nil {
		t.Fatal(err)
	}
	release()
	time.Sleep(2 * letterWait)
	scanTimes(t, observer, dir, main, 2)
	if got := compactionLetters(t, dir, main.Name); len(got) != 0 {
		t.Fatalf("a letter after the command closed the record: %q", got)
	}
}

// A command killed while it asks — SIGKILL included — lets go of its record
// with its process, and main's wrapper then closes it.
func TestAKilledCommandsRecordIsClosed(t *testing.T) {
	saved := letterWait
	letterWait = 50 * time.Millisecond
	t.Cleanup(func() { letterWait = saved })
	dir, main, peer, observer := lettersFixture(t)
	pending, err := json.Marshal(control.Pending{ID: letterA, Worker: peer, AskerEpoch: main.Epoch(), AskedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestHelperHoldsARecord$")
	command.Env = append(os.Environ(), "REWAKE_TEST_HOLD_DIR="+dir, "REWAKE_TEST_HOLD_ASKER="+main.Name, "REWAKE_TEST_HOLD_PENDING="+string(pending))
	out, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "held\n" {
		t.Fatalf("the helper said %q, %v", line, err)
	}
	if err := registry.Remove(dir, peer.Name); err != nil {
		t.Fatal(err)
	}
	scanTimes(t, observer, dir, main, 1)
	time.Sleep(2 * letterWait)
	scanTimes(t, observer, dir, main, 1)
	if got := compactionLetters(t, dir, main.Name); len(got) != 0 {
		t.Fatalf("a letter while the command lives: %q", got)
	}
	if err := command.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	scanTimes(t, observer, dir, main, 1)
	time.Sleep(2 * letterWait)
	scanTimes(t, observer, dir, main, 1)
	want := "Rewake: the compaction of worker-fixture you asked for failed (worker-fixture left before the compaction ended: not registered)."
	if got := compactionLetters(t, dir, main.Name); len(got) != 1 || got[0] != want {
		t.Fatalf("letters %q, want %q", got, want)
	}
	if left := control.PendingOf(dir, main.Name); len(left) != 0 {
		t.Fatalf("the record outlived its letter: %+v", left)
	}
}

// TestHelperHoldsARecord is the command of TestAKilledCommandsRecordIsClosed:
// it leaves a record, says so, and holds it until it is killed.
func TestHelperHoldsARecord(t *testing.T) {
	dir := os.Getenv("REWAKE_TEST_HOLD_DIR")
	if dir == "" {
		t.Skip("run by TestAKilledCommandsRecordIsClosed")
	}
	var pending control.Pending
	if err := json.Unmarshal([]byte(os.Getenv("REWAKE_TEST_HOLD_PENDING")), &pending); err != nil {
		t.Fatal(err)
	}
	if _, err := control.Remember(dir, os.Getenv("REWAKE_TEST_HOLD_ASKER"), pending); err != nil {
		t.Fatal(err)
	}
	_, _ = os.Stdout.WriteString("held\n")
	time.Sleep(time.Minute)
}

// compactionLetters are main's letters of compactions, without departures.
func compactionLetters(t *testing.T, dir, name string) []string {
	t.Helper()
	var texts []string
	for _, m := range letters(t, dir, name) {
		if m.Departure == nil && strings.HasPrefix(m.Text, "Rewake: ") {
			texts = append(texts, m.Text)
		}
	}
	return texts
}
