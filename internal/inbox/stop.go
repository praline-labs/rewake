package inbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// A stop is records of the mailbox (8-stop), which every later call that
// would change the mailbox answers first, whatever call found it. Each cause
// has a key, from its kind, the paths it rests on and the operation it is
// about; each time a cause is found while no occurrence of its key is open is
// an occurrence: one write-once record, stops/<key>/<occurrence>. A stop holds
// while any occurrence is open.
//
// An occurrence is resolved only by evidence about its effect, never by its
// cause going away (docs/rules/effects.md#how-e8-lifts-a-stop): removing a
// record whose effect is unknown does not make the effect absent, and a plan
// that read "no such file" there would publish without the evidence E1 asks
// for. So a cause the plan found is resolved once a plan under the mailbox
// lock reads each path it names, as its kind, and decides the operation it
// is about; a path gone since the occurrence was recorded resolves nothing.
// One an effect met, which the plan did not show, is resolved once a barrier
// has run that operation through. The resolution is a record of its own,
// <occurrence>.resolved, naming the evidence; no stop record is edited or
// removed, and a cause found again after its resolution is a new occurrence,
// which needs evidence of its own.

// stopsDir holds the stop records of a mailbox.
const stopsDir = "stops"

const (
	resolvedSuffix = ".resolved"
	toldSuffix     = ".told"
)

// The kinds of cause.
const (
	// causeUnreadable is a record that cannot be read as its kind.
	causeUnreadable = "unreadable"
	// causeUnlisted is a path of no kind a mailbox holds, or of a kind in a
	// shape it is never written in.
	causeUnlisted = "unlisted"
	// causeEffect is an unknown an effect met, which the plan did not show.
	causeEffect = "effect"
	// causeUndecided is a plan that failed on nothing a path names: a
	// lookup, say.
	causeUndecided = "undecided"
)

// stopCause is one cause a pass found.
type stopCause struct {
	Kind string `json:"kind"`
	// Paths are what the cause rests on, relative to the state directory.
	Paths []string `json:"paths,omitempty"`
	// Op is the operation the cause is about: a journal, or a journal and
	// one of its reports; empty for a record that names none.
	Op    string `json:"op,omitempty"`
	Cause string `json:"cause"`
}

// key is fixed-length, so a path's history adds entries, never a longer
// name.
func (c stopCause) key() string {
	parts := append([]string{c.Kind, c.Op}, slices.Sorted(slices.Values(c.Paths))...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:16])
}

// stopOccurrence is the record of one occurrence.
type stopOccurrence struct {
	stopCause
	// Present are the paths that were there when it was recorded, Absent
	// those a look found not there: only "no such file", or "not a
	// directory" on the way to it, proves absence (E6). A path in neither
	// could not be looked at, so whether it was there is unknown. One gone
	// since that was present, or whose presence was unknown, may have been
	// removed while stopped: its absence is no evidence. One that was absent
	// is read as the operation reads it.
	Present []string  `json:"present,omitempty"`
	Absent  []string  `json:"absent,omitempty"`
	At      time.Time `json:"at"`
}

// stopResolution is the record that resolves an occurrence.
type stopResolution struct {
	Evidence []string  `json:"evidence"`
	At       time.Time `json:"at"`
}

// readResolution reads the resolution at path as its kind: evidence named
// and a time. Its name being there proves nothing until its bytes say so, and
// an entry that names nothing — empty, blank, or a null the decoder reads as
// empty — is no evidence.
func (w world) readResolution(path string) error {
	raw, err := w.readFile(path)
	if err != nil {
		return err
	}
	var resolution stopResolution
	if err := json.Unmarshal(raw, &resolution); err != nil {
		return err
	}
	if len(resolution.Evidence) == 0 || resolution.At.IsZero() {
		return fmt.Errorf("the resolution %s names no evidence", path)
	}
	for _, evidence := range resolution.Evidence {
		if strings.TrimSpace(evidence) == "" {
			return fmt.Errorf("the resolution %s names an evidence entry that says nothing", path)
		}
	}
	return nil
}

// openStop is an occurrence without a resolution.
type openStop struct {
	key, id string
	record  stopOccurrence
	// unreadable says why the record cannot be read: what stopped the
	// mailbox is unknown, and nothing resolves it until it reads.
	unreadable error
	// resolution says why a resolution beside the record cannot be read:
	// whether the occurrence was resolved is unknown, so it stays open and
	// nothing resolves it again until the resolution reads.
	resolution error
	told       bool
	// removed are the paths found gone since the record.
	removed []string
}

func stopsPath(dir, name string) string {
	return filepath.Join(state.InboxPath(dir, name), stopsDir)
}

func (o openStop) path(dir, name string) string {
	return filepath.Join(stopsPath(dir, name), o.key, o.id)
}

// reportOf is the report an occurrence is about, empty when it is about no
// one report.
func (o openStop) reportOf() string {
	if _, report, ok := strings.Cut(o.record.Op, "/"); ok {
		return report
	}
	return ""
}

func (o openStop) describe(dir, name string) string {
	if o.unreadable != nil {
		return fmt.Sprintf("the stop record %s cannot be read (%v), so why it stopped is unknown", o.path(dir, name), o.unreadable)
	}
	text := o.record.Cause
	if o.resolution != nil {
		text += fmt.Sprintf("; its resolution %s cannot be read (%v), so whether it was resolved is unknown", o.path(dir, name)+resolvedSuffix, o.resolution)
	}
	for _, rel := range o.removed {
		if slices.Contains(o.record.Present, rel) {
			text += fmt.Sprintf("; %s was removed while stopped, which is no evidence of what it said", filepath.Join(dir, rel))
		} else {
			text += fmt.Sprintf("; %s is gone, and whether it was there when the stop was recorded is unknown, so its absence is no evidence", filepath.Join(dir, rel))
		}
	}
	return text
}

