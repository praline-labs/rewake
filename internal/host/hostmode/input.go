package hostmode

import (
	"bytes"
	"time"
)

const (
	// keyHold is how long an escape sequence cut by the end of a read waits
	// for the rest: long enough for the next read of the same key, short
	// enough that a lone Escape still feels immediate.
	keyHold = 50 * time.Millisecond
	// prefixLapse is how long a typed prefix waits for its command.
	prefixLapse = 2 * time.Second
	// stringLimit bounds an OSC, DCS or APC string in the input: one longer is
	// passed on as it stands rather than held without end.
	stringLimit = 4096
)

// The prefix in the forms a terminal may send Ctrl+\ as but the kitty
// protocol's: the legacy byte and xterm's modifyOtherKeys.
var prefixes = [][]byte{{0x1c}, []byte("\x1b[27;5;92~")}

// prefixCode is the key code of the prefix's backslash.
const prefixCode = '\\'

var (
	pasteStart = []byte("\x1b[200~")
	pasteEnd   = []byte("\x1b[201~")
)

// action is what one piece of input asks of the host: bytes for the shown
// program, a mouse report, a paste's start or end, or a command typed after
// the prefix.
type action struct {
	send []byte
	// mouse is a report the host routes: to the bar, or re-encoded to the
	// shown program in the form it asked for.
	mouse *mouseEvent
	// paste is pasteOpens or pasteCloses at the person's terminal's paste
	// brackets, which reach the program only when it asked for them.
	paste   int
	command byte
}

const (
	pasteOpens = 1 + iota
	pasteCloses
)

// input turns the person's keystrokes into actions. Its state survives a
// read cut at any offset: a sequence cut by the end of a read is held, a
// pending prefix and an open paste carry to the next read.
type input struct {
	held      []byte
	heldSince time.Time
	pending   bool
	pendingAt time.Time
	paste     bool
	// taken is the code of the last key the host took after the prefix in
	// the CSI u form, whose release is the host's too.
	taken int
}

// feed takes one read.
func (in *input) feed(data []byte, now time.Time) []action {
	if len(in.held) == 0 && len(data) > 0 {
		in.heldSince = now
	}
	in.held = append(in.held, data...)
	var out []action
	for len(in.held) > 0 {
		n := tokenLength(in.held)
		if n == 0 {
			break
		}
		out = in.take(out, in.held[:n], now)
		in.held = in.held[n:]
		in.heldSince = now
	}
	return in.lapse(out, now)
}

// tick passes on a held tail that waited keyHold and drops a prefix that
// waited prefixLapse.
func (in *input) tick(now time.Time) []action {
	var out []action
	if len(in.held) > 0 && now.Sub(in.heldSince) >= keyHold {
		out = in.take(out, in.held, now)
		in.held = nil
	}
	return in.lapse(out, now)
}

// deadline is when tick has something to do, zero when nothing waits.
func (in *input) deadline() time.Time {
	var at time.Time
	if len(in.held) > 0 {
		at = in.heldSince.Add(keyHold)
	}
	if in.pending {
		if lapse := in.pendingAt.Add(prefixLapse); at.IsZero() || lapse.Before(at) {
			at = lapse
		}
	}
	return at
}

func (in *input) lapse(out []action, now time.Time) []action {
	if in.pending && now.Sub(in.pendingAt) >= prefixLapse {
		in.pending = false
	}
	return merge(out)
}

