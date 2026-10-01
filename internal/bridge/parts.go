package bridge

import (
	"encoding/json"
	"unicode/utf8"
)

// EncodedSize is the size of a result carrying this stdout and stderr, as the
// server will encode it: each as a JSON string, with its escapes, plus room
// for the server's framing. HTML escaping is counted, since it only ever makes
// a string longer.
func EncodedSize(stdout, stderr string) int {
	out, _ := json.Marshal(stdout)
	errOut, _ := json.Marshal(stderr)
	return len(out) + len(errOut) + resultOverhead
}

// Fits says whether a result is inside ResultCap.
func Fits(stdout, stderr string) bool { return EncodedSize(stdout, stderr) <= ResultCap }

// Cut returns where the part of text starting at start ends: the furthest
// UTF-8 boundary at most BodyCap bytes on for which fits holds. It returns
// start when not even one character fits, which the caller refuses rather
// than emitting a result over the bound.
func Cut(text string, start int, fits func(end int) bool) int {
	limit := min(len(text), start+BodyCap)
	if limit < len(text) {
		limit = boundaryAtOrBelow(text, limit, start)
	}
	if limit <= start {
		return start
	}
	if fits(limit) {
		return limit
	}
	// fits(low) holds or low is start; fits(high) does not.
	low, high := start, limit
	for {
		middle := boundaryAtOrBelow(text, low+(high-low)/2, low)
		if middle <= low {
			middle = boundaryAbove(text, low)
		}
		if middle >= high {
			return low
		}
		if fits(middle) {
			low = middle
		} else {
			high = middle
		}
	}
}

// boundaryAtOrBelow is the nearest UTF-8 character start at or below at.
func boundaryAtOrBelow(text string, at, floor int) int {
	for at > floor && at < len(text) && !utf8.RuneStart(text[at]) {
		at--
	}
	return at
}

// boundaryAbove is the start of the character after the one at at.
func boundaryAbove(text string, at int) int {
	if at >= len(text) {
		return len(text)
	}
	_, size := utf8.DecodeRuneInString(text[at:])
	return at + size
}
