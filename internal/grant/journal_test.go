package grant

import (
	"fmt"
	"testing"
	"time"
)

func TestJournalIsPerRun(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	entries := []Entry{{Path: "/w/a", Message: "m1", At: at, Outcome: Granted}}
	if err := Save(dir, "writer", "e1", entries); err != nil {
		t.Fatal(err)
	}
	got := Load(dir, "writer", "e1")
	if len(got) != 1 || got[0].Path != "/w/a" || !got[0].Live() || !got[0].At.Equal(at) {
		t.Fatalf("Load = %+v", got)
	}
	if Load(dir, "writer", "e2") != nil || Load(dir, "other", "e1") != nil {
		t.Fatal("another run read this run's journal")
	}
	if Save(dir, "../x", "e1", entries) == nil || Save(dir, "writer", "", entries) == nil {
		t.Fatal("an invalid identity was saved")
	}
}

func TestJournalDropsEndedEntriesFirst(t *testing.T) {
	dir := t.TempDir()
	ended := time.Now()
	var entries []Entry
	for i := range maxEntries + 3 {
		entry := Entry{Path: fmt.Sprintf("/w/%d", i), Message: "m", Outcome: Granted}
		if i%10 == 5 {
			entry.Outcome, entry.EndedAt = Revoked, &ended
		}
		entries = append(entries, entry)
	}
	if err := Save(dir, "writer", "e1", entries); err != nil {
		t.Fatal(err)
	}
	got := Load(dir, "writer", "e1")
	if len(got) != maxEntries {
		t.Fatalf("kept %d entries, want %d", len(got), maxEntries)
	}
	live := 0
	for _, entry := range got {
		if entry.Live() {
			live++
		}
	}
	if live != maxEntries-4 {
		t.Fatalf("kept %d live entries of %d: live ones were dropped before ended ones", live, maxEntries-3)
	}
}
