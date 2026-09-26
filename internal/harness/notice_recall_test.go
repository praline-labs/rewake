package harness

import (
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

// recallOf is the note inbox.Recall writes for a withdrawn task, its own text
// and all: the line a notice gives it is only as good as that text.
func recallOf(t *testing.T, id, from string, at time.Time) inbox.Message {
	t.Helper()
	recall, err := inbox.Recall(t.TempDir(), inbox.Message{ID: id, From: from, To: "api", Kind: inbox.Task, CreatedAt: at, Text: "delete the staging bucket"})
	if err != nil {
		t.Fatal(err)
	}
	recall.CreatedAt = at
	return recall
}

// A recall is never hidden behind a newer neighbor: it gets its own line,
// first, and the neighbor keeps the line it would have had.
func TestARecallIsShownBesideANewerLetter(t *testing.T) {
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	recall := recallOf(t, "100-5ca1ab1e0001", "main", at)
	newer := inbox.Message{ID: "n", From: "peer", Kind: inbox.Note, CreatedAt: at.Add(time.Second), Text: "build finished"}
	got := Notice(inbox.Message{ID: "group", Batch: []inbox.Message{newer, recall}})
	want := "Rewake: 2 new messages\n  ↳ " + recall.Text + "\n  ↳ peer notify: build finished"
	if got != want || strings.Contains(got, "…") {
		t.Fatalf("notice = %q, want %q", got, want)
	}
}

// Two recalls in one notice are two lines, the older first: neither hides the
// other.
func TestTwoRecallsAreBothShown(t *testing.T) {
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	first := recallOf(t, "100-5ca1ab1e0001", "main", at)
	second := recallOf(t, "100-5ca1ab1e0002", "main", at.Add(time.Second))
	got := Notice(inbox.Message{ID: "group", Batch: []inbox.Message{second, first}})
	want := "Rewake: 2 new messages\n  ↳ " + first.Text + "\n  ↳ " + second.Text
	if got != want {
		t.Fatalf("notice = %q, want %q", got, want)
	}
}

// The bound on a line cuts a long recall at its end, where the sender and the
// time are; which message not to act on survives.
func TestALongRecallKeepsItsInstruction(t *testing.T) {
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	recall := recallOf(t, "100-5ca1ab1e0001", strings.Repeat("sender", 20), at)
	neighbor := inbox.Message{ID: "n", From: "peer", Kind: inbox.Note, CreatedAt: at, Text: "build finished"}
	for _, message := range []inbox.Message{recall, {ID: "group", Batch: []inbox.Message{neighbor, recall}}} {
		got := Notice(message)
		if !strings.Contains(got, "\n  ↳ Do not act on task 5ca1ab1e0001 from sender") || !strings.Contains(got, "…") {
			t.Fatalf("notice = %q", got)
		}
		for _, line := range strings.Split(got, "\n")[1:] {
			if len([]rune(line)) > 100 {
				t.Fatalf("line of %d runes: %q", len([]rune(line)), line)
			}
		}
	}
}

// A replacement sent by rewake edit shows the new work and names what it
// replaces, in the one line a notice has for it.
func TestAReplacementNamesTheMessageItReplaces(t *testing.T) {
	replacement := inbox.Message{ID: "r", From: "main", Kind: inbox.Task, Replaces: "100-5ca1ab1e0001", Text: "rerun the lint and the vet\ndetails"}
	want := "\n  ↳ Replaces 5ca1ab1e0001 (withdrawn): rerun the lint and the vet"
	if got := Notice(replacement); !strings.HasSuffix(got, want) {
		t.Fatalf("notice = %q", got)
	}
	older := inbox.Message{ID: "o", From: "peer", Kind: inbox.Note, Text: "build finished"}
	replacement.CreatedAt = time.Now()
	if got := Notice(inbox.Message{ID: "group", Batch: []inbox.Message{older, replacement}}); got != "Rewake: 2 new messages\n  ↳ main task: Replaces 5ca1ab1e0001 (withdrawn): rerun the lint and the vet\n  ↳ peer notify: build finished" {
		t.Fatalf("grouped notice = %q", got)
	}
}

// A replacement is never hidden behind a newer neighbor either: its line,
// naming what it replaces, comes first, and the neighbor keeps its own.
func TestAReplacementIsShownBesideANewerLetter(t *testing.T) {
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	replacement := inbox.Message{ID: "r", From: "main", Kind: inbox.Note, CreatedAt: at, Replaces: "100-5ca1ab1e0001", Text: "use the replica"}
	newer := inbox.Message{ID: "n", From: "peer", Kind: inbox.Note, CreatedAt: at.Add(time.Second), Text: "build finished"}
	got := Notice(inbox.Message{ID: "group", Batch: []inbox.Message{newer, replacement}})
	want := "Rewake: 2 new messages\n  ↳ main notify: Replaces 5ca1ab1e0001 (withdrawn): use the replica\n  ↳ peer notify: build finished"
	if got != want {
		t.Fatalf("notice = %q, want %q", got, want)
	}
}

// Two replacements are two lines, the older first, and a recall beside a
// replacement is a line too: no correction hides another.
func TestCorrectionsAreAllShownOldestFirst(t *testing.T) {
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	first := inbox.Message{ID: "r1", From: "main", Kind: inbox.Note, CreatedAt: at, Replaces: "100-5ca1ab1e0001", Text: "use the replica"}
	second := inbox.Message{ID: "r2", From: "lead", Kind: inbox.Note, CreatedAt: at.Add(time.Second), Replaces: "100-5ca1ab1e0002", Text: "skip the lint"}
	got := Notice(inbox.Message{ID: "group", Batch: []inbox.Message{second, first}})
	want := "Rewake: 2 new messages\n  ↳ main notify: Replaces 5ca1ab1e0001 (withdrawn): use the replica\n  ↳ lead notify: Replaces 5ca1ab1e0002 (withdrawn): skip the lint"
	if got != want {
		t.Fatalf("two replacements: notice = %q, want %q", got, want)
	}
	recall := recallOf(t, "100-5ca1ab1e0003", "main", at.Add(2*time.Second))
	newer := inbox.Message{ID: "n", From: "peer", Kind: inbox.Note, CreatedAt: at.Add(3 * time.Second), Text: "build finished"}
	got = Notice(inbox.Message{ID: "group", Batch: []inbox.Message{newer, recall, first}})
	want = "Rewake: 3 new messages\n  ↳ main notify: Replaces 5ca1ab1e0001 (withdrawn): use the replica\n  ↳ " + recall.Text + "\n  ↳ peer notify: build finished"
	if got != want {
		t.Fatalf("a recall and a replacement: notice = %q, want %q", got, want)
	}
}

// A single delivery whose notice shows a newer unread letter still shows the
// correction it carries, on a line of its own before that letter's preview.
func TestASingleCorrectionIsShownBesideTheNewestUnread(t *testing.T) {
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	newer := inbox.Message{ID: "n", From: "peer", Kind: inbox.Note, CreatedAt: at.Add(time.Second), Text: "build finished"}
	recall := recallOf(t, "100-5ca1ab1e0001", "main", at)
	recall.Unread, recall.Latest = 2, &newer
	want := "Rewake: peer notify, 2 new messages\n  ↳ " + recall.Text + "\n  ↳ build finished"
	if got := Notice(recall); got != want {
		t.Fatalf("recall: notice = %q, want %q", got, want)
	}
	replacement := inbox.Message{ID: "r", From: "main", Kind: inbox.Task, CreatedAt: at, Replaces: "100-5ca1ab1e0002", Text: "use the replica", Unread: 2, Latest: &newer}
	want = "Rewake: peer notify, 2 new messages\n  ↳ main task: Replaces 5ca1ab1e0002 (withdrawn): use the replica\n  ↳ build finished"
	if got := Notice(replacement); got != want {
		t.Fatalf("replacement: notice = %q, want %q", got, want)
	}
	// Shown as the newest letter itself, a correction takes no second line.
	recall.Latest = nil
	if got := Notice(recall); strings.Count(got, "Do not act on") != 1 {
		t.Fatalf("recall shown as itself: notice = %q", got)
	}
}
