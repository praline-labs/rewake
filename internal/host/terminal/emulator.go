package terminal

import (
	"image/color"
	"io"
)

// emulator is the one seam the terminal library sits behind
// (docs/v2/host-mode.md): bytes in, a resize, the
// screen out, and what it answers the program. Swapping the library touches
// this package alone. Every method but replies runs under the terminal's
// lock; replies is read by its own goroutine, which never blocks the writer.
type emulator interface {
	// write feeds the program's output. Answers to the program's queries
	// come out of replies while it runs.
	write(p []byte)
	resize(size Size)
	// defaults sets the colors the program's queries of the default
	// foreground and background are answered with; nil keeps one.
	defaults(fg, bg color.Color)
	// look reads the whole screen, the cursor and the modes.
	look() look
	// replay is the screen and the input modes as bytes for a fresh
	// terminal (replay.go); unlike look it leaves the events.
	replay() []byte
	replies() io.Reader
	// stopReplies ends replies, so its reader returns.
	stopReplies() error
}

// newEmulator makes the library's emulator.
var newEmulator = newVT
