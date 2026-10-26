package mail_test

import (
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
)

// recallOf is the note mail.Recall writes for a withdrawn task, its own text
// and all: the line a notice gives it is only as good as that text.
func recallOf(t *testing.T, id, from string, at time.Time) mail.Message {
	t.Helper()
	recall, err := mail.Recall(buildtest.Dir(t), mail.Message{ID: id, From: from, To: "api", Kind: mail.Task, CreatedAt: at, Text: "delete the staging bucket"})
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
	newer := mail.Message{ID: "n", From: "peer", Kind: mail.Note, CreatedAt: at.Add(time.Second), Text: "build finished"}
	got := mail.Notice(mail.Message{ID: "group", Batch: []mail.Message{newer, recall}})
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
	got := mail.Notice(mail.Message{ID: "group", Batch: []mail.Message{second, first}})
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
	neighbor := mail.Message{ID: "n", From: "peer", Kind: mail.Note, CreatedAt: at, Text: "build finished"}
	for _, message := range []mail.Message{recall, {ID: "group", Batch: []mail.Message{neighbor, recall}}} {
		got := mail.Notice(message)
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
	replacement := mail.Message{ID: "r", From: "main", Kind: mail.Task, Replaces: "100-5ca1ab1e0001", Text: "rerun the lint and the vet\ndetails"}
	want := "\n  ↳ Replaces 5ca1ab1e0001 (withdrawn): rerun the lint and the vet" + previewOnly
	if got := mail.Notice(replacement); !strings.HasSuffix(got, want) {
		t.Fatalf("notice = %q", got)
	}
	older := mail.Message{ID: "o", From: "peer", Kind: mail.Note, Text: "build finished"}
	replacement.CreatedAt = time.Now()
	if got := mail.Notice(mail.Message{ID: "group", Batch: []mail.Message{older, replacement}}); got != "Rewake: 2 new messages\n  ↳ main task: Replaces 5ca1ab1e0001 (withdrawn): rerun the lint… (a preview only: run rewake inbox)\n  ↳ peer notify: build finished" {
		t.Fatalf("grouped notice = %q", got)
	}
}

// A replacement is never hidden behind a newer neighbor either: its line,
// naming what it replaces, comes first, and the neighbor keeps its own.
func TestAReplacementIsShownBesideANewerLetter(t *testing.T) {
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	replacement := mail.Message{ID: "r", From: "main", Kind: mail.Note, CreatedAt: at, Replaces: "100-5ca1ab1e0001", Text: "use the replica"}
	newer := mail.Message{ID: "n", From: "peer", Kind: mail.Note, CreatedAt: at.Add(time.Second), Text: "build finished"}
	got := mail.Notice(mail.Message{ID: "group", Batch: []mail.Message{newer, replacement}})
	want := "Rewake: 2 new messages\n  ↳ main notify: Replaces 5ca1ab1e0001 (withdrawn): use the replica\n  ↳ peer notify: build finished"
	if got != want {
		t.Fatalf("notice = %q, want %q", got, want)
	}
}

// Two replacements are two lines, the older first, and a recall beside a
// replacement is a line too: no correction hides another.
func TestCorrectionsAreAllShownOldestFirst(t *testing.T) {
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	first := mail.Message{ID: "r1", From: "main", Kind: mail.Note, CreatedAt: at, Replaces: "100-5ca1ab1e0001", Text: "use the replica"}
	second := mail.Message{ID: "r2", From: "lead", Kind: mail.Note, CreatedAt: at.Add(time.Second), Replaces: "100-5ca1ab1e0002", Text: "skip the lint"}
	got := mail.Notice(mail.Message{ID: "group", Batch: []mail.Message{second, first}})
	want := "Rewake: 2 new messages\n  ↳ main notify: Replaces 5ca1ab1e0001 (withdrawn): use the replica\n  ↳ lead notify: Replaces 5ca1ab1e0002 (withdrawn): skip the lint"
	if got != want {
		t.Fatalf("two replacements: notice = %q, want %q", got, want)
	}
	recall := recallOf(t, "100-5ca1ab1e0003", "main", at.Add(2*time.Second))
	newer := mail.Message{ID: "n", From: "peer", Kind: mail.Note, CreatedAt: at.Add(3 * time.Second), Text: "build finished"}
	got = mail.Notice(mail.Message{ID: "group", Batch: []mail.Message{newer, recall, first}})
	want = "Rewake: 3 new messages\n  ↳ main notify: Replaces 5ca1ab1e0001 (withdrawn): use the replica\n  ↳ " + recall.Text + "\n  ↳ peer notify: build finished"
	if got != want {
		t.Fatalf("a recall and a replacement: notice = %q, want %q", got, want)
	}
}

// A single correcting letter is the letter its notice shows, so it takes no
// second line.
func TestASingleCorrectionIsShownOnce(t *testing.T) {
	recall := recallOf(t, "100-5ca1ab1e0001", "main", time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC))
	if got := mail.Notice(recall); strings.Count(got, "Do not act on") != 1 {
		t.Fatalf("recall shown as itself: notice = %q", got)
	}
}
