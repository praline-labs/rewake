package inbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/state"
)

// A pending mark says the turn now running is not the end of the work
// (docs/turn-end-recovery.md#pending-marks). Each `rewake pending` call makes
// a mark of its own, named by its run, the boot-clock time At its process
// started, and a random id, so a later mark never writes over an earlier one.
// No turn end removes a mark: the evidence an earlier end's retry needs is
// never destroyed by a later end. A mark stays for its run's life, and goes
// when the run's records are swept (sweepMarks). Being used is no effect: the
// journal of the end records the mark that decided it.
//
// A mark belongs to the turn whose window holds its At: from just after the
// turn's start to its end, inclusive (TurnWindowStart). Marks change only
// under the mailbox lock.

// Mark is one pending mark.
type Mark struct {
	// Name is the file the mark is kept in, which orders marks of the same
	// time without regard to the order of retries.
	Name string `json:"name"`
	At   int64  `json:"at"`
	Text string `json:"text"`
}

type markRecord struct {
	Epoch string `json:"epoch"`
	At    int64  `json:"at"`
	Text  string `json:"text"`
}

// marksPath holds the marks of one run.
func marksPath(dir, name, epoch string) (string, bool) {
	if epoch == "" || strings.ContainsAny(epoch, `/\`) || strings.HasPrefix(epoch, ".") {
		return "", false
	}
	return filepath.Join(pendingDir(dir, name), "marks", epoch), true
}

// MarkName is the file name of a mark made at at under the random id.
func MarkName(at int64, id string) string { return fmt.Sprintf("%020d-%s", at, id) }

// parseMarkName reads back the time a mark's name carries.
func parseMarkName(file string) (int64, bool) {
	at, id, found := strings.Cut(file, "-")
	if !found || len(at) != 20 || !safeID(id) {
		return 0, false
	}
	value, err := strconv.ParseInt(at, 10, 64)
	return value, err == nil && value > 0
}

// MarkPending records a mark of run epoch made at at, under the name its call
// drew: a retry of the same call finds it there and writes nothing. The
// caller holds the mailbox lock. A mark with no time would belong to no turn,
// and is refused.
func MarkPending(dir, name, epoch, file, text string, at int64) error {
	path, ok := marksPath(dir, name, epoch)
	if !ok || at <= 0 {
		return errors.New("a pending mark needs its run and the time it was made")
	}
	if parsed, ok := parseMarkName(file); !ok || parsed != at {
		return fmt.Errorf("%q does not name a pending mark made at %d", file, at)
	}
	for _, directory := range []string{pendingDir(dir, name), filepath.Dir(path), path} {
		if err := state.EnsureSubdir(directory); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(path, file)); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	raw, err := json.Marshal(markRecord{Epoch: epoch, At: at, Text: text})
	if err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(path, file), raw)
}

// MarkWithin answers the mark that decides a turn end of run epoch whose
// window runs from just after start to ended, inclusive: the latest by At,
// then by name. A mark whose name falls in the window and that cannot be read
// is an error naming it: it may be this turn's. The caller holds the mailbox
// lock.
func MarkWithin(dir, name, epoch string, start, ended int64) (Mark, bool, error) {
	path, ok := marksPath(dir, name, epoch)
	if !ok || ended <= 0 {
		return Mark{}, false, nil
	}
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return Mark{}, false, nil
	}
	if err != nil {
		return Mark{}, false, fmt.Errorf("the pending marks %s cannot be listed: %w", path, err)
	}
	var found Mark
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			// A write that never finished.
			continue
		}
		at, ok := parseMarkName(entry.Name())
		if !ok {
			return Mark{}, false, fmt.Errorf("%s does not name a pending mark, so whether it is this turn's is unknown", filepath.Join(path, entry.Name()))
		}
		if at <= start || at > ended {
			continue
		}
		raw, err := state.ReadFile(filepath.Join(path, entry.Name()))
		var record markRecord
		if err == nil {
			err = json.Unmarshal(raw, &record)
		}
		if err == nil && (record.Epoch != epoch || record.At != at || strings.TrimSpace(record.Text) == "") {
			err = errors.New("it does not say what its name says")
		}
		if err != nil {
			return Mark{}, false, fmt.Errorf("the pending mark %s is not readable, and may be this turn's: %w", filepath.Join(path, entry.Name()), err)
		}
		if at > found.At || at == found.At && entry.Name() > found.Name {
			found = Mark{Name: entry.Name(), At: at, Text: record.Text}
		}
	}
	return found, found.Name != "", nil
}

// sweepMarks drops the marks of runs other than the live one: only a run
// retries its own ends.
func sweepMarks(dir, name, live string) {
	root := filepath.Join(pendingDir(dir, name), "marks")
	runs, err := os.ReadDir(root)
	if err != nil {
		return
	}
	kept := keptByStopOf(dir, name)
	for _, run := range runs {
		if run.Name() != live && !kept.keeps(filepath.Join(root, run.Name())) {
			_ = os.RemoveAll(filepath.Join(root, run.Name()))
		}
	}
}
