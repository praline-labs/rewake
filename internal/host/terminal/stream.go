package terminal

// The raw stream (docs/v2/shell.md#how-a-pane-is-fed-the-raw-stream): a
// client that draws the program in an emulator of its own gets a preamble
// built from this grid's emulator and then every byte the emulator takes
// after it. The cut is made in commit, under the grid's lock, between two
// pieces the emulator took whole: a read of the pseudo-terminal is not the
// emulator's boundary, since Write queues and the holder delays a suffix.

// streamQueue bounds the pieces waiting for one client. A client further
// behind is dropped, its stream closed, rather than stop the emulator or
// skip bytes: a stream with a gap draws garbage, and the client attaches
// again for a fresh preamble.
const streamQueue = 4096

// Stream is one client's raw stream.
type Stream struct {
	// Preamble draws the screen as the emulator holds it and sets the modes
	// a key's bytes depend on; it ends with the bytes of a sequence the
	// program has not finished, so the client's parser stands where the
	// emulator's does.
	Preamble []byte
	// Live are the pieces the emulator took after the preamble, in order and
	// never written again; closed when the output is over, when the client
	// fell too far behind, or once Detach is called.
	Live <-chan []byte

	g    *Grid
	live chan []byte
}

// Attach is a raw stream from now on.
func (g *Grid) Attach() *Stream {
	s := &Stream{g: g, live: make(chan []byte, streamQueue)}
	s.Live = s.live
	g.mu.Lock()
	defer g.mu.Unlock()
	s.Preamble = append(g.emu.replay(), g.unfinished.bytes()...)
	if g.ended {
		close(s.live)
		return s
	}
	if g.streams == nil {
		g.streams = map[*Stream]struct{}{}
	}
	g.streams[s] = struct{}{}
	return s
}

// Detach ends the stream; Live is closed after it.
func (s *Stream) Detach() {
	s.g.mu.Lock()
	defer s.g.mu.Unlock()
	s.g.dropLocked(s)
}

func (g *Grid) dropLocked(s *Stream) {
	if _, ok := g.streams[s]; ok {
		delete(g.streams, s)
		close(s.live)
	}
}

// streamLocked hands a piece the emulator just took to every stream. The
// piece is never written again, so the streams share it.
func (g *Grid) streamLocked(p []byte, end bool) {
	if len(p) > 0 {
		g.unfinished.track(p)
		for s := range g.streams {
			select {
			case s.live <- p:
			default:
				g.dropLocked(s)
			}
		}
	}
	if end {
		for s := range g.streams {
			g.dropLocked(s)
		}
	}
}

// unfinishedKept bounds the bytes kept of an unfinished sequence. Only a
// string — a long copy — outgrows it; its head is kept, so a late client's
// parser is inside a string too and takes the rest as the string's, losing
// only that one string rather than drawing its tail as text.
const unfinishedKept = 4 << 10

// unfinished follows the program's bytes as a terminal's parser does and
// keeps those since it last stood in the ground state: a pty read has no
// sequence boundary, and a client attached after ESC [ ? 2004 and before
// the h would take the h for text and keep paste off.
type unfinished struct {
	state parseState
	kept  []byte
	// utf8 counts the continuation bytes a character in text still needs.
	utf8 int
}

type parseState uint8

const (
	ground parseState = iota
	escape            // after ESC, maybe with intermediates
	csi
	text       // a string: OSC, DCS, SOS, PM, APC
	textEscape // ESC inside a string: ST, or a new sequence
)

func (u *unfinished) bytes() []byte { return append([]byte(nil), u.kept...) }

func (u *unfinished) track(p []byte) {
	for _, b := range p {
		u.step(b)
		switch {
		case u.state == ground && u.utf8 == 0:
			u.kept = u.kept[:0]
		case len(u.kept) < unfinishedKept:
			u.kept = append(u.kept, b)
		}
	}
}

func (u *unfinished) step(b byte) {
	if b == 0x18 || b == 0x1a {
		// CAN and SUB cancel any sequence.
		u.state, u.utf8 = ground, 0
		return
	}
	switch u.state {
	case ground:
		u.ground(b)
	case escape:
		switch {
		case b == 0x1b:
			u.restart()
		case b >= 0x20 && b <= 0x2f:
			// An intermediate: the final byte is still to come.
		case b == '[':
			u.state = csi
		case b == ']' || b == 'P' || b == 'X' || b == '^' || b == '_':
			u.state = text
		case b >= 0x30 && b <= 0x7e:
			u.state = ground
		}
	case csi:
		switch {
		case b == 0x1b:
			u.restart()
		case b >= 0x40 && b <= 0x7e:
			u.state = ground
		}
	case text:
		switch b {
		case 0x1b:
			u.state = textEscape
		case 0x07:
			u.state = ground
		}
	case textEscape:
		if b == '\\' {
			u.state = ground
		} else {
			// ESC ended the string and began a new sequence.
			u.kept = append(u.kept[:0], 0x1b)
			u.state = escape
			u.step(b)
		}
	}
}

// restart is an ESC that abandons the sequence so far and begins another.
func (u *unfinished) restart() {
	u.kept = u.kept[:0]
	u.state = escape
}

func (u *unfinished) ground(b byte) {
	switch {
	case b == 0x1b:
		u.state, u.utf8 = escape, 0
	case b >= 0xc0 && b < 0xf8:
		u.utf8 = 1
		if b >= 0xe0 {
			u.utf8 = 2
		}
		if b >= 0xf0 {
			u.utf8 = 3
		}
		u.kept = u.kept[:0]
	case b >= 0x80 && b < 0xc0 && u.utf8 > 0:
		u.utf8--
	default:
		u.utf8 = 0
	}
}
