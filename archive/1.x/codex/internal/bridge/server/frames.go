package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sync/atomic"
)

// What comes in is bounded before it is parsed
// (docs/mail-bridge-server.md#the-answer): a frame is one line of at most
// maxFrame bytes, and a request id one the answer can carry within its own
// room in the framing.
const (
	// maxFrame holds the words' own 32 KiB escaped, with room to spare.
	maxFrame = 256 << 10
	// maxID is the most an id may take encoded.
	maxID = 128
)

// errFrameTooLong is a line past maxFrame, read to its end and discarded.
var errFrameTooLong = errors.New("a frame longer than a frame can be")

// frames reads newline-delimited frames.
type frames struct{ reader *bufio.Reader }

func newFrames(in io.Reader) *frames { return &frames{reader: bufio.NewReaderSize(in, 64<<10)} }

// next returns the next non-empty frame, or errFrameTooLong for one past the
// bound, after which reading goes on at the following line. A last line
// without its newline is a frame all the same.
func (f *frames) next() ([]byte, error) {
	for {
		line, long, err := f.line()
		if long {
			return nil, errFrameTooLong
		}
		if trimmed := bytes.TrimRight(line, "\r\n"); len(bytes.TrimSpace(trimmed)) > 0 {
			return trimmed, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// line reads to the next newline, keeping no more than one frame can hold.
func (f *frames) line() ([]byte, bool, error) {
	var line []byte
	long := false
	for {
		chunk, err := f.reader.ReadSlice('\n')
		if !long && len(line)+len(chunk) > maxFrame+1 {
			long, line = true, nil
		}
		if !long {
			line = append(line, chunk...)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, long, err
	}
}

// validID says whether an answer can carry this id: an integer, or a string
// of at most maxID bytes encoded. An absent id is a notification.
func validID(raw json.RawMessage) bool {
	if len(raw) == 0 || len(raw) > maxID {
		return false
	}
	if raw[0] == '"' {
		var text string
		return json.Unmarshal(raw, &text) == nil
	}
	for i, c := range raw {
		if c == '-' && i == 0 && len(raw) > 1 {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// input is stdin read apart from the dispatch: frames in order, nil for one
// over the bound, then closed with err set at the end of stdin.
type input struct {
	frames     chan []byte
	dispatched chan struct{}
	err        error
	// backedUp is set while a frame waits for the dispatch; ended once
	// stdin ended.
	backedUp, ended atomic.Bool
}

// inputAhead is how many frames reading runs ahead of the dispatch.
const inputAhead = 16

func (s *Server) read(stdin io.Reader) *input {
	in := &input{frames: make(chan []byte, inputAhead), dispatched: make(chan struct{})}
	go func() {
		defer close(in.frames)
		frames := newFrames(stdin)
		for {
			frame, err := frames.next()
			switch {
			case errors.Is(err, errFrameTooLong):
				frame = nil
			case err != nil:
				if !errors.Is(err, io.EOF) {
					in.err = err
				}
				in.ended.Store(true)
				return
			}
			select {
			case in.frames <- frame:
			default:
				in.backedUp.Store(true)
				in.frames <- frame
				in.backedUp.Store(false)
			}
		}
	}()
	return in
}
