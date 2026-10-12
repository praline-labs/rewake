package upgraderig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The dimensions of the upgrade matrix (docs/v2/stage3-upgrade.md#the-tests).
// Each value is spelled as the table spells it, so the coverage the tests
// log reads against the table.
var (
	olds = []string{
		"quiet", "live wrapper", "launch lock held", "name lock held", "mailbox lock held",
		"unreadable record", "other namespace",
		"late bare room", "late live room", "late unreadable record", "late lock", "late replaced lock",
		"recreated bare", "recreated live", "rename fails", "sync fails", "delete fails", "partial validated",
	}
	stops    = []string{"none", "kill each", "pause admitted", "pause set aside", "pause validated"}
	entries  = []string{"rooms", "candidate", "validated", "rooms+candidate", "all"}
	launches = []string{"one", "two"}
)

// scenario is one world of the matrix: a value of each dimension.
type scenario struct {
	old, stop, trees, launches string
}

func (s scenario) values() []string { return []string{s.old, s.stop, s.trees, s.launches} }

func (s scenario) name() string { return strings.Join(s.values(), "/") }

func (s scenario) hasRooms() bool {
	return s.trees == "rooms" || s.trees == "rooms+candidate" || s.trees == "all"
}

func (s scenario) hasCandidate() bool { return s.trees != "rooms" && s.trees != "validated" }

func (s scenario) hasValidated() bool { return s.trees == "validated" || s.trees == "all" }

// checked says whether the world has a tree admission checks: rooms/ or a
// candidate. A validated tree is out of every 1.x path and never checked.
func (s scenario) checked() bool { return s.hasRooms() || s.hasCandidate() }

// feasible says whether a world can be set: a condition needs a tree it
// applies to, and a pause a step the clearing reaches.
func feasible(s scenario) bool {
	switch s.old {
	case "quiet", "delete fails":
	case "partial validated":
		if !s.hasValidated() {
			return false
		}
	default:
		if !s.checked() {
			return false
		}
	}
	switch s.stop {
	case "pause set aside":
		return s.hasRooms()
	case "pause validated":
		return s.checked()
	}
	return true
}

// The ids of the trees a world starts with, as a clearer that died left them.
const (
	leftID      = "1.2.0123456789abcdef"
	validatedID = "3.4.0123456789abcdef"
)

// target is where a world's condition lies: rooms/ when it has one, else the
// candidate. Its room is "old" either way, and holds the letter m1.
func (w *world) target(s scenario) string {
	if s.hasRooms() {
		return filepath.Join(w.base, "rooms")
	}
	return filepath.Join(w.base, ".rooms-1x-candidate-"+leftID)
}

// set makes the trees a world starts with and the condition it holds before
// any launch: processes of the stand-in for live state, files for the rest.
func (w *world) set(s scenario) {
	w.t.Helper()
	if s.hasRooms() {
		tree := filepath.Join(w.base, "rooms")
		w.standin("letter", tree, "old", "web", "m1")
	}
	if s.hasCandidate() {
		room, letter := "left", "m-left"
		if !s.hasRooms() {
			room, letter = "old", "m1"
		}
		w.standin("letter", filepath.Join(w.base, ".rooms-1x-candidate-"+leftID), room, "web", letter)
	}
	if s.hasValidated() {
		tree := filepath.Join(w.base, ".rooms-1x-validated-"+validatedID)
		for _, room := range []string{"va", "vb", "vc"} {
			w.standin("letter", tree, room, "web", "m-"+room)
		}
		write(w.t, filepath.Join(w.base, "v2", ".cleared-1x-"+validatedID+".json"), `{"rooms":3,"unread":3,"undelivered":0}`)
		if s.old == "partial validated" {
			if err := os.RemoveAll(filepath.Join(tree, "va")); err != nil {
				w.t.Fatal(err)
			}
		}
	}
	tree := w.target(s)
	switch s.old {
	case "live wrapper":
		w.standin("session", tree, "old", "web")
	case "launch lock held":
		w.standin("launching", tree, "old")
	case "name lock held":
		w.standin("hold", tree, "old", "sessions/.web.lock")
	case "mailbox lock held":
		w.standin("hold", tree, "old", "inbox/web/.lock")
	case "unreadable record":
		write(w.t, filepath.Join(tree, "old", "sessions", "web.json"), "{")
	case "other namespace":
		write(w.t, filepath.Join(tree, "old", "sessions", "web.json"), `{"servicePid":1,"serviceStart":1,"pidNamespace":"pid:[1]"}`)
	}
}

// eventStep is the step at which a world's condition arrives after the
// launch began, or "" for one there before.
func (s scenario) eventStep() string {
	switch {
	case strings.HasPrefix(s.old, "late "):
		return "legacy admitted"
	case strings.HasPrefix(s.old, "recreated "):
		if s.hasRooms() {
			return "legacy set aside"
		}
		return "legacy admitted"
	}
	return ""
}

// event brings a late condition about, where a 1.x process would: in the
// tree it opened before a rename, or in a rooms/ it recreates by path. It
// answers whether the condition arrived: a late one needs its tree still
// there, as a process cannot open a tree another launch already removed.
func (w *world) event(s scenario) bool {
	w.t.Helper()
	// A late condition arrives at admission, before any rename: in rooms/ the
	// 1.x process opens what the rename then carries away.
	tree := w.target(s)
	rooms := filepath.Join(w.base, "rooms")
	if _, err := os.Lstat(tree); err != nil && strings.HasPrefix(s.old, "late ") {
		return false
	}
	switch s.old {
	case "late bare room":
		w.standin("room", tree, "new")
	case "late live room":
		w.standin("session", tree, "new", "late")
	case "late unreadable record":
		write(w.t, filepath.Join(tree, "old", "sessions", "late.json"), "x")
	case "late lock":
		w.standin("hold", tree, "old", "sessions/.late.lock")
	case "late replaced lock":
		lock := filepath.Join(tree, "old", ".launch.lock")
		_ = os.Remove(lock)
		write(w.t, lock, "")
	case "recreated bare":
		w.standin("room", rooms, "x")
	case "recreated live":
		w.standin("session", rooms, "x", "web")
	}
	return true
}

// fault is the part of a world's fault plan its condition sets.
func (s scenario) fault() string {
	switch s.old {
	case "rename fails":
		return "other:failat=rename:/.rooms-1x-"
	case "sync fails":
		return "other:failat=sync:"
	case "delete fails":
		return "other:failat=remove:/.rooms-1x-validated-"
	}
	return ""
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
