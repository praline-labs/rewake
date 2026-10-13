package mail

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// heldBy are the reports a pass holds to their evidence (journal_held.go):
// those an open occurrence is about, and those one resolved is about. A
// resolution is durable before the journal records what its evidence
// settled, and a pass that only looks at the stop (MailboxStopped) resolves
// without running the journal at all; a report released from its occurrence
// there would meet its run's end in the next pass as an ordinary report and
// be made moot, though what resolved it proves it landed. So the occurrence
// keeps the report held until its journal records it: the journal never
// delivers a report it recorded, so a resolved occurrence about one recorded
// already holds nothing. A report no occurrence was ever about keeps the
// ordinary rule. An open occurrence wins over a resolved one about the same
// report: it is the stop the report answers to. A resolved record that
// cannot be read may hold a report, so it fails the look as any record would.
// The records are the stop's own, read as stopState reads them, not as a
// pass reads the mailbox.
func (w world) heldBy(name string, open []openStop) (map[string]openStop, error) {
	held, err := w.resolvedReports(name)
	for report, stop := range heldReports(open) {
		held[report] = stop
	}
	return held, err
}

// resolvedReports are the resolved occurrences about a report, by the report.
// A resolution that does not read leaves its occurrence open, where stopState
// finds it, so it is not one of these.
func (w world) resolvedReports(name string) (map[string]openStop, error) {
	held := map[string]openStop{}
	root := stopsPath(w.dir, name)
	keys, err := w.readDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return held, nil
	}
	if err != nil {
		return held, fmt.Errorf("the stop records %s cannot be listed, so which reports they hold is unknown: %w", root, err)
	}
	var failed []error
	for _, key := range keys {
		if !key.IsDir() || strings.HasPrefix(key.Name(), ".") {
			continue
		}
		entries, err := w.readDir(filepath.Join(root, key.Name()))
		if err != nil {
			failed = append(failed, fmt.Errorf("the stop records %s cannot be listed, so which reports they hold is unknown: %w", filepath.Join(root, key.Name()), err))
			continue
		}
		for _, entry := range entries {
			id, resolved := strings.CutSuffix(entry.Name(), resolvedSuffix)
			if !resolved || strings.HasPrefix(id, ".") {
				continue
			}
			stop := openStop{key: key.Name(), id: id}
			if w.readResolution(stop.path(w.dir, name)+resolvedSuffix) != nil {
				continue
			}
			raw, err := w.readFile(stop.path(w.dir, name))
			if errors.Is(err, os.ErrNotExist) {
				// As stopState reads it: a record listed and gone was never
				// written, the resolution beside it being no record.
				continue
			}
			if err == nil {
				if err = json.Unmarshal(raw, &stop.record); err != nil {
					err = unknownRecord(stop.path(w.dir, name), err)
				}
			}
			if err != nil {
				failed = append(failed, fmt.Errorf("the resolved stop record %s cannot be read, so which report it holds is unknown: %w", stop.path(w.dir, name), err))
				continue
			}
			if report := stop.reportOf(); report != "" {
				held[report] = stop
			}
		}
	}
	return held, errors.Join(failed...)
}
