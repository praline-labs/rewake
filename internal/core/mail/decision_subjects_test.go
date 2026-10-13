package mail

import (
	"context"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// Subjects are found behind every other unknown of the mailbox (Q5,
// docs/v2/stage3-decision-build.md): J's two reports, an interim end's report
// and a failure report to the sender's own name with no waits, each in a
// journal of its own and each unknown, beside an unrelated journal that does
// not read, named before them all or after. A report is a subject once an
// occurrence about it is open — J's second only after R is decided, since J
// meets it only then — and is listed and found while the rest stays
// unknown; each decision disposes of the occurrences about its own report
// alone, and every other occurrence, the unreadable journal's among them,
// stays open.
func TestDecisionFindsSubjectsBehindOtherUnknowns(t *testing.T) {
	for _, unrelated := range []string{"aaa", "zzz"} {
		t.Run(unrelated, func(t *testing.T) {
			t.Parallel()
			s := newMatrixScene(t, false)
			r := s.report
			r2 := s.secondReport()
			s.writeMark(r, "damaged")
			interim := s.journalOf("pend", s.epoch, Message{To: "web", ToEpoch: r.ToEpoch, Kind: Interim, Text: "still on it", InReplyTo: []string{NewID()}})
			api := startRoleRun(t, s.dir, "api", "")
			failure := s.journalOf("fail", api.session.Epoch(), Message{To: "api", ToEpoch: api.session.Epoch(), Kind: Error, Text: "the turn failed"})
			// A mark that does not read in the sender's own mailbox stops it as
			// a record before any step: the failure's unknown is its letter's
			// place, which holds what is no letter.
			s.writeMark(failure, onceIntent)
			s.access.mu.Lock()
			s.access.stat = func(path string, _ bool) (fs.FileInfo, error) {
				if filepath.Base(path) == failure.ID+".json" && filepath.Dir(path) == state.InboxPath(s.dir, "api") {
					return notRegular{filepath.Base(path)}, nil
				}
				return nil, nil
			}
			s.access.mu.Unlock()
			// A barrier records the first unknown it meets in each journal; the
			// next skips the reports already held.
			for range 3 {
				_ = s.barrier()
			}
			about := s.occurrencesAbout(t)
			for _, report := range []Message{r, interim, failure} {
				if len(about[report.ID]) != 1 {
					t.Fatalf("occurrences about %s: %v; all %v", report.ID, about[report.ID], about)
				}
			}
			if _, err := FindSubject(s.dir, "api", r2.ID); err == nil || len(about[r2.ID]) != 0 {
				t.Fatalf("J's second report is a subject before J met it: %v, %v", err, about[r2.ID])
			}
			s.decideOnly(t, r, about)
			for range 2 {
				_ = s.barrier()
			}
			writeRaw(t, filepath.Join(JournalPath(s.dir, "api"), unrelated), "{")
			_ = s.barrier()
			about = s.occurrencesAbout(t)
			reports := []Message{r2, interim, failure}
			for _, report := range reports {
				if len(about[report.ID]) != 1 {
					t.Fatalf("occurrences about %s: %v; all %v", report.ID, about[report.ID], about)
				}
			}
			if len(about[""]) != 1 {
				t.Fatalf("the unreadable journal's occurrences: %v", about[""])
			}

			listing, err := ListDecisions(s.dir, "api")
			if err != nil {
				t.Fatal(err)
			}
			for _, report := range reports {
				if !slices.ContainsFunc(listing.Subjects, func(v SubjectView) bool { return v.Subject.Report == report.ID && v.Decided == nil }) {
					t.Errorf("%s is not listed as a subject: %+v", report.ID, listing.Subjects)
				}
			}
			if !slices.ContainsFunc(listing.Undecidable, func(u Undecidable) bool { return strings.Contains(u.Why, filepath.Join("journal", unrelated)) }) {
				t.Errorf("the unreadable journal is not listed: %+v", listing.Undecidable)
			}
			for _, report := range reports {
				s.decideOnly(t, report, about)
			}
			if err := s.barrier(); err == nil {
				t.Error("the barrier went past the unreadable journal once every report was decided")
			}
		})
	}
}

// decideOnly has the person decide report "arrived" and holds the decision
// to the occurrences about it: it disposes of those, and every other open
// occurrence in about, but those of reports decided before, stays as it was.
func (s *matrixScene) decideOnly(t *testing.T, report Message, about map[string][]string) {
	t.Helper()
	subject, err := FindSubject(s.dir, "api", report.ID)
	if err != nil || subject.Kind != KindOf(report) || subject.Recipient != report.To {
		t.Fatalf("the subject %s: %+v, %v", report.ID, subject, err)
	}
	answer, err := Decide(context.Background(), s.dir, DecideRequest{Sender: "api", Report: report.ID, Choice: ChoiceArrived, By: DecidedBy{Kind: ByPerson}, Expect: subject})
	if err != nil {
		t.Fatalf("the decision on %s: %v", report.ID, err)
	}
	var disposed []string
	for _, d := range answer.Decision.Disposes {
		disposed = append(disposed, d.Occurrence)
	}
	if !slices.Equal(disposed, about[report.ID]) {
		t.Errorf("the decision on %s disposed %v; the occurrences about it are %v", report.ID, disposed, about[report.ID])
	}
	left := s.occurrencesAbout(t)
	for other, ids := range about {
		// A report decided before disposed of its own.
		decided := other == report.ID || other != "" && fileExists(DecisionPath(s.dir, "api", other))
		if !decided && !slices.Equal(left[other], ids) {
			t.Errorf("deciding %s touched the occurrences about %q: %v, were %v", report.ID, other, left[other], ids)
		}
	}
}

// journalOf writes api's journal id, of run epoch, carrying one report
// shaped as report says, its mark unreadable.
func (s *matrixScene) journalOf(id, epoch string, report Message) Message {
	s.t.Helper()
	report.ID, report.From, report.FromEpoch, report.CreatedAt = NewID(), "api", epoch, time.Now()
	journal := TurnJournal{Epoch: epoch, Reports: []Message{report}}
	if report.Kind == Interim {
		journal.Interim = &report.Text
	}
	if err := WriteJournal(s.dir, "api", id, journal); err != nil {
		s.t.Fatal(err)
	}
	s.writeMark(report, "damaged")
	return report
}

// occurrencesAbout are the open occurrences of api's mailbox by the report
// each is about; "" holds those about none.
func (s *matrixScene) occurrencesAbout(t *testing.T) map[string][]string {
	t.Helper()
	about := map[string][]string{}
	for _, stop := range stopsOnRecord(t, s.dir, "api") {
		about[stop.reportOf()] = append(about[stop.reportOf()], stop.id)
	}
	return about
}
