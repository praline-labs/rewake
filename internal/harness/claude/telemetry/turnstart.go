package telemetry

import (
	"os"
	"path/filepath"
	"strconv"
)

// The start of a Claude Code turn, for binding a `rewake pending` mark to the
// turn it was made in. The harness says a turn starts by running the
// UserPromptSubmit hook — for typed input and for mail delivered through the
// inbox socket alike — and that hook runs in the background, so it may finish
// after the agent has already run `rewake pending`. What it records is
// therefore its process's start on the boot clock, taken before anything else,
// and what is read back is the latest reading recorded: a late hook of the
// current turn records an earlier time than the mark's and cannot pass it,
// and a turn that started after the mark always records a later one.
//
// No lock: the hook sits in front of every prompt and must not wait. Each
// reading is a file of its own in a directory, named by the reading and never
// overwritten, and the latest is the largest name. Two hooks recording at once
// cannot leave the older reading on top, which a single file replaced by
// renames could: a writer's check after its own rename does not see an older
// writer renaming after it.

// TurnStartPath is where a session's turn starts are recorded: beside its
// telemetry socket, so it names the same run and goes with it.
func TurnStartPath(socket string) string { return socket + ".turn" }

// RecordTurnStart adds a reading, and drops the older ones it finds so the
// directory stays a handful of entries.
func RecordTurnStart(socket string, started int64) {
	if socket == "" || started <= 0 {
		return
	}
	path := TurnStartPath(socket)
	if err := os.MkdirAll(path, 0o700); err != nil {
		return
	}
	name := strconv.FormatInt(started, 10)
	file, err := os.OpenFile(filepath.Join(path, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		_ = file.Close()
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if value, err := strconv.ParseInt(entry.Name(), 10, 64); err == nil && value < started {
			_ = os.Remove(filepath.Join(path, entry.Name()))
		}
	}
}

// ReadTurnStart is the latest recorded turn start, or 0 when none is known.
func ReadTurnStart(path string) int64 {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	var latest int64
	for _, entry := range entries {
		if value, err := strconv.ParseInt(entry.Name(), 10, 64); err == nil && value > latest {
			latest = value
		}
	}
	return latest
}
