package mirror

import "unicode/utf8"

// cutMark stands where a clipped text lost its middle.
const cutMark = "\n… cut …\n"

// clip is a text kept within a bound of its serialized bytes: whole, or its
// head and its tail with cutMark between. Its size is counted as the text
// grows, never by re-reading it: a streamed reply is appended to thousands
// of times.
//
// The tail is allowed to grow to half the bound before it is cut back to a
// quarter, so a long stream re-sends the item once per quarter of the bound
// rather than on every delta.
type clip struct {
	head, tail string
	headSize   int
	tailSize   int
	cut        bool
	max        int
}

func newClip(bound int, text string) clip {
	c := clip{max: bound}
	c.set(text)
	return c
}

// String is the text as the face draws it.
func (c *clip) String() string {
	if !c.cut {
		return c.head
	}
	return c.head + cutMark + c.tail
}

// size is the text's serialized size, within max.
func (c *clip) size() int {
	if !c.cut {
		return c.headSize
	}
	return c.headSize + jsonSize(cutMark) + c.tailSize
}

// set replaces the text.
func (c *clip) set(text string) {
	c.head, c.tail, c.tailSize, c.cut = text, "", 0, false
	c.headSize = jsonSize(text)
	if c.headSize > c.max {
		c.head, c.headSize = prefixWithin(text, c.max/2)
		c.tail, c.tailSize = suffixWithin(text, c.max/4)
		c.cut = true
	}
}

// add appends text, and says whether the result is still the old text with
// text after it; when it is not, the text was cut and the whole of it is to
// be sent again.
func (c *clip) add(text string) bool {
	size := jsonSize(text)
	if !c.cut {
		if c.headSize+size <= c.max {
			c.head += text
			c.headSize += size
			return true
		}
		c.set(c.head + text)
		return false
	}
	c.tail += text
	c.tailSize += size
	if c.tailSize <= c.max/2-jsonSize(cutMark) {
		return true
	}
	c.tail, c.tailSize = suffixWithin(c.tail, c.max/4)
	return false
}

// runeSize is what one rune takes inside a JSON string, counted as the
// largest encoding/json writes it: control characters and invalid bytes as
// \uXXXX, the quote and the backslash escaped.
func runeSize(r rune, width int) int {
	switch {
	case r == '"' || r == '\\' || r == '\n' || r == '\r' || r == '\t':
		return 2
	case r < 0x20, r == utf8.RuneError && width == 1, r == ' ', r == ' ':
		return 6
	default:
		return width
	}
}

// jsonSize is text's size inside a JSON string, without the quotes.
func jsonSize(text string) int {
	size := 0
	for i := 0; i < len(text); {
		r, width := utf8.DecodeRuneInString(text[i:])
		size += runeSize(r, width)
		i += width
	}
	return size
}

// prefixWithin is the longest head of text whose serialized size is within
// budget, cut between runes, with that size.
func prefixWithin(text string, budget int) (string, int) {
	size := 0
	for i := 0; i < len(text); {
		r, width := utf8.DecodeRuneInString(text[i:])
		next := runeSize(r, width)
		if size+next > budget {
			return text[:i], size
		}
		size += next
		i += width
	}
	return text, size
}

// suffixWithin is the longest tail of text within budget, as prefixWithin.
func suffixWithin(text string, budget int) (string, int) {
	size := 0
	for i := len(text); i > 0; {
		r, width := utf8.DecodeLastRuneInString(text[:i])
		next := runeSize(r, width)
		if size+next > budget {
			return text[i:], size
		}
		size += next
		i -= width
	}
	return text, size
}
