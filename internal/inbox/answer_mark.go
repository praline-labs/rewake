package inbox

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
)

// A waiting send's mark says when it was last touched in its content, as a
// reading of the boot clock, and not only by its modification time. The mark
// is touched by the send and judged by the serving process, and a file's time
// is the wall clock, which can be stepped by seconds at any moment: a step
// forward between a touch and a look made a live send look gone, and its
// answer was announced to the agent as well as handed to the send. The boot
// clock is one for every process and never stepped. A mark with no readable
// reading — written by an earlier build, or caught between the truncation and
// the write of a touch — is judged by its time, as before.

// markContent is what a touch writes: the reading and a newline, so a read
// that caught the write half done is told apart from a whole one.
func markContent(boot int64) []byte {
	return []byte(strconv.FormatInt(boot, 10) + "\n")
}

// touchMark writes the current reading into a mark that exists. It never
// creates one: a report taken as the answer removes the mark while the send is
// still printing it, and a mark written back then would hide that report from
// everyone else.
func touchMark(path string) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return
	}
	_, _ = file.Write(markContent(boottime.Now()))
	_ = file.Close()
}

// markFresh reports whether the send that owns a mark touched it within
// answeringFresh. The clock is read after the mark: the heartbeat touches it
// without the mailbox lock, and a reading taken before the file could be older
// than the touch it then finds, which made a live send look gone. An error says
// a mark is there and could not be read: a live send may own it.
func markFresh(path string) (bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	boot := boottime.Now()
	if text, whole := strings.CutSuffix(string(raw), "\n"); whole && boot != 0 {
		if touched, err := strconv.ParseInt(text, 10, 64); err == nil && touched > 0 {
			age := boot - touched
			// Negative is a reading from before a reboot: not a live send.
			return age >= 0 && time.Duration(age) < answeringFresh, nil
		}
	}
	// legacy(rewake <2026-09-26): marks of earlier builds are empty and judged by their mtime; remove when no session started by such a build is registered, keeping the mtime for a half-written touch or no boot clock
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return time.Since(info.ModTime()) < answeringFresh, nil
}