// stopState answers the open occurrences of a mailbox: the one reader of a
// stop. Records that cannot be listed are an error, which is a stop too.
func stopState(dir, name string) ([]openStop, error) { return live(dir).stopState(name) }

func (w world) stopState(name string) ([]openStop, error) {
	root := stopsPath(w.dir, name)
	keys, err := w.readDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("the stop records %s cannot be listed, so whether %s is stopped is unknown: %w", root, name, err)
	}
	var open []openStop
	for _, key := range keys {
		if !key.IsDir() || strings.HasPrefix(key.Name(), ".") {
			continue
		}
		entries, err := w.readDir(filepath.Join(root, key.Name()))
		if err != nil {
			return nil, fmt.Errorf("the stop records %s cannot be listed, so whether %s is stopped is unknown: %w", filepath.Join(root, key.Name()), name, err)
		}
		names := map[string]bool{}
		for _, entry := range entries {
			names[entry.Name()] = true
		}
		for _, entry := range entries {
			id := entry.Name()
			if strings.HasPrefix(id, ".") || strings.HasSuffix(id, resolvedSuffix) || strings.HasSuffix(id, toldSuffix) {
				continue
			}
			stop := openStop{key: key.Name(), id: id, told: names[id+toldSuffix]}
			if names[id+resolvedSuffix] {
				// A resolution listed and gone is no resolution either: none
				// is ever removed, so the listing is all that is known.
				err := w.readResolution(stop.path(w.dir, name) + resolvedSuffix)
				if err == nil {
					continue
				}
				stop.resolution = err
			}
			raw, err := w.readFile(stop.path(w.dir, name))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err == nil {
				err = json.Unmarshal(raw, &stop.record)
			}
			stop.unreadable = err
			open = append(open, stop)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].id < open[j].id })
	return open, nil
}

// RecordedStopError is a stop some earlier call found and recorded.
type RecordedStopError struct {
	Name, Cause string
}

func (e *RecordedStopError) Error() string {
	return fmt.Sprintf("%s stopped changing its mailbox: %s; the next turn end looks again", e.Name, e.Cause)
}

func recordedStopError(dir, name string, open []openStop) error {
	if len(open) == 0 {
		return nil
	}
	causes := make([]string, 0, len(open))
	for _, stop := range open {
		causes = append(causes, stop.describe(dir, name))
	}
	return &RecordedStopError{Name: name, Cause: strings.Join(causes, "; and ")}
}

// UnknownRecordError is a record an effect needed and could not read as what
// it is: an unknown, not a failure a retry may clear.
type UnknownRecordError struct {
	Path string
	Err  error
}

func (e *UnknownRecordError) Error() string { return e.Err.Error() }

func (e *UnknownRecordError) Unwrap() error { return e.Err }

// unknownRecord marks err as the unknown a record at path leaves.
func unknownRecord(path string, err error) error {
	return &UnknownRecordError{Path: path, Err: err}
}

// opError names the operation an error came from: a journal, and inside it
// a report.
type opError struct {
	op  string
	err error
}

func (e *opError) Error() string { return e.err.Error() }

func (e *opError) Unwrap() error { return e.err }

func inOp(op string, err error) error {
	if err == nil {
		return nil
	}
	return &opError{op: op, err: err}
}

// heldReportError is a report an open occurrence is about whose recipient's
// run is not live: it stays where it is, its occurrence open, rather than be
// recorded moot, which says nothing of whether it landed.
type heldReportError struct{ stop openStop }

func (e *heldReportError) Error() string {
	return fmt.Sprintf("the report %s stays unsettled while a stop about it is open: %s", e.stop.reportOf(), e.stop.record.Cause)
}

// causesOf names the causes an error carries, one per branch of a join, each
// with the operations it came through. kind is what an unknown read is: one
// the plan found, or one an effect met.
func causesOf(dir string, err error, unknownKind string) []stopCause {
	var causes []stopCause
	seen := map[string]bool{}
	add := func(cause stopCause) {
		if key := cause.key(); !seen[key] {
			seen[key] = true
			causes = append(causes, cause)
		}
	}
	var visit func(top error, ops []string)
	visit = func(top error, ops []string) {
		for at := top; at != nil; at = errors.Unwrap(at) {
			switch e := at.(type) {
			case *heldReportError:
				add(e.stop.record.stopCause)
				return
			case *opError:
				ops = append(ops, e.op)
			case *unlistedRecordError:
				add(stopCause{Kind: causeUnlisted, Paths: []string{relative(dir, e.Path)}, Op: strings.Join(ops, "/"), Cause: top.Error()})
				return
			case *UnknownRecordError:
				add(stopCause{Kind: unknownKind, Paths: []string{relative(dir, e.Path)}, Op: strings.Join(ops, "/"), Cause: top.Error()})
				return
			case interface{ Unwrap() []error }:
				for _, branch := range e.Unwrap() {
					visit(branch, slices.Clone(ops))
				}
				return
			}
		}
		add(stopCause{Kind: causeUndecided, Op: strings.Join(ops, "/"), Cause: top.Error()})
	}
	if err != nil {
		visit(err, nil)
	}
	return causes
}

// relative is a path as a stop record names it: under the state directory,
// so that the key is the same wherever the directory lies.
func relative(dir, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}