// take routes one whole token.
func (in *input) take(out []action, token []byte, now time.Time) []action {
	token = bytes.Clone(token)
	switch {
	case in.paste && bytes.Equal(token, pasteEnd):
		in.paste = false
		return append(out, action{paste: pasteCloses})
	case in.paste:
		return append(out, action{send: token})
	case bytes.Equal(token, pasteStart):
		in.paste, in.pending = true, false
		return append(out, action{paste: pasteOpens})
	}
	if m, ok := parseMouse(token); ok {
		return append(out, action{mouse: &m})
	}
	if focusReport(token) {
		return append(out, action{send: token})
	}
	if in.pending {
		// A release, the prefix's own among them, leaves the prefix waiting.
		if keyRelease(token) {
			return out
		}
		in.pending = false
		if isPrefix(token) {
			return append(out, action{send: token})
		}
		in.taken = 0
		if k, ok := parseKitty(token); ok {
			in.taken = k.code
		}
		if c, ok := command(token); ok {
			return append(out, action{command: c})
		}
		// Any other key cancels the prefix and is dropped.
		return out
	}
	if isPrefix(token) {
		in.pending, in.pendingAt = true, now
		return out
	}
	if k, ok := parseKitty(token); ok && k.event == eventRelease && (k.code == prefixCode || k.code == in.taken) {
		// The release of a key the host took is the host's, as its press was.
		in.taken = 0
		return out
	}
	return append(out, action{send: token})
}

// merge joins neighboring plain sends, so a read becomes one write.
func merge(actions []action) []action {
	var out []action
	for _, a := range actions {
		if last := len(out) - 1; last >= 0 && a.send != nil && out[last].send != nil {
			out[last].send = append(out[last].send, a.send...)
			continue
		}
		out = append(out, a)
	}
	return out
}

// isPrefix says the token presses Ctrl+\ in any form: a repeat counts, as a
// held key repeats the legacy byte.
func isPrefix(token []byte) bool {
	for _, p := range prefixes {
		if bytes.Equal(token, p) {
			return true
		}
	}
	k, ok := parseKitty(token)
	return ok && k.code == prefixCode && k.mods == modCtrl && k.event != eventRelease
}

// command is the command key the token presses, as text or, under the kitty
// protocol's report of all keys, as CSI u with no modifier.
func command(token []byte) (byte, bool) {
	if len(token) == 1 && commandKey(token[0]) {
		return token[0], true
	}
	k, ok := parseKitty(token)
	if !ok || k.mods != 0 || k.event == eventRelease || k.code > 0x7f || !commandKey(byte(k.code)) {
		return 0, false
	}
	return byte(k.code), true
}

func commandKey(b byte) bool {
	return b == 'n' || b == 'p' || b == 'm' || b == 's' || (b >= '0' && b <= '9')
}

func focusReport(token []byte) bool {
	return bytes.Equal(token, []byte("\x1b[I")) || bytes.Equal(token, []byte("\x1b[O"))
}

// tokenLength is the length of the whole token data starts with, or 0 when
// data begins a sequence it does not finish: one byte of text or control, or
// one escape sequence.
func tokenLength(data []byte) int {
	if data[0] != 0x1b {
		return 1
	}
	if len(data) == 1 {
		return 0
	}
	switch data[1] {
	case '[':
		return csiLength(data)
	case 'O':
		if len(data) < 3 {
			return 0
		}
		return 3
	case ']', 'P', '_', '^', 'X':
		return stringLength(data)
	}
	// Alt and a key.
	return 2
}

// csiLength reads CSI parameters and intermediates up to a final byte; CSI M
// with no parameters is an X10 mouse report and takes three bytes more.
func csiLength(data []byte) int {
	for i := 2; i < len(data); i++ {
		b := data[i]
		switch {
		case b >= 0x20 && b <= 0x3f:
			continue
		case b >= 0x40 && b <= 0x7e:
			if b == 'M' && i == 2 {
				if len(data) < 6 {
					return 0
				}
				return 6
			}
			return i + 1
		default:
			// Not a CSI after all: pass what was read and go on.
			return i
		}
	}
	return 0
}

// stringLength reads a control string up to BEL or ST.
func stringLength(data []byte) int {
	for i := 2; i < len(data); i++ {
		switch {
		case data[i] == 0x07:
			return i + 1
		case data[i] == 0x1b && i+1 < len(data) && data[i+1] == '\\':
			return i + 2
		}
	}
	if len(data) >= stringLimit {
		return len(data)
	}
	return 0
}
