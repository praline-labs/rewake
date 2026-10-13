package terminal

import (
	"slices"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// The emulator ends every glyph cluster at the end of each write: a mark, a
// variation selector or a joiner that arrives in the next read becomes a
// cell of its own, and "e" then U+0301 split across two reads is two cells
// where it is one (both library versions probed, docs/v2/agent-deck-ideas.md).
// So the last cluster of a read is held back and written with the next read,
// or alone once holdFor passes with nothing after it.

// holdWindow is the most bytes looked at for the cluster to hold. A cluster
// longer than it is written at once rather than held, so what is held stays
// one cluster and never a run of text.
const holdWindow = 64

// heldFrom is where the bytes held back begin: the start of the last glyph
// cluster when data ends in text, len(data) when it ends in a control byte,
// which no cluster crosses. The bytes of an unfinished escape sequence may be
// held too; that only delays them, in order.
func heldFrom(data []byte) int {
	lo := max(0, len(data)-holdWindow)
	start := len(data)
	for start > lo && printable(data[start-1]) {
		start--
	}
	truncated := start == lo && lo > 0 && printable(data[lo-1])
	for start < len(data) && !utf8.RuneStart(data[start]) {
		start++
	}
	if start == len(data) {
		return len(data)
	}
	last := start
	for offset := start; offset < len(data); {
		cluster, _ := ansi.FirstGraphemeCluster(data[offset:], ansi.GraphemeWidth)
		if len(cluster) == 0 {
			break
		}
		last = offset
		offset += len(cluster)
	}
	if truncated && last == start {
		return len(data)
	}
	return last
}

// holder keeps the bytes held back between reads.
type holder struct{ held []byte }

// take is what of the held bytes and chunk may be written now; the rest is
// held.
func (h *holder) take(chunk []byte) []byte {
	data := chunk
	if len(h.held) > 0 {
		// A fresh array: what is returned is written while the rest is held.
		data = slices.Concat(h.held, chunk)
	}
	cut := heldFrom(data)
	h.held = append([]byte(nil), data[cut:]...)
	return data[:cut]
}

// release is everything held, which may be written now.
func (h *holder) release() []byte {
	p := h.held
	h.held = nil
	return p
}

func (h *holder) holding() bool { return len(h.held) > 0 }

// printable is a byte that may belong to text: not a C0 control and not DEL.
// Bytes of a multi-byte character are all above them.
func printable(b byte) bool {
	return b >= 0x20 && b != 0x7f
}
