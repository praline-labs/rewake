package terminal

import (
	"bytes"
	"strconv"
)

// oscLimit bounds an OSC string kept whole: a copy of a long selection is
// tens of kilobytes, and one longer is dropped rather than kept without end.
const oscLimit = 1 << 20

// oscFilter drops the bytes above ASCII inside an OSC string, and hands each
// whole OSC string, as the program wrote it, to done. The library's parser
// (x/ansi v0.11.7 and v0.11.8) ends an OSC string at the byte 0x9C, the
// 8-bit ST, even inside a UTF-8 character, so Claude Code's title
// "✳ Claude Code" (✳ is E2 9C B3) ended early and the rest was drawn as text
// over its logo. A program in a UTF-8 locale never sends a bare 8-bit ST;
// what the emulator loses is only the non-ASCII characters, and no OSC it
// acts on needs them. What the host passes on — a title, a notification —
// is taken from done, whole.
type oscFilter struct {
	esc, osc bool
	// buf is the OSC string so far; long says it outgrew oscLimit.
	buf  []byte
	long bool
	done func(payload []byte)
}

func (f *oscFilter) filter(p []byte) []byte {
	out := p
	copied := false
	for i, b := range p {
		drop := false
		switch {
		case f.osc:
			switch b {
			case 0x07:
				f.end(true)
			case 0x18, 0x1a:
				f.end(false)
			case 0x1b:
				f.end(true)
				f.esc = true
			default:
				if len(f.buf) < oscLimit {
					f.buf = append(f.buf, b)
				} else {
					f.long = true
				}
			}
			drop = b >= 0x80
		case f.esc:
			f.esc = false
			f.osc = b == ']'
			f.buf, f.long = f.buf[:0], false
		case b == 0x1b:
			f.esc = true
		}
		switch {
		case drop && !copied:
			out, copied = append([]byte(nil), p[:i]...), true
		case !drop && copied:
			out = append(out, b)
		}
	}
	return out
}

// end closes the OSC string, handing it on when it finished rather than was
// canceled.
func (f *oscFilter) end(finished bool) {
	f.osc = false
	if finished && !f.long && f.done != nil {
		f.done(bytes.Clone(f.buf))
	}
}

// oscSeen keeps what an OSC string says for the host mode
// (docs/v2/host-mode.md#rendering-from-the-emulators): a title (0 and 2) as
// the screen's, a copy (52, not its query) and a notification (9 but its
// progress 9;4, 99, 777) as events for the person's terminal.
func (v *vtEmulator) oscSeen(payload []byte) {
	command, rest, _ := bytes.Cut(payload, []byte(";"))
	n, err := strconv.Atoi(string(command))
	if err != nil {
		return
	}
	switch {
	case n == 0 || n == 2:
		v.title = string(rest)
	case n == 52 && !bytes.HasSuffix(rest, []byte(";?")),
		n == 9 && !bytes.HasPrefix(rest, []byte("4;")),
		n == 99, n == 777:
		v.events = append(v.events, Event{OSC: string(payload)})
	}
}
