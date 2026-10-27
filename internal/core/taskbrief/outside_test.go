package taskbrief

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The briefs kept in a folder of the project (docs/briefs.md#where-a-brief-lives):
// the records on disk, what a wiped room still finds, and an id never taken
// again while the room lives.

func folderHome(t *testing.T) (Home, string) {
	t.Helper()
	folder := filepath.Join(t.TempDir(), "briefs")
	return Home{Room: room(t), Folder: folder}, folder
}

func TestABriefInTheProjectsFolderSurvivesAWipedRoom(t *testing.T) {
	home, folder := folderHome(t)
	first, err := New(home, main, "# Plan\n- [ ] one\n", "")
	if err != nil || first.ID != 1 || first.Room != "room" {
		t.Fatalf("brief %+v, %v", first, err)
	}
	for _, name := range []string{"brief.json", "log/1.json", "text/1.md"} {
		if _, err := os.Stat(filepath.Join(folder, "brief-1", name)); err != nil {
			t.Errorf("the brief's own format is missing %s: %v", name, err)
		}
	}
	if err := Bind(home, 1, worker, Work); err != nil {
		t.Fatal(err)
	}
	if _, err := Tick(home, 1, worker, 1, 1, true); err != nil {
		t.Fatal(err)
	}
	if _, number, err := Comment(home, 1, worker, "done"); err != nil || number != 1 {
		t.Fatalf("comment %d, %v", number, err)
	}

	wiped := Home{Room: room(t), Folder: folder}
	briefs, problems, err := List(wiped, true)
	if err != nil || len(briefs) != 1 || len(problems) != 0 || briefs[0].Done() != 1 {
		t.Fatalf("a wiped room lists %d briefs, %v, %v", len(briefs), problems, err)
	}
	next, err := New(wiped, main, "two", "")
	if err != nil || next.ID != 2 {
		t.Fatalf("the id after a wipe is %d, %v: it must pass brief-1", next.ID, err)
	}
}

func TestADirBriefIsIndexedAndClosedWhereItIs(t *testing.T) {
	home := Home{Room: room(t)}
	task := t.TempDir()
	brief, err := New(home, main, "# Task\n", task)
	if err != nil || brief.ID != 1 {
		t.Fatalf("brief %+v, %v", brief, err)
	}
	if _, err := os.Stat(filepath.Join(task, "brief-1", "brief.json")); err != nil {
		t.Fatal(err)
	}
	if again, err := New(home, main, "# Two\n", task); err != nil || again.ID != 2 {
		t.Fatalf("a second brief in one folder: %+v, %v", again, err)
	}
	if already, err := Close(home, 1); err != nil || already {
		t.Fatalf("close: %v, already %v", already, err)
	}
	if _, err := os.Stat(filepath.Join(task, "brief-1", "brief.json")); err != nil {
		t.Errorf("a closed brief of the project's folder must stay where it is: %v", err)
	}
	if already, err := Close(home, 1); err != nil || !already {
		t.Errorf("a second close: already %v, %v", already, err)
	}
	open, _, _ := List(home, false)
	all, _, _ := List(home, true)
	if len(open) != 1 || len(all) != 2 || all[1].ID != 1 || !all[1].IsClosed() {
		t.Errorf("open %d, all %d: the closed brief goes after the open one", len(open), len(all))
	}
	var closed *ClosedError
	if _, _, err := Comment(home, 1, worker, "late"); !errors.As(err, &closed) {
		t.Errorf("a comment on a closed brief: %v", err)
	}
}

func TestAMovedBriefReadsMovedAndKeepsItsID(t *testing.T) {
	home, folder := folderHome(t)
	if _, err := New(home, main, "# A\n- [ ] x\n", ""); err != nil {
		t.Fatal(err)
	}
	if err := Bind(home, 1, worker, Work); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(folder, folder+"-archived"); err != nil {
		t.Fatal(err)
	}
	briefs, problems, err := List(home, true)
	if err != nil || len(briefs) != 0 || len(problems) != 1 || problems[0].Why != "moved" {
		t.Fatalf("list: %d briefs, %v, %v", len(briefs), problems, err)
	}
	var moved *MovedError
	if _, err := Tick(home, 1, worker, 1, 1, true); !errors.As(err, &moved) {
		t.Errorf("a tick on a moved brief: %v", err)
	}
	if _, err := Load(home, 1); !errors.As(err, &moved) {
		t.Errorf("show of a moved brief: %v", err)
	}
	if already, err := Close(home, 1); err != nil || already {
		t.Fatalf("close of a moved brief: %v, %v", already, err)
	}
	if _, problems, _ := List(home, true); len(problems) != 0 {
		t.Errorf("a moved brief closed still lists as a problem: %v", problems)
	}
	if next, err := New(home, main, "# B\n", ""); err != nil || next.ID != 2 {
		t.Errorf("the id of a moved brief was taken again: %+v, %v", next, err)
	}
}

func TestAnotherRoomsBriefInASharedFolderIsNotOurs(t *testing.T) {
	home, folder := folderHome(t)
	other := Home{Room: buildRoomNamed(t, "other"), Folder: folder}
	foreign, err := New(other, main, "# Theirs\n", "")
	if err != nil || foreign.ID != 1 {
		t.Fatalf("%+v, %v", foreign, err)
	}
	if briefs, problems, err := List(home, true); err != nil || len(briefs) != 0 || len(problems) != 0 {
		t.Errorf("the room lists %d briefs of another room: %v, %v", len(briefs), problems, err)
	}
	if _, err := Load(home, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("show of another room's brief: %v", err)
	}
	if _, _, err := Comment(home, 1, worker, "hi"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a write to another room's brief: %v", err)
	}
	if next, err := New(home, main, "# Ours\n", ""); err != nil || next.ID != 2 {
		t.Errorf("a name in the shared folder was taken: %+v, %v", next, err)
	}
}

func TestOnlyAFolderOfTheProjectIsAdmitted(t *testing.T) {
	home := Home{Room: room(t)}
	for name, folder := range map[string]string{
		"relative": "briefs",
		"unclean":  t.TempDir() + "/a/../b",
		"state":    filepath.Join(home.Room, "briefs"),
		"around":   filepath.Dir(filepath.Dir(filepath.Dir(home.Room))),
	} {
		_, err := New(home, main, "x", folder)
		if !errors.Is(err, state.ErrOutsideRefused) {
			t.Errorf("%s: %v", name, err)
		}
	}
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := New(home, main, "x", link); !errors.Is(err, state.ErrOutsideRefused) {
		t.Errorf("a link: %v", err)
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Errorf("a refused folder was written: %v", entries)
	}
}

// buildRoomNamed is a leased room of its own base under a given name.
func buildRoomNamed(t *testing.T, name string) string {
	t.Helper()
	base := t.TempDir()
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	return buildtest.Room(t, base, name)
}

// Another room's brief that stands where ours was is not ours to write.
func TestARoomDoesNotWriteABriefThatReplacedItsOwn(t *testing.T) {
	folder := t.TempDir()
	a := Home{Room: buildRoomNamed(t, "a"), Folder: folder}
	b := Home{Room: buildRoomNamed(t, "b"), Folder: folder}
	if _, err := New(a, main, "# A", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(folder, "brief-1"), filepath.Join(t.TempDir(), "archived")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(b, main, "# B", ""); err != nil {
		t.Fatal(err)
	}
	var moved *MovedError
	if _, err := Edit(a, 1, main, 1, "# overwritten by A"); !errors.As(err, &moved) {
		got, _ := Load(b, 1)
		t.Fatalf("room A edited room B's brief (%v): room %q text %q", err, got.Room, got.Text)
	}
	if _, _, err := Comment(a, 1, worker, "x"); !errors.As(err, &moved) {
		t.Errorf("a comment on it: %v", err)
	}
}

// A link put where a directory of the brief was must not take a write out of
// the admitted folder.
func TestALinkInsideAnAdmittedFolderTakesNoWrite(t *testing.T) {
	home, folder := folderHome(t)
	if _, err := New(home, main, "# A", ""); err != nil {
		t.Fatal(err)
	}
	text := filepath.Join(folder, "brief-1", "text")
	elsewhere := filepath.Join(t.TempDir(), "text")
	if err := os.Rename(text, elsewhere); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, text); err != nil {
		t.Fatal(err)
	}
	if _, err := Edit(home, 1, main, 1, "# escaped"); err == nil {
		raw, _ := os.ReadFile(filepath.Join(elsewhere, "2.md"))
		t.Fatalf("an edit went through a link out of the folder: %q", raw)
	}
}

// A write inside an admission is a write of its room: once the lease is gone
// it is refused.
func TestAnAdmissionEndsWithItsRoomsLease(t *testing.T) {
	home := Home{Room: room(t)}
	folder := t.TempDir()
	err := state.WithOutside(home.Room, folder, func() error {
		buildtest.Release(home.Room)
		return state.WriteAtomic(filepath.Join(folder, "x"), []byte("escaped"))
	})
	if !errors.Is(err, state.ErrNoLease) {
		t.Fatalf("a write after the room's lease was released: %v", err)
	}
}

// A brief closed after its record went is a tombstone, not an error of the
// scan.
func TestAClosedTombstoneIsNotListed(t *testing.T) {
	home, folder := folderHome(t)
	if _, err := New(home, main, "# A", ""); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(folder, "brief-1", "brief.json")
	if err := os.Rename(record, record+".moved"); err != nil {
		t.Fatal(err)
	}
	if _, err := Close(home, 1); err != nil {
		t.Fatal(err)
	}
	if _, problems, err := List(home, true); err != nil || len(problems) != 0 {
		t.Fatalf("a closed moved brief remains listed: %v, %v", problems, err)
	}
}
