package inbox

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"github.com/praline-labs/rewake/internal/state"
)

// The latest turn start of a run, as the host heard it from the harness
// (docs/v2/stage3-steps-adapters.md, S7): what a pending mark needs to know
// that its turn ended when the end itself was lost. A start recorded after a
// mark's time proves the mark's turn over; one recorded before it, or none,
// proves nothing.
//
// No lock: the host records a start from the reader of the harness's reports,
// which must not wait. Each reading is a file of its own beside the run's read
// clock, named by the reading and never overwritten, and the latest is the
// largest name, so two writers at once cannot leave the older reading on top.

// turnStartsDir is the run's directory of readings.
const turnStartsDir = ".turn-starts"

// RecordTurnStart adds a reading, on the boot clock, and drops the older ones
// it finds so the directory stays a handful of entries. The time is the
// harness's for the start, at or before anything the turn ran.
func RecordTurnStart(dir, name, epoch string, started int64) error {
	path, ok := awaitingPath(dir, name, epoch)
	if !ok || started <= 0 {
		return errors.New("a turn start needs a run and a time")
	}
	if err := state.EnsureSubdir(state.AwaitingPath(dir, name)); err != nil {
		return err
	}
	if err := state.EnsureSubdir(path); err != nil {
		return err
	}
	path = filepath.Join(path, turnStartsDir)
	if err := state.EnsureSubdir(path); err != nil {
		return err
	}
	reading := strconv.FormatInt(started, 10)
	file, err := os.OpenFile(filepath.Join(path, reading), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		err = file.Close()
	} else if errors.Is(err, os.ErrExist) {
		err = nil
	}
	if err != nil {
		return err
	}
	entries, _ := os.ReadDir(path)
	for _, entry := range entries {
		if value, parsed := strconv.ParseInt(entry.Name(), 10, 64); parsed == nil && value < started {
			_ = os.Remove(filepath.Join(path, entry.Name()))
		}
	}
	return nil
}

// LatestTurnStart is the latest recorded turn start of the run, 0 when none
// is recorded. A directory that cannot be read is an error: it may hold a
// start that proves a turn ended.
func LatestTurnStart(dir, name, epoch string) (int64, error) {
	path, ok := awaitingPath(dir, name, epoch)
	if !ok {
		return 0, nil
	}
	entries, err := os.ReadDir(filepath.Join(path, turnStartsDir))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var latest int64
	for _, entry := range entries {
		if value, err := strconv.ParseInt(entry.Name(), 10, 64); err == nil && value > latest {
			latest = value
		}
	}
	return latest, nil
}
