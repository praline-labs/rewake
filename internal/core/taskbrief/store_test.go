package taskbrief

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

var (
	main   = Author{Name: "main-one", Epoch: "100.1.boot"}
	worker = Author{Name: "write-one", Epoch: "200.2.boot"}
)

func room(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	return buildtest.Room(t, base, "room")
}

func made(t *testing.T, dir, text string) Brief {
	t.Helper()
	brief, err := New(dir, main, text)
	if err != nil {
		t.Fatal(err)
	}
	if err := Bind(dir, brief.ID, worker); err != nil {
		t.Fatal(err)
	}
	return brief
}

func loaded(t *testing.T, dir string, id int) Brief {
	t.Helper()
	brief, err := Load(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	return brief
}

// The records on disk are the contract the shell's view and every later
// build read: brief.json, the log by number and the text by edition.
func TestANewBriefIsWholeOnDisk(t *testing.T) {
	dir := room(t)
	brief := made(t, dir, "# Plan\n- [ ] one\n")
	path := filepath.Join(state.BriefsPath(dir), "1")
	for _, file := range []string{"brief.json", "log/1.json", "text/1.md"} {
		if _, err := os.Stat(filepath.Join(path, file)); err != nil {
			t.Errorf("%s: %v", file, err)
		}
	}
	got := loaded(t, dir, 1)
	if brief.ID != 1 || got.Edition != 1 || got.Revision != 1 || got.Title() != "Plan" || len(got.Bound) != 1 || got.Bound[0].Name != worker.Name {
		t.Fatalf("brief = %+v", got)
	}
	leftovers, _ := filepath.Glob(filepath.Join(state.BriefsPath(dir), tmpPrefix+"*"))
	if len(leftovers) != 0 {
		t.Errorf("left %v", leftovers)
	}
}

// An id is the highest open or archived plus one, so a closed brief's id is
// never given again.
func TestIDsCountTheArchiveToo(t *testing.T) {
	dir := room(t)
	made(t, dir, "one")
	made(t, dir, "two")
	if _, err := Close(dir, 2); err != nil {
		t.Fatal(err)
	}
	if third := made(t, dir, "three"); third.ID != 3 {
		t.Fatalf("the next id is %d", third.ID)
	}
}

// The review's case: two items of one text, the first ticked; main deletes
// it from the text with the ticks applied, and the second is not done.
func TestAnEditCarriesTheTicksInMainsText(t *testing.T) {
	dir := room(t)
	brief := made(t, dir, "## Frontend\n- [ ] Run tests\n## Backend\n- [ ] Run tests\n")
	if _, err := Tick(dir, brief.ID, worker, 1, 1, true); err != nil {
		t.Fatal(err)
	}
	read := loaded(t, dir, brief.ID)
	if read.Revision != 2 || !strings.Contains(read.Markdown(), "- [x] Run tests\n## Backend\n- [ ] Run tests") {
		t.Fatalf("revision %d, markdown %q", read.Revision, read.Markdown())
	}
	text := strings.Replace(read.Markdown(), "## Frontend\n- [x] Run tests\n", "", 1)
	edited, err := Edit(dir, brief.ID, main, read.Revision, text)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Edition != 2 || len(edited.Items()) != 1 || edited.Items()[0].Done {
		t.Fatalf("after the edit: edition %d, items %+v", edited.Edition, edited.Items())
	}
}

// A tick or an edit since main read the brief refuses main's edit, and a
// comment does not.
func TestAnEditNamesTheRevisionItStartedFrom(t *testing.T) {
	dir := room(t)
	brief := made(t, dir, "- [ ] one\n- [ ] two\n")
	if _, err := Comment(dir, brief.ID, worker, "on it"); err != nil {
		t.Fatal(err)
	}
	if read := loaded(t, dir, brief.ID); read.Revision != 1 {
		t.Fatalf("a comment moved the revision to %d", read.Revision)
	}
	if _, err := Tick(dir, brief.ID, worker, 1, 2, true); err != nil {
		t.Fatal(err)
	}
	var stale *StaleRevisionError
	if _, err := Edit(dir, brief.ID, main, 1, "- [ ] new\n"); !errors.As(err, &stale) || stale.Current != 3 {
		t.Fatalf("an edit from revision 1: %v", err)
	}
	if read := loaded(t, dir, brief.ID); read.Edition != 1 {
		t.Fatalf("a refused edit made edition %d", read.Edition)
	}
}

// A tick names its edition: refused once main edited since, never once the
// worker's own ticks came, and applied only within that edition.
func TestATickNamesItsEdition(t *testing.T) {
	dir := room(t)
	brief := made(t, dir, "- [ ] one\n- [ ] two\n")
	for _, n := range []int{1, 2} {
		if _, err := Tick(dir, brief.ID, worker, 1, n, true); err != nil {
			t.Fatalf("tick %d: %v", n, err)
		}
	}
	item, err := Tick(dir, brief.ID, worker, 1, 2, false)
	if err != nil || item.Text != "two" || item.Done {
		t.Fatalf("untick: %+v, %v", item, err)
	}
	read := loaded(t, dir, brief.ID)
	if read.Done() != 1 || read.Edition != 1 {
		t.Fatalf("done %d, edition %d", read.Done(), read.Edition)
	}
	if _, err := Edit(dir, brief.ID, main, read.Revision, "- [ ] one\n- [ ] two\n- [ ] three\n"); err != nil {
		t.Fatal(err)
	}
	var stale *StaleEditionError
	if _, err := Tick(dir, brief.ID, worker, 1, 3, true); !errors.As(err, &stale) || stale.Current != 2 {
		t.Fatalf("a tick of edition 1: %v", err)
	}
	if again := loaded(t, dir, brief.ID); again.Done() != 0 {
		t.Fatalf("edition 1's ticks count in edition 2: %+v", again.Items())
	}
	var none *NoItemError
	if _, err := Tick(dir, brief.ID, worker, 2, 4, true); !errors.As(err, &none) {
		t.Fatalf("item 4 of 3: %v", err)
	}
}

func TestOnlyABoundRunTicksAndComments(t *testing.T) {
	dir := room(t)
	brief := made(t, dir, "- [ ] one\n")
	later := Author{Name: worker.Name, Epoch: "300.3.boot"}
	var unbound *NotBoundError
	if _, err := Tick(dir, brief.ID, later, 1, 1, true); !errors.As(err, &unbound) {
		t.Fatalf("a later run of the name ticked: %v", err)
	}
	if _, err := Comment(dir, brief.ID, later, "hi"); !errors.As(err, &unbound) {
		t.Fatalf("a later run of the name commented: %v", err)
	}
	if err := Bind(dir, brief.ID, worker); err != nil {
		t.Fatal(err)
	}
	if err := Bind(dir, brief.ID, later); err != nil {
		t.Fatal(err)
	}
	if read := loaded(t, dir, brief.ID); len(read.Bound) != 2 {
		t.Fatalf("bound = %+v, want the earlier run kept and the later added once", read.Bound)
	}
}

func TestACommentIsBounded(t *testing.T) {
	dir := room(t)
	brief := made(t, dir, "text\n")
	if _, err := Comment(dir, brief.ID, worker, strings.Repeat("é", MaxComment)); err != nil {
		t.Fatalf("1000 characters: %v", err)
	}
	for _, text := range []string{strings.Repeat("a", MaxComment+1), " \n\t", "\xff"} {
		if _, err := Comment(dir, brief.ID, worker, text); err == nil {
			t.Errorf("comment %q was taken", text[:min(len(text), 10)])
		}
	}
	entry, err := Comment(dir, brief.ID, worker, "-- a dash first")
	if err != nil || entry.N != 3 {
		t.Fatalf("entry %+v, %v", entry, err)
	}
	if comments := loaded(t, dir, brief.ID).Comments(); len(comments) != 2 || comments[1].Text != "-- a dash first" {
		t.Fatalf("comments = %+v", comments)
	}
}

func TestAClosedBriefIsArchivedAndTakesNothing(t *testing.T) {
	dir := room(t)
	brief := made(t, dir, "- [ ] one\n")
	made(t, dir, "second\n")
	if already, err := Close(dir, brief.ID); err != nil || already {
		t.Fatalf("close: %v, %v", already, err)
	}
	if already, err := Close(dir, brief.ID); err != nil || !already {
		t.Fatalf("close again: %v, %v", already, err)
	}
	var closed *ClosedError
	if _, err := Tick(dir, brief.ID, worker, 1, 1, true); !errors.As(err, &closed) {
		t.Fatalf("a tick of a closed brief: %v", err)
	}
	if _, err := Edit(dir, brief.ID, main, 1, "x"); !errors.As(err, &closed) {
		t.Fatalf("an edit of a closed brief: %v", err)
	}
	open, _, err := List(dir, false)
	if err != nil || len(open) != 1 || open[0].ID != 2 {
		t.Fatalf("open = %+v, %v", open, err)
	}
	all, _, err := List(dir, true)
	if err != nil || len(all) != 2 || all[0].ID != 2 || all[1].ID != 1 || !all[1].IsClosed() {
		t.Fatalf("all = %+v, %v", all, err)
	}
}

// A close cut short after its record says closed is finished by the next.
func TestACloseCutShortIsFinished(t *testing.T) {
	dir := room(t)
	brief := made(t, dir, "text\n")
	if err := state.WithBriefLock(dir, func() error {
		record := loaded(t, dir, brief.ID).Record
		now := record.Created
		record.Closed = &now
		return writeRecord(openPath(dir, brief.ID), record)
	}); err != nil {
		t.Fatal(err)
	}
	if read := loaded(t, dir, brief.ID); !read.IsClosed() {
		t.Fatal("the record's close is not read")
	}
	if already, err := Close(dir, brief.ID); err != nil || already {
		t.Fatalf("close: %v, %v", already, err)
	}
	if _, err := os.Stat(filepath.Join(state.ClosedBriefsPath(dir), "1", "brief.json")); err != nil {
		t.Fatal(err)
	}
}

// A brief that does not read is named beside the others, and a stray entry
// of the directory is not taken for one.
func TestAnUnreadableBriefIsAProblem(t *testing.T) {
	dir := room(t)
	made(t, dir, "fine\n")
	broken := made(t, dir, "broken\n")
	if err := os.WriteFile(filepath.Join(openPath(dir, broken.ID), "brief.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(state.BriefsPath(dir), "notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	briefs, problems, err := List(dir, true)
	if err != nil || len(briefs) != 1 || briefs[0].ID != 1 || len(problems) != 2 {
		t.Fatalf("briefs %+v, problems %+v, %v", briefs, problems, err)
	}
}

func TestWritesNeedTheLeaseAndRefuseInAView(t *testing.T) {
	dir := room(t)
	brief := made(t, dir, "text\n")
	leave := state.EnterView()
	if _, err := New(dir, main, "x"); !errors.Is(err, state.ErrViewWrite) {
		t.Errorf("new in a view: %v", err)
	}
	if _, err := Comment(dir, brief.ID, worker, "x"); !errors.Is(err, state.ErrViewWrite) {
		t.Errorf("comment in a view: %v", err)
	}
	if _, _, err := List(dir, true); err != nil {
		t.Errorf("list in a view: %v", err)
	}
	_ = leave()
	base := t.TempDir()
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	unleased := buildtest.Established(t, base, "unleased")
	if _, err := New(unleased, main, "x"); !errors.Is(err, state.ErrNoLease) {
		t.Errorf("new without the lease: %v", err)
	}
}

func TestAMainTextIsChecked(t *testing.T) {
	dir := room(t)
	for _, text := range []string{"", "  \n", "\xff", strings.Repeat("a", MaxText+1)} {
		if _, err := New(dir, main, text); !errors.Is(err, ErrBadText) {
			t.Errorf("text of %d bytes: %v", len(text), err)
		}
	}
	if _, err := Load(dir, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("a refused text left a brief: %v", err)
	}
}
