package mail

import (
	"strings"
	"unicode"
)

// Preview shares the same bounded first-line view between notices and inbox peeks.
func Preview(text string) string { return preview(text) }

// previewCells is what a preview line holds: the four-cell indent leaves 96
// of the notice's 100 columns.
const previewCells = 96

// preview is the author's first line, never a generated summary, within
// previewCells; non-ASCII characters count conservatively as two.
func preview(text string) string { return previewWithin(text, previewCells) }

// previewWithin is preview within cells, for a line that keeps the rest of
// its cells for words of its own.
func previewWithin(text string, cells int) string {
	if index := strings.IndexAny(text, "\r\n\u2028\u2029"); index >= 0 {
		text = text[:index]
	}
	var clean []rune
	for _, r := range text {
		if r == '\t' {
			r = ' '
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		clean = append(clean, r)
	}
	if strings.TrimSpace(string(clean)) == "" {
		return ""
	}
	if textCells(string(clean)) <= cells {
		return string(clean)
	}
	end, width := 0, 0
	for end < len(clean) && width+previewWidth(clean[end]) <= cells-1 {
		width += previewWidth(clean[end])
		end++
	}
	return string(clean[:end]) + "…"
}

func textCells(text string) int {
	width := 0
	for _, r := range text {
		width += previewWidth(r)
	}
	return width
}

func previewWidth(r rune) int {
	if unicode.Is(unicode.Mn, r) {
		return 0
	}
	if r < 128 || r == '…' {
		return 1
	}
	return 2
}
