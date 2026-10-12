package legacy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/legacy"
)

// mailbox is the directory of a 1.x mailbox in a room.
func mailbox(room, name string) string { return filepath.Join(room, "inbox", name) }

// journal writes a turn journal of a mailbox in the field names 1.x writes.
func journal(t *testing.T, room, name, id, body string) {
	t.Helper()
	mustWrite(t, filepath.Join(mailbox(room, name), "journal", id), body)
}

// The counter counts a letter once, however many links its waiting and
// unread copies have, and not when it is a tombstone, read, failed or
// withdrawn; a report is undelivered only when no mark and no letter shows
// it went; anything it cannot read makes its count unknown, never zero.
func TestTheCounterCountsEachThingOnceAndNeverGuesses(t *testing.T) {
	type want struct{ rooms, unread, undelivered int }
	const report = `{"Epoch":"e1","Reports":[{"id":"r1","to":"api","toEpoch":"e2"}]}`
	cases := []struct {
		name  string
		setup func(t *testing.T, room string)
		want  want
	}{
		{"an empty room", func(*testing.T, string) {}, want{1, 0, 0}},
		{"a letter waiting and its unread link", func(t *testing.T, room string) {
			box := mailbox(room, "web")
			mustWrite(t, filepath.Join(box, "m1.json"), `{"id":"m1"}`)
			mustMkdir(t, filepath.Join(box, "unread"))
			if err := os.Link(filepath.Join(box, "m1.json"), filepath.Join(box, "unread", "m1.json")); err != nil {
				t.Fatal(err)
			}
			mustWrite(t, filepath.Join(box, "unread", "m2.json"), `{"id":"m2"}`)
		}, want{1, 2, 0}},
		{"read, failed, withdrawn and a tombstone", func(t *testing.T, room string) {
			box := mailbox(room, "web")
			for id, status := range map[string]string{"m1": `{"state":"read"}`, "m2": `{"state":"failed"}`, "m3": `{"state":"delivered","withdrawn":true}`, "m4": `{"state":"delivered"}`} {
				mustWrite(t, filepath.Join(box, "unread", id+".json"), `{"id":"`+id+`"}`)
				mustWrite(t, filepath.Join(box, id+".status"), status)
			}
			mustWrite(t, filepath.Join(box, "unread", "m5.json"), `{"id":"m5","withdrawn":{"by":"x"}}`)
			mustWrite(t, filepath.Join(box, "done", "m6.json"), `{"id":"m6"}`)
			mustWrite(t, filepath.Join(box, ".tmp-m7.json"), `{`)
		}, want{1, 1, 0}},
		{"a mailbox directory that cannot be listed", func(t *testing.T, room string) {
			mustWrite(t, filepath.Join(mailbox(room, "web"), "unread"), "")
		}, want{1, legacy.Unknown, 0}},
		{"a letter that does not parse", func(t *testing.T, room string) {
			mustWrite(t, filepath.Join(mailbox(room, "web"), "m1.json"), `{`)
		}, want{1, legacy.Unknown, 0}},
		{"a report never published", func(t *testing.T, room string) { journal(t, room, "web", "j1", report) }, want{1, 0, 1}},
		{"a report the journal lists published or moot", func(t *testing.T, room string) {
			journal(t, room, "web", "j1", `{"Reports":[{"id":"r1","to":"api","toEpoch":"e2"},{"id":"r2","to":"api","toEpoch":"e2"}],"Published":["r1"],"Moot":["r2"]}`)
		}, want{1, 0, 0}},
		{"a report its mark says published", func(t *testing.T, room string) {
			journal(t, room, "web", "j1", report)
			mustWrite(t, filepath.Join(mailbox(room, "api"), "once", "e2", "r1"), "published")
		}, want{1, 0, 0}},
		{"an intent whose letter is there", func(t *testing.T, room string) {
			journal(t, room, "web", "j1", report)
			mustWrite(t, filepath.Join(mailbox(room, "api"), "once", "e2", "r1"), "intent")
			mustWrite(t, filepath.Join(mailbox(room, "api"), "done", "r1.json"), `{"id":"r1"}`)
		}, want{1, 0, 0}},
		{"an intent with no letter", func(t *testing.T, room string) {
			journal(t, room, "web", "j1", report)
			mustWrite(t, filepath.Join(mailbox(room, "api"), "once", "e2", "r1"), "intent")
		}, want{1, 0, 1}},
		{"a mark that says neither", func(t *testing.T, room string) {
			journal(t, room, "web", "j1", report)
			mustWrite(t, filepath.Join(mailbox(room, "api"), "once", "e2", "r1"), "?")
		}, want{1, 0, legacy.Unknown}},
		{"a journal done and a journal kept done", func(t *testing.T, room string) {
			journal(t, room, "web", "j1", `{"Reports":[{"id":"r1","to":"api","toEpoch":"e2"}],"Done":true}`)
			journal(t, room, "web", "j2.done", report)
		}, want{1, 0, 0}},
		{"a journal that does not parse", func(t *testing.T, room string) { journal(t, room, "web", "j1", "{") }, want{1, 0, legacy.Unknown}},
		{"a conversion with unknown reports", func(t *testing.T, room string) {
			journal(t, room, "web", "conversion", `{"Unknown":["r1"]}`)
		}, want{1, 0, legacy.Unknown}},
		{"a turn record of the build before", func(t *testing.T, room string) {
			mustWrite(t, filepath.Join(mailbox(room, "web"), "turns", "t1"), "{}")
		}, want{1, 0, legacy.Unknown}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tree := t.TempDir()
			room := filepath.Join(tree, "old")
			mustMkdir(t, filepath.Join(room, "inbox", "web"))
			c.setup(t, room)
			got := legacy.Count(tree)
			if got != (legacy.Counts{Rooms: c.want.rooms, Unread: c.want.unread, Undelivered: c.want.undelivered}) {
				t.Fatalf("counted %+v, want %+v", got, c.want)
			}
		})
	}
	t.Run("rooms add up, an unknown stays unknown", func(t *testing.T) {
		tree := t.TempDir()
		for _, name := range []string{"a", "b"} {
			mustWrite(t, filepath.Join(mailbox(filepath.Join(tree, name), "web"), "m1.json"), `{"id":"m1"}`)
		}
		if got := legacy.Count(tree); got != (legacy.Counts{Rooms: 2, Unread: 2}) {
			t.Fatalf("counted %+v", got)
		}
		mustWrite(t, filepath.Join(mailbox(filepath.Join(tree, "c"), "web"), "m1.json"), `{`)
		if got := legacy.Count(tree); got != (legacy.Counts{Rooms: 3, Unread: legacy.Unknown}) {
			t.Fatalf("counted %+v", got)
		}
		mustWrite(t, filepath.Join(tree, "stray"), "")
		if got := legacy.Count(tree); got.Rooms != legacy.Unknown {
			t.Fatalf("a tree with a stray file counted %+v", got)
		}
	})
}
