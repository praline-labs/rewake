package mail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// decisionLab is journalLab's report decided: the subject, and a decision on
// it that holds together, with nothing written yet.
type decisionLab struct {
	dir, epoch string
	report     Message
	stop       Disposal
}

func newDecisionLab(t *testing.T) decisionLab {
	t.Helper()
	dir, epoch, report := journalLab(t)
	return decisionLab{dir: dir, epoch: epoch, report: report, stop: Disposal{Key: strings.Repeat("ab", 16), Occurrence: NewID()}}
}

func (l decisionLab) subject() Subject {
	r := l.report
	return Subject{
		Sender: r.From, SenderRun: r.FromEpoch, Journal: "end", Report: r.ID, Kind: r.Kind,
		Recipient: r.To, RecipientRun: r.ToEpoch,
	}
}

// decision is a decision on the lab's report: choice, by the person, its
// observations what a capture read of the journal.
func (l decisionLab) decision(t *testing.T, choice string) Decision {
	t.Helper()
	files := newCapture(osAccess{}, l.dir)
	if _, err := files.ReadFile(filepath.Join(JournalPath(l.dir, "api"), "end")); err != nil {
		t.Fatal(err)
	}
	observations, err := files.Observations()
	if err != nil {
		t.Fatal(err)
	}
	d := Decision{
		ID: NewID(), Choice: choice, By: DecidedBy{Kind: ByPerson}, At: time.Now(), Delivery: deliveryUnproven,
		Subject: l.subject(), Disposes: []Disposal{l.stop}, Observations: observations,
		Notes: []DecisionNote{{Told: ToldRecipient, Note: Message{
			ID: NewID(), Kind: Note, From: "api", To: l.report.To, ToEpoch: l.report.ToEpoch, Text: "decided", CreatedAt: time.Now(),
		}}},
	}
	if choice == ChoiceNotArrived {
		d.Resend = l.resend(t, d.ID, func(*TurnJournal) {})
	}
	return d
}

