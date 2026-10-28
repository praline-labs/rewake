package spacehost

import (
	"encoding/json"
	"fmt"
	"image/color"
	"net"
	"os"
	"time"

	"github.com/praline-labs/rewake/internal/core/screen"
	"github.com/praline-labs/rewake/internal/host/terminal"
)

// snapshot serves one client of the snapshot mode: the worker's screen as
// the emulator holds it, one JSON line of a screen.Frame, then the end. It
// is what rewake screen shows of a worker when no shell answers
// (docs/screen.md); the client sends no frame, so the worker's size stays.
func (r *relay) snapshot(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	frame := frameOf(r.grid.Snapshot())
	// The person's default colors, as the shell gave them to this host; the
	// palette is not passed to a host, so indexed colors are xterm's.
	fg, bg := terminal.ParseDefaults(os.Getenv(terminal.DefaultsEnv))
	frame.Fg, frame.Bg = colorHex(fg), colorHex(bg)
	data, err := frame.Marshal()
	if err != nil {
		// The client reads a line either way; a refusal is one it can show.
		data, _ = json.Marshal(map[string]string{"error": err.Error()})
	}
	_ = conn.SetWriteDeadline(time.Now().Add(clientWrite))
	_, _ = conn.Write(append(data, '\n'))
}

// frameOf is a snapshot as the screen model; a default color stays empty.
func frameOf(snap terminal.Snapshot) *screen.Frame {
	frame := &screen.Frame{Source: screen.SourceHost, Cols: snap.Size.Cols, Rows: snap.Size.Rows}
	if snap.Cursor.Visible {
		frame.Cursor = &screen.Cursor{X: snap.Cursor.Col, Y: snap.Cursor.Row}
	}
	for _, row := range snap.Rows {
		line := make([]screen.Run, 0, len(row))
		for _, run := range row {
			line = append(line, screen.Run{
				T: run.Text, W: run.Cols, Fg: hexOf(run.Fg), Bg: hexOf(run.Bg), A: int(run.Attrs),
			})
		}
		frame.Lines = append(frame.Lines, line)
	}
	return frame
}

// colorHex is a color as "#rrggbb"; nil is empty.
func colorHex(c color.Color) string {
	if c == nil {
		return ""
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// hexOf is a run's color as "#rrggbb"; the default is empty.
func hexOf(c terminal.Color) string {
	switch c.Kind {
	case terminal.RGB:
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	case terminal.Indexed:
		r, g, b := indexed(c.Index)
		return fmt.Sprintf("#%02x%02x%02x", r, g, b)
	}
	return ""
}

// basic are xterm's sixteen colors.
var basic = [16][3]uint8{
	{0, 0, 0},
	{205, 0, 0},
	{0, 205, 0},
	{205, 205, 0},
	{0, 0, 238},
	{205, 0, 205},
	{0, 205, 205},
	{229, 229, 229},
	{127, 127, 127},
	{255, 0, 0},
	{0, 255, 0},
	{255, 255, 0},
	{92, 92, 255},
	{255, 0, 255},
	{0, 255, 255},
	{255, 255, 255},
}

// indexed is xterm's palette entry: the sixteen, the 6x6x6 cube, the greys.
func indexed(i uint8) (uint8, uint8, uint8) {
	switch {
	case i < 16:
		return basic[i][0], basic[i][1], basic[i][2]
	case i < 232:
		level := func(n uint8) uint8 {
			if n == 0 {
				return 0
			}
			return 55 + 40*n
		}
		n := i - 16
		return level(n / 36), level(n / 6 % 6), level(n % 6)
	}
	v := 8 + 10*(i-232)
	return v, v, v
}
