package inbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/state"
)

// Every test of this package makes its state directory through stateDir,
// which checks, once the test is over and before the directory goes, that
// every file its writers left in any mailbox is of a kind the list names
// (records.go): a writer that adds a path without adding its kind fails here
// rather than stopping every mailbox its first file lands in.
func checkRecordKinds(t *testing.T, dir string) {
	t.Helper()
	t.Cleanup(func() {
		entries, err := os.ReadDir(state.InboxesPath(dir))
		if err != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			unlisted, err := UnlistedRecords(dir, entry.Name())
			if err != nil {
				continue
			}
			for _, path := range unlisted {
				t.Errorf("a writer left %s in the mailbox of %s, which is no kind of record on the list", path, entry.Name())
			}
		}
	})
}

// The list names every path the barrier meets; one it does not know stops
// the mailbox, and so does one of a known kind that does not read.
func TestAFileOfNoKnownKindStopsTheMailbox(t *testing.T) {
	for _, path := range []string{"stray", "journal/nested/file", "pending/kept.json"} {
		t.Run(path, func(t *testing.T) {
			dir := stateDir(t)
			file := filepath.Join(state.InboxPath(dir, "api"), path)
			if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
			if MailboxStopped(dir, "api") == nil {
				t.Fatalf("%s did not stop the mailbox", path)
			}
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// A path of every kind on the list, taken by an entry that is not a regular
// file, stops the mailbox before any effect: a directory there hides the
// record the barrier would read, and a link may lead anywhere. The sample
// path is checked against the list, so a kind added later is covered too.
func TestANonRegularEntryAtEveryRecordPathStops(t *testing.T) {
	kinds := append([]recordKind{{pattern: unfinishedWrite}}, recordKinds...)
	for _, kind := range kinds {
		sample := strings.NewReplacer("*", "x").Replace(kind.pattern)
		if kind.pattern == unfinishedWrite {
			sample = "journal/" + sample
		}
		if got, ok := kindOf(sample); !ok || got.pattern != kind.pattern {
			t.Fatalf("the sample %s of %s is of %q", sample, kind.pattern, got.pattern)
		}
		for _, shape := range []string{"directory", "link"} {
			t.Run(kind.pattern+"/"+shape, func(t *testing.T) {
				dir := stateDir(t)
				path := filepath.Join(state.InboxPath(dir, "api"), sample)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
				var err error
				if shape == "directory" {
					err = os.Mkdir(path, 0o700)
				} else {
					err = os.Symlink(target, path)
				}
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.RemoveAll(path) })
				if live(dir).readMailbox("api") == nil {
					t.Fatalf("a %s at %s did not stop the reading", shape, sample)
				}
				if MailboxStopped(dir, "api") == nil {
					t.Fatalf("a %s at %s did not stop the mailbox", shape, sample)
				}
			})
		}
	}
}

// The check over the writers names a directory no kind names or holds, as
// it names a file: a writer that makes one would stop every mailbox it lands
// in. A directory where a record belongs is a known path in the wrong shape,
// which the reading stops on, not an unlisted one.
func TestUnlistedRecordsNamesAStrayDirectory(t *testing.T) {
	dir := stateDir(t)
	mailbox := state.InboxPath(dir, "api")
	for _, path := range []string{"stray", "pending/kept.json", "awaiting/run"} {
		if err := os.MkdirAll(filepath.Join(mailbox, path), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	unlisted, err := UnlistedRecords(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(unlisted) != 1 || unlisted[0] != "stray/" {
		t.Fatalf("unlisted: %v", unlisted)
	}
	if err := os.RemoveAll(mailbox); err != nil {
		t.Fatal(err)
	}
}