// resend is the journal a decision installs, as edit leaves it.
func (l decisionLab) resend(t *testing.T, decision string, edit func(*TurnJournal)) json.RawMessage {
	t.Helper()
	copied := l.report
	copied.ID, copied.Resends, copied.Decision = NewID(), l.report.ID, decision
	journal := TurnJournal{Epoch: l.epoch, Op: ResendJournal(decision), Reports: []Message{copied}}
	edit(&journal)
	raw, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// writeDecision is the only writer of a decision this part has: a test's,
// through the seam's write-once primitive under the lease stateDir took.
func writeDecision(t *testing.T, dir string, d Decision) string {
	t.Helper()
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return writeDecisionBytes(t, dir, d.Subject.Sender, d.Subject.Report, raw)
}

func writeDecisionBytes(t *testing.T, dir, name, report string, raw []byte) string {
	t.Helper()
	path := DecisionPath(dir, name, report)
	if err := state.EnsureSubdir(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := state.PublishExclusive(path, raw); err != nil {
		t.Fatal(err)
	}
	return path
}

// A decision gives authority only whole and in its place: every field it is
// read by is checked against its place and against the others, and a record
// that fails any of them reads as nothing decided.
func TestDecisionCorruptionNeverAuthorizesEffects(t *testing.T) {
	lab := newDecisionLab(t)
	for _, choice := range []string{ChoiceArrived, ChoiceNotArrived} {
		d := lab.decision(t, choice)
		raw, _ := json.Marshal(d)
		path := DecisionPath(lab.dir, "api", lab.report.ID)
		read, err := parseDecision(path, raw)
		if err != nil || read.ID != d.ID || read.Choice != choice {
			t.Fatalf("a whole decision %s does not read: %+v %v", choice, read, err)
		}
	}

	broken := map[string]func(*Decision){
		"a stranger's id":          func(d *Decision) { d.ID = "decision" },
		"no choice":                func(d *Decision) { d.Choice = "maybe" },
		"no time":                  func(d *Decision) { d.At = time.Time{} },
		"delivery proven":          func(d *Decision) { d.Delivery = "proven" },
		"another sender":           func(d *Decision) { d.Subject.Sender = "web" },
		"another report":           func(d *Decision) { d.Subject.Report = NewID() },
		"no sender run":            func(d *Decision) { d.Subject.SenderRun = "" },
		"a done journal":           func(d *Decision) { d.Subject.Journal = "end" + doneSuffix },
		"a journal path":           func(d *Decision) { d.Subject.Journal = "../end" },
		"no kind":                  func(d *Decision) { d.Subject.Kind = "" },
		"no recipient run":         func(d *Decision) { d.Subject.RecipientRun = "" },
		"a person verified":        func(d *Decision) { d.By.Verified = true },
		"a person naming a run":    func(d *Decision) { d.By.Run = d.Subject.RecipientRun },
		"a recipient unverified":   func(d *Decision) { d.By = DecidedBy{Kind: ByRecipient, Name: "web", Run: d.Subject.RecipientRun} },
		"a recipient's other run":  func(d *Decision) { d.By = DecidedBy{Kind: ByRecipient, Name: "web", Run: "old", Verified: true} },
		"nobody":                   func(d *Decision) { d.By = DecidedBy{Kind: "main"} },
		"a parent with no resend":  func(d *Decision) { d.Parent = NewID() },
		"a resend with no parent":  func(d *Decision) { d.Subject.Journal = ResendJournal(NewID()) },
		"nothing disposed":         func(d *Decision) { d.Disposes = nil },
		"an occurrence twice":      func(d *Decision) { d.Disposes = append(d.Disposes, d.Disposes[0]) },
		"a whole key":              func(d *Decision) { d.Disposes[0].Occurrence = "" },
		"a key of no shape":        func(d *Decision) { d.Disposes[0].Key = "key" },
		"a note told twice":        func(d *Decision) { d.Notes = append(d.Notes, d.Notes[0]) },
		"a note to someone else":   func(d *Decision) { d.Notes[0].Note.To = "ops" },
		"a task for a note":        func(d *Decision) { d.Notes[0].Note.Kind = Task },
		"a note from someone else": func(d *Decision) { d.Notes[0].Note.From = "web" },
		"nothing observed":         func(d *Decision) { d.Observations = nil },
		"bytes against their hash": func(d *Decision) { d.Observations[0].Bytes = append(d.Observations[0].Bytes, ' ') },
		"a failure without lstat": func(d *Decision) {
			d.Observations[0] = Observation{Op: observedRead, Path: d.Observations[0].Path, Err: &ObservedError{Kind: errNotExist, Text: "gone"}}
		},
		"an observation outside": func(d *Decision) { d.Observations[0].Path = "../elsewhere" },
	}
	for name, edit := range broken {
		t.Run(name, func(t *testing.T) {
			for _, choice := range []string{ChoiceArrived, ChoiceNotArrived} {
				d := lab.decision(t, choice)
				edit(&d)
				raw, _ := json.Marshal(d)
				if _, err := parseDecision(DecisionPath(lab.dir, "api", lab.report.ID), raw); err == nil {
					t.Fatalf("a decision %s with %s reads", choice, name)
				}
			}
		})
	}

	resends := map[string]func(d Decision) json.RawMessage{
		"none":                 func(Decision) json.RawMessage { return nil },
		"not a journal":        func(Decision) json.RawMessage { return json.RawMessage(`{"Reports":3}`) },
		"a field of no record": func(Decision) json.RawMessage { return json.RawMessage(`{"Epoch":"api-epoch","Extra":1}`) },
		"two reports": func(d Decision) json.RawMessage {
			return lab.resend(t, d.ID, func(j *TurnJournal) { j.Reports = append(j.Reports, j.Reports[0]) })
		},
		"another run": func(d Decision) json.RawMessage {
			return lab.resend(t, d.ID, func(j *TurnJournal) { j.Epoch = "other" })
		},
		"the original's operation": func(d Decision) json.RawMessage {
			return lab.resend(t, d.ID, func(j *TurnJournal) { j.Op = "end" })
		},
		"the original's tail": func(d Decision) json.RawMessage {
			return lab.resend(t, d.ID, func(j *TurnJournal) { j.Clear = []Waiter{{Name: "web"}} })
		},
		"a held end": func(d Decision) json.RawMessage {
			return lab.resend(t, d.ID, func(j *TurnJournal) { j.Held = "end" })
		},
		"progress": func(d Decision) json.RawMessage {
			return lab.resend(t, d.ID, func(j *TurnJournal) { j.Published = []string{j.Reports[0].ID} })
		},
		"the same id": func(d Decision) json.RawMessage {
			return lab.resend(t, d.ID, func(j *TurnJournal) { j.Reports[0].ID = lab.report.ID })
		},
		"another decision": func(d Decision) json.RawMessage {
			return lab.resend(t, d.ID, func(j *TurnJournal) { j.Reports[0].Decision = NewID() })
		},
		"another recipient run": func(d Decision) json.RawMessage {
			return lab.resend(t, d.ID, func(j *TurnJournal) { j.Reports[0].ToEpoch = "old" })
		},
		"another kind": func(d Decision) json.RawMessage {
			return lab.resend(t, d.ID, func(j *TurnJournal) { j.Reports[0].Kind = Task })
		},
	}
	for name, resend := range resends {
		t.Run("a resend with "+name, func(t *testing.T) {
			d := lab.decision(t, ChoiceNotArrived)
			d.Resend = resend(d)
			raw, _ := json.Marshal(d)
			if _, err := parseDecision(DecisionPath(lab.dir, "api", lab.report.ID), raw); err == nil {
				t.Fatalf("a decision whose resend is %s reads", name)
			}
		})
	}
	t.Run("arrived with a resend", func(t *testing.T) {
		d := lab.decision(t, ChoiceArrived)
		d.Resend = lab.resend(t, d.ID, func(*TurnJournal) {})
		raw, _ := json.Marshal(d)
		if _, err := parseDecision(DecisionPath(lab.dir, "api", lab.report.ID), raw); err == nil {
			t.Fatal("a decision of arrived with a resend reads")
		}
	})

	raw, _ := json.Marshal(lab.decision(t, ChoiceArrived))
	for name, bytes := range map[string][]byte{
		"two records":   append(append([]byte{}, raw...), raw...),
		"an extra":      []byte(strings.Replace(string(raw), `{"id"`, `{"extra":1,"id"`, 1)),
		"no record":     nil,
		"another shape": []byte(`[]`),
	} {
		if _, err := parseDecision(DecisionPath(lab.dir, "api", lab.report.ID), bytes); err == nil {
			t.Errorf("a decision record holding %s reads", name)
		}
	}
}

// A decision that does not read is an unknown of its own: the mailbox stops
// on it before any effect, and the report it names is not published for it.
func TestAnUnreadableDecisionStopsItsMailbox(t *testing.T) {
	lab := newDecisionLab(t)
	path := writeDecisionBytes(t, lab.dir, "api", lab.report.ID, []byte(`{"id":`))
	err := finishAll(t, lab.dir)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("an unreadable decision did not stop the mailbox: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state.InboxPath(lab.dir, "web"), lab.report.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("the report was published past an unreadable decision: %v", err)
	}
}

// The records beside a decision are read as strictly: a closure only for the
// report its name says, a note told only for an answer that tells one.
func TestDecisionRecordsBesideItReadStrictly(t *testing.T) {
	lab := newDecisionLab(t)
	base := DecisionPath(lab.dir, "api", lab.report.ID)
	closure := Closure{Decision: NewID(), Journal: "end", Report: lab.report.ID, Waits: []Waiter{{Name: "web"}}, At: time.Now()}
	raw, _ := json.Marshal(closure)
	if _, err := parseClosure(base+closedSuffix, raw); err != nil {
		t.Fatalf("a whole closure does not read: %v", err)
	}
	for name, edit := range map[string]func(*Closure){
		"another report":  func(c *Closure) { c.Report = NewID() },
		"no decision":     func(c *Closure) { c.Decision = "" },
		"a done journal":  func(c *Closure) { c.Journal = "end" + doneSuffix },
		"no time":         func(c *Closure) { c.At = time.Time{} },
		"too many waits":  func(c *Closure) { c.Waits = make([]Waiter, maxClosedWaits+1) },
		"no journal name": func(c *Closure) { c.Journal = "" },
	} {
		c := closure
		edit(&c)
		raw, _ := json.Marshal(c)
		if _, err := parseClosure(base+closedSuffix, raw); err == nil {
			t.Errorf("a closure with %s reads", name)
		}
	}

	told := ToldRecord{Name: "web", Run: lab.report.ToEpoch, Note: NewID(), Answer: ToldWritten, At: time.Now()}
	for _, answer := range []string{ToldWritten, ToldBefore, ToldEnded} {
		told.Answer = answer
		raw, _ := json.Marshal(told)
		if _, err := parseTold(base+toldRecipientSuffix, raw); err != nil {
			t.Fatalf("a note told %s does not read: %v", answer, err)
		}
	}
	for name, edit := range map[string]func(*ToldRecord){
		"a failed publication": func(r *ToldRecord) { r.Answer = "failed" },
		"no answer":            func(r *ToldRecord) { r.Answer = "" },
		"no run":               func(r *ToldRecord) { r.Run = "" },
		"no note":              func(r *ToldRecord) { r.Note = "note" },
		"no time":              func(r *ToldRecord) { r.At = time.Time{} },
	} {
		r := told
		edit(&r)
		raw, _ := json.Marshal(r)
		if _, err := parseTold(base+toldMainSuffix, raw); err == nil {
			t.Errorf("a note told with %s reads", name)
		}
	}

	summary := ResendSummary{Report: NewID(), Disposition: DispositionPublished, Decision: NewID()}
	if err := summary.check(ResendJournal(summary.Decision)); err != nil {
		t.Fatalf("a whole summary does not hold: %v", err)
	}
	for name, edit := range map[string]func(*ResendSummary){
		"another decision's journal": func(s *ResendSummary) { s.Decision = NewID() },
		"decided with no child":      func(s *ResendSummary) { s.Disposition = DispositionDecided },
		"a child of a published one": func(s *ResendSummary) { s.Child = NewID() },
		"no disposition":             func(s *ResendSummary) { s.Disposition = "landed" },
		"no report":                  func(s *ResendSummary) { s.Report = "" },
	} {
		s := summary
		edit(&s)
		if s.check(ResendJournal(summary.Decision)) == nil {
			t.Errorf("a summary with %s holds", name)
		}
	}
	journals := JournalPath(lab.dir, "api")
	done, _ := json.Marshal(TurnJournal{Epoch: lab.epoch, Done: true, Summary: &summary})
	if _, err := parseJournal(filepath.Join(journals, ResendJournal(summary.Decision)+doneSuffix), done); err != nil {
		t.Fatalf("a resend's done journal with its summary does not read: %v", err)
	}
	for _, name := range []string{"end", "end" + doneSuffix, ResendJournal(NewID())} {
		if _, err := parseJournal(filepath.Join(journals, name), done); err == nil {
			t.Errorf("the journal %s reads with a summary of decision %s", name, summary.Decision)
		}
	}
	stray, _ := json.Marshal(TurnJournal{Epoch: lab.epoch, Decided: []DecidedStep{{Report: lab.report.ID, Decision: "nobody"}}})
	if _, err := parseJournal(filepath.Join(journals, "end"), stray); err == nil {
		t.Error("a journal reads with a step decided by no decision")
	}
}

// decidedRecords are the records a decision on a report of the journal old
// leaves once its effects are done — the decision of arrived, its note told,
// its closure — by their paths under the sender's mailbox.
func decidedRecords(t *testing.T, sender, run, recipient, recipientRun string) map[string]string {
	t.Helper()
	report := NewID()
	note := Message{ID: NewID(), Kind: Note, From: sender, To: recipient, ToEpoch: recipientRun, Text: "decided", CreatedAt: time.Now()}
	d := Decision{
		ID: NewID(), Choice: ChoiceArrived, By: DecidedBy{Kind: ByPerson}, At: time.Now(), Delivery: deliveryUnproven,
		Subject: Subject{
			Sender: sender, SenderRun: run, Journal: "old", Report: report, Kind: Finished,
			Recipient: recipient, RecipientRun: recipientRun,
		},
		Disposes:     []Disposal{{Key: strings.Repeat("cd", 16), Occurrence: NewID()}},
		Notes:        []DecisionNote{{Told: ToldRecipient, Note: note}},
		Observations: []Observation{{Op: observedLstat, Path: filepath.Join("inbox", sender), Info: &ObservedInfo{Mode: 0o700, Size: 0}}},
	}
	records := map[string]any{
		report:                       d,
		report + toldRecipientSuffix: ToldRecord{Name: recipient, Run: recipientRun, Note: note.ID, Answer: ToldWritten, At: time.Now()},
		report + toldMainSuffix:      ToldRecord{Name: "lead", Run: "lead-run", Note: NewID(), Answer: ToldEnded, At: time.Now()},
		report + closedSuffix:        Closure{Decision: d.ID, Journal: "old", Report: report, Waits: []Waiter{}, At: time.Now()},
	}
	files := map[string]string{}
	for name, record := range records {
		raw, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Join(decisionsDir, name)] = string(raw)
	}
	return files
}
