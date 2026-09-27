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

// Past its bound a journal forgets the oldest ended entries, and never a live
// one: nine grants of eight directories each once lost the first grant here,
// and with it the way to take it back.
func TestJournalNeverDropsALiveEntry(t *testing.T) {
	dir := t.TempDir()
	ended := time.Now()
	var entries []Entry
	for i := range maxEnded + 3 {
		entries = append(entries, Entry{Path: fmt.Sprintf("/ended/%d", i), Message: "old", Outcome: Revoked, EndedAt: &ended})
	}
	for i := range MaxLive + 8 {
		entries = append(entries, Entry{Path: fmt.Sprintf("/live/%d", i), Message: "m", Outcome: Granted})
	}
	if err := Save(dir, "writer", "e1", entries); err != nil {
		t.Fatal(err)
	}
	got := Load(dir, "writer", "e1")
	live, kept := 0, 0
	for _, entry := range got {
		if entry.Live() {
			live++
		} else {
			kept++
		}
	}
	if live != MaxLive+8 || kept != maxEnded || got[0].Path != "/ended/3" {
		t.Fatalf("kept %d live of %d and %d ended of %d, first %q", live, MaxLive+8, kept, maxEnded+3, got[0].Path)
	}
}
