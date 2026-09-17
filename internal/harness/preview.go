package harness

import (
	"strings"
	"unicode"
)

// preview is the author's first line, never a generated summary. The four-cell
// indent leaves 96 cells; non-ASCII characters count conservatively as two.
func preview(text string) string {
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
	width := 0
	for _, r := range clean {
		width += previewWidth(r)
	}
	if width <= 96 {
		return string(clean)
	}
	end, width := 0, 0
	for end < len(clean) && width+previewWidth(clean[end]) <= 95 {
		width += previewWidth(clean[end])
		end++
	}
	return string(clean[:end]) + "…"
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
