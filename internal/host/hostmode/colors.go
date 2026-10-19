package hostmode

import (
	"bytes"
	"regexp"
	"time"
)

// colorWait bounds the wait for the person's terminal to name its colors. A
// terminal answers the device attributes asked last whatever it knows, so
// the wait ends with that answer and the bound is only for one that answers
// nothing.
const colorWait = 300 * time.Millisecond

// colorQuery asks the person's terminal for its default foreground and
// background, then for its device attributes.
const colorQuery = "\x1b]10;?\x1b\\\x1b]11;?\x1b\\\x1b[c"

var (
	colorReply      = regexp.MustCompile(`\x1b\]1([01]);([^\x07\x1b]*)(?:\x07|\x1b\\)`)
	attributesReply = regexp.MustCompile(`\x1b\[\?[0-9;]*c`)
	// lateReply is one of either as a whole token.
	lateReply = regexp.MustCompile(`^(?:\x1b\]1[01];[^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[\?[0-9;]*c)$`)
)

// askColors writes the query and reads the answers off keys, the person's
// terminal's input: the colors named, and what else came, which is the
// person's typing.
func askColors(write func([]byte) error, keys <-chan []byte) (fg, bg string, rest []byte) {
	if write([]byte(colorQuery)) != nil {
		return "", "", nil
	}
	var got []byte
	deadline := time.After(colorWait)
	for !attributesReply.Match(got) {
		select {
		case p := <-keys:
			got = append(got, p...)
			continue
		case <-deadline:
		}
		break
	}
	for _, m := range colorReply.FindAllSubmatch(got, -1) {
		if string(m[1]) == "0" {
			fg = string(m[2])
		} else {
			bg = string(m[2])
		}
	}
	rest = colorReply.ReplaceAll(got, nil)
	rest = attributesReply.ReplaceAll(rest, nil)
	return fg, bg, bytes.Clone(rest)
}

// isReply says the token is the person's terminal answering the host's query
// after the wait: the programs' queries are answered by their emulators, so
// no answer from the person's terminal is ever a program's.
func isReply(token []byte) bool { return lateReply.Match(token) }
