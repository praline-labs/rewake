package inbox

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

// stopLook is what one call learned of a mailbox's stop: what its plan
// found, the occurrences open after it, and the writes of its records that
// failed.
type stopLook struct {
	dir, name string
	records   mailboxRecords
	// found is what the plan found, as it said it; keys are its causes.
	found error
	keys  map[string]bool
	open  []openStop
	// failed are the stop's writes that did not go through: the answer is
	// then the one place left that names what was found.
	failed []error
}

// lookAtStop plans the mailbox, records an occurrence of every cause the
// plan found that has none open, resolves each the plan decided from the
// evidence it reads, and tells main of each occurrence still open. The
// caller holds the mailbox lock: the plan is a decision only under it.
func lookAtStop(dir, name string, open []openStop) stopLook {
	look := stopLook{dir: dir, name: name, keys: map[string]bool{}}
	planned := plan(dir, name, heldReports(open))
	look.records, look.found = planned.records, planned.found
	w := live(dir)
	for _, cause := range causesOf(dir, planned.found, causeUnreadable) {
		key := cause.key()
		look.keys[key] = true
		// Repeated observation adds nothing: the open occurrence is it.
		if slices.ContainsFunc(open, func(stop openStop) bool { return stop.key == key }) {
			continue
		}
		stop, err := w.recordOccurrence(name, cause)
		if err != nil {
			look.failed = append(look.failed, err)
			continue
		}
		open = append(open, stop)
	}
	for _, stop := range open {
		if stop.unreadable == nil && stop.resolution == nil && !look.keys[stop.key] {
			evidence, decided := look.evidence(&stop, planned.decided)
			if decided {
				err := w.resolveStop(name, stop, evidence)
				if err == nil {
					continue
				}
				look.failed = append(look.failed, err)
			}
		}
		look.open = append(look.open, stop)
	}
	for _, stop := range look.open {
		w.tellMain(name, stop)
	}
	return look
}

// evidence is what resolves an occurrence the plan did not find again, and
// whether there is any: a cause the plan found is resolved once the plan
// decided its operation and read every path it names, none removed since;
// an effect's, once its journal is completed. It notes on stop the paths
// found removed.
func (l stopLook) evidence(stop *openStop, decided decidedOps) ([]string, bool) {
	if stop.record.Kind == causeEffect {
		if done := planning(l.dir).completedOf(l.name, *stop); done != "" {
			return []string{done}, true
		}
	}
	evidence, removed, err := planning(l.dir).evidenceOf(*stop)
	stop.removed = removed
	return evidence, err == nil && removed == nil && stop.record.Kind != causeEffect && decided.covers(stop.record.Op)
}

// stopped says the mailbox is stopped for every call but the barrier.
func (l stopLook) stopped() bool {
	return l.found != nil || len(l.failed) > 0 || len(l.open) > 0
}

// holdsEffects says the barrier may not run its effects: what is open is
// more than occurrences only running the effects through can resolve.
func (l stopLook) holdsEffects() bool {
	return l.found != nil || len(l.failed) > 0 || slices.ContainsFunc(l.open, func(stop openStop) bool {
		return stop.unreadable != nil || stop.resolution != nil || stop.record.Kind != causeEffect || len(stop.removed) > 0
	})
}

// answer names what the plan found, every occurrence open beside it, and
// the writes that failed.
func (l stopLook) answer() error {
	var beside []openStop
	for _, stop := range l.open {
		if !l.keys[stop.key] {
			beside = append(beside, stop)
		}
	}
	return errors.Join(l.found, recordedStopError(l.dir, l.name, beside), errors.Join(l.failed...))
}

// decidedOps are the operations a plan ran through: journals, and reports
// inside them.
type decidedOps map[string]bool

// covers says the plan decided op: one about no operation is the reading's,
// which every plan runs through.
func (d decidedOps) covers(op string) bool {
	journal, report, _ := strings.Cut(op, "/")
	return op == "" || d[journal] || report != "" && d[report]
}

// heldReports are the open occurrences about a report, by the report.
func heldReports(open []openStop) map[string]openStop {
	held := map[string]openStop{}
	for _, stop := range open {
		if report := stop.reportOf(); report != "" && stop.unreadable == nil {
			held[report] = stop
		}
	}
	return held
}

// recordOccurrence writes a new occurrence of cause, write-once.
func (w world) recordOccurrence(name string, cause stopCause) (openStop, error) {
	present, absent := existenceOf(w.dir, cause.Paths)
	stop := openStop{key: cause.key(), id: NewID(), record: stopOccurrence{stopCause: cause, Present: present, Absent: absent, At: time.Now()}}
	raw, err := json.Marshal(stop.record)
	if err == nil {
		err = w.ensureDir(stopsPath(w.dir, name))
	}
	if err == nil {
		err = w.ensureDir(filepath.Dir(stop.path(w.dir, name)))
	}
	if err == nil {
		err = w.publish(stop.path(w.dir, name), raw)
	}
	if err != nil {
		return stop, fmt.Errorf("could not record the stop: %w", err)
	}
	return stop, nil
}

