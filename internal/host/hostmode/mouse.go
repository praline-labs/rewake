package hostmode

import (
	"bytes"
	"strconv"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// The bits of a mouse report's button code.
const (
	mouseButtons   = 3  // the button, 3 meaning none or a release
	mouseModifiers = 28 // Shift 4, Meta 8, Control 16
	mouseMotion    = 32
	mouseWheel     = 64
)

// mouseEvent is one mouse report from the person's terminal: its button code
// as SGR gives it, the cell from 1, and whether a button was let go.
type mouseEvent struct {
	code     int
	col, row int
	release  bool
}

// parseMouse reads a report in the SGR form (CSI < b ; x ; y M or m), which
// the host asks the person's terminal for, and the X10 form (CSI M and three
// bytes) of a terminal that does not know SGR.
func parseMouse(token []byte) (mouseEvent, bool) {
	if len(token) == 6 && bytes.HasPrefix(token, []byte("\x1b[M")) {
		code := int(token[3]) - 32
		return mouseEvent{
			code: code, col: int(token[4]) - 32, row: int(token[5]) - 32,
			release: code&mouseButtons == 3 && code&(mouseMotion|mouseWheel) == 0,
		}, true
	}
	if len(token) < 9 || !bytes.HasPrefix(token, []byte("\x1b[<")) {
		return mouseEvent{}, false
	}
	final := token[len(token)-1]
	if final != 'M' && final != 'm' {
		return mouseEvent{}, false
	}
	fields := bytes.Split(token[3:len(token)-1], []byte(";"))
	if len(fields) != 3 {
		return mouseEvent{}, false
	}
	var n [3]int
	for i, f := range fields {
		v, err := strconv.Atoi(string(f))
		if err != nil || v < 0 {
			return mouseEvent{}, false
		}
		n[i] = v
	}
	return mouseEvent{code: n[0], col: n[1], row: n[2], release: final == 'm'}, true
}

// press says the event is a button going down, the wheel included.
func (m mouseEvent) press() bool { return !m.release && m.code&mouseMotion == 0 }

// leftClick says the event is the left button going down.
func (m mouseEvent) leftClick() bool {
	return m.press() && m.code&(mouseButtons|mouseWheel) == 0
}

// encode is the event as the program with modes asks to hear it, at the cell
// (col, row) of its own screen, or nothing when it did not ask for it: its
// tracking mode filters what is reported (9 presses, 1000 releases too, 1002
// motion with a button down, 1003 all motion), 1006 chooses the SGR form.
func (m mouseEvent) encode(modes terminal.Modes, col, row int) []byte {
	motion := m.code&mouseMotion != 0
	switch modes.Mouse {
	case terminal.MouseOff:
		return nil
	case terminal.MouseX10:
		if !m.press() {
			return nil
		}
	case terminal.MouseNormal:
		if motion {
			return nil
		}
	case terminal.MouseButton:
		if motion && m.code&mouseButtons == 3 {
			return nil
		}
	}
	code := m.code
	if modes.Mouse == terminal.MouseX10 {
		// X10 tracking reports no modifiers.
		code &^= mouseModifiers
	}
	if modes.MouseSGR {
		final := byte('M')
		if m.release {
			final = 'm'
		}
		return append([]byte("\x1b[<"+strconv.Itoa(code)+";"+strconv.Itoa(col)+";"+strconv.Itoa(row)), final)
	}
	if m.release {
		// The legacy form names no button on a release.
		code = code&^mouseButtons | 3
	}
	// A byte carries each value plus 32: a cell past 223 cannot be said.
	if code+32 > 0xff || col+32 > 0xff || row+32 > 0xff {
		return nil
	}
	return []byte{0x1b, '[', 'M', byte(code + 32), byte(col + 32), byte(row + 32)}
}
