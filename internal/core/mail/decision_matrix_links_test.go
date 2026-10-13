package mail

import (
	"os"
	"path/filepath"
	"testing"
)

// The decision's worlds behind a link (docs/v2/stage3-decision-manifest.md):
// R's occurrence on record, then the sender's mailbox or its stops moved
// aside and a link put in its place — dangling, or to the records themselves
// — or the directory of every mailbox replaced by a dangling link. The stops
// read as absent or unreadable, never followed: decide refuses, writes no
// decision on either side of the link and publishes nothing; with the
// directory back, the same words decide. A link above the mailbox to its own
// records is no world here: nothing promises a refusal for every ancestor
// replaced (the manifest says what it does).
func TestDecisionWorldsBehindLinks(t *testing.T) {
	places := map[string]func(dir string) string{
		"mailbox":   func(dir string) string { return JournalPath(dir, "api") + "/.." },
		"stops":     func(dir string) string { return stopsPath(dir, "api") },
		"mailboxes": func(dir string) string { return JournalPath(dir, "api") + "/../.." },
	}
	for name, place := range places {
		for _, dangling := range []bool{true, false} {
			label := name + "-to-records"
			if dangling {
				label = name + "-dangling"
			} else if name == "mailboxes" {
				continue
			}
			t.Run(label, func(t *testing.T) {
				t.Parallel()
				s := newMatrixScene(t, false)
				r := s.report
				s.writeMark(r, "damaged")
				if err := s.barrier(); err == nil {
					t.Fatal("the barrier went past R's unknown outcome")
				}
				path := filepath.Clean(place(s.dir))
				aside := path + ".aside"
				if err := os.Rename(path, aside); err != nil {
					t.Fatal(err)
				}
				target := aside
				if dangling {
					target = path + ".nowhere"
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
				before := len(s.access.letters)
				answer, err := s.decide(r, ChoiceNotArrived, false)
				if err == nil || answer.Decision.ID != "" {
					t.Fatalf("decide behind a link answered %+v, %v", answer, err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(aside, path); err != nil {
					t.Fatal(err)
				}
				if fileExists(DecisionPath(s.dir, "api", r.ID)) || len(s.access.letters) != before {
					t.Fatalf("a refusal behind a link wrote: the decision %v, letters %v", fileExists(DecisionPath(s.dir, "api", r.ID)), s.access.letters)
				}
				if _, err := s.decide(r, ChoiceNotArrived, false); err != nil {
					t.Fatalf("with the directory back, decide answered %v", err)
				}
			})
		}
	}
}