// existenceOf names the paths that are there and those a look found not
// there: "no such file", or "not a directory", which says a component on the
// way is a file, so nothing can stand at the path itself. A path whose look
// failed otherwise — permission, input and output — is in neither, its
// presence unknown. It looks past the seam: it is what the record keeps of
// the moment, not what a pass decides by. Only this record of the moment
// takes "not a directory" for absence; reading the path later is evidenceOf's,
// and there it reads nothing yet.
func existenceOf(dir string, paths []string) (present, absent []string) {
	for _, rel := range paths {
		_, err := os.Lstat(absolute(dir, rel))
		switch {
		case err == nil:
			present = append(present, rel)
		case errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR):
			absent = append(absent, rel)
		}
	}
	return present, absent
}

func absolute(dir, rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(dir, rel)
}

// resolveStop writes the resolution of an occurrence, naming its evidence.
// One resolved already stays resolved.
func (w world) resolveStop(name string, stop openStop, evidence []string) error {
	raw, err := json.Marshal(stopResolution{Evidence: evidence, At: time.Now()})
	if err == nil {
		err = w.publish(stop.path(w.dir, name)+resolvedSuffix, raw)
	}
	if err != nil && !errors.Is(err, state.ErrNameTaken) {
		return fmt.Errorf("could not record that the stop %s is resolved: %w", stop.path(w.dir, name), err)
	}
	return nil
}

// evidenceOf reads each path an occurrence names, as its kind: what it holds
// now is the evidence the plan decided by. A path gone since the record, or
// gone with its presence then unknown, may have been removed while stopped,
// which is no evidence; one a look found not there then is read as the
// operation reads it. An error says a path does not read yet.
func (w world) evidenceOf(stop openStop) (evidence, removed []string, err error) {
	for _, rel := range stop.record.Paths {
		path := absolute(w.dir, rel)
		info, err := w.stat(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if slices.Contains(stop.record.Absent, rel) {
				evidence = append(evidence, rel+": absent, as it was when the stop was recorded")
			} else {
				removed = append(removed, rel)
			}
		case err != nil:
			return nil, removed, err
		case info.IsDir():
			entries, err := w.readDir(path)
			if err != nil {
				return nil, removed, err
			}
			evidence = append(evidence, fmt.Sprintf("%s: a directory of %d entries", rel, len(entries)))
		default:
			raw, err := w.readFile(path)
			if err != nil {
				return nil, removed, err
			}
			if kind, ok := kindAt(rel); ok && kind.parse != nil {
				if err := kind.parse(path, raw); err != nil {
					return nil, removed, unknownRecord(path, err)
				}
			}
			evidence = append(evidence, fmt.Sprintf("%s: %d bytes, sha256 %x", rel, len(raw), sha256.Sum256(raw)))
		}
	}
	if len(evidence) == 0 {
		evidence = []string{"the plan ran the operation through"}
	}
	return evidence, removed, nil
}

// kindAt is the kind of a path under the state directory, in whichever
// mailbox.
func kindAt(rel string) (recordKind, bool) {
	parts := strings.SplitN(rel, string(filepath.Separator), 3)
	if len(parts) < 3 || parts[0] != "inbox" {
		return recordKind{}, false
	}
	return kindOf(parts[2])
}

// tellMain tells the room's main of an occurrence, once: through a letter
// published once under the occurrence's id, and a record that it was. A main
// that cannot be told now is told by the next call that finds the
// occurrence open. A mailbox that is main's own tells nobody: every call it
// makes answers the stop.
func (w world) tellMain(name string, stop openStop) {
	if stop.told {
		return
	}
	main, ok := roomMainOf(w.dir)
	if !ok || main.Name == name {
		return
	}
	from := ""
	if self, err := registry.LookupReadOnly(w.dir, name); err == nil {
		from = self.Epoch()
	}
	note := Message{
		ID: stop.id, From: name, FromEpoch: from, To: main.Name, ToEpoch: main.Epoch(), Kind: Note, CreatedAt: time.Now(),
		Text: fmt.Sprintf("%s stopped changing its mailbox: %s. Letters to it still arrive; it reads and reports nothing until evidence settles the cause.", name, stop.describe(w.dir, name)),
	}
	ctx, cancel := context.WithTimeout(context.Background(), recipientLockWait)
	defer cancel()
	if _, err := PublishOnce(ctx, w.dir, note, nil); err != nil {
		return
	}
	_ = w.publish(stop.path(w.dir, name)+toldSuffix, []byte(main.Name+" "+main.Epoch()+"\n"))
}

// roomMainOf is the room's live main, read without the cleanup a lookup does:
// the caller holds a mailbox lock.
func roomMainOf(dir string) (registry.Session, bool) {
	sessions, err := registry.ListReadOnly(dir)
	if err != nil {
		return registry.Session{}, false
	}
	for _, session := range sessions {
		if role.Of(session.Role).ID == role.Main.ID {
			return session, true
		}
	}
	return registry.Session{}, false
}
