package taskbrief

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// The items of a text: task-list lines outside a fenced block, found by a
// line scanner of rewake's own, so Go takes no markdown dependency. The
// shell renders the same text with a full parser; an odd checkbox the two
// read apart is a known limit (docs/briefs.md).

var (
	// taskLine is a list item whose content starts with a checkbox and a
	// blank, as GFM's task list item: "- [ ] x", "* [x] x", "1. [ ] x".
	taskLine = regexp.MustCompile(`^[ \t]*(?:[-*+]|[0-9]{1,9}[.)])[ \t]+\[([ xX])\][ \t](.*)$`)
	// fenceLine opens or closes a fenced block.
	fenceLine = regexp.MustCompile("^[ \t]*(`{3,}|~{3,})(.*)$")
)

// Item is one task-list line of an edition, numbered from 1 in the text's
// order.
type Item struct {
	N    int    `json:"n"`
	Text string `json:"text"`
	Done bool   `json:"done"`
	// line is the item's line in the text, mark the byte of its box's
	// character there.
	line, mark int
}

// scan reads the items of a text, each done as the text marks it.
func scan(text string) []Item {
	var items []Item
	fence := ""
	for index, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if m := fenceLine.FindStringSubmatch(line); m != nil {
			switch {
			case fence == "":
				fence = m[1]
			case m[1][0] == fence[0] && len(m[1]) >= len(fence) && strings.TrimSpace(m[2]) == "":
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		m := taskLine.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		items = append(items, Item{
			N:    len(items) + 1,
			Text: strings.TrimSpace(line[m[4]:m[5]]),
			Done: line[m[2]] != ' ',
			line: index,
			mark: m[2],
		})
	}
	return items
}

// title is a text's first "# " heading outside a fenced block, else its
// first line that is not blank, cut to maxTitle code points.
func title(text string) string {
	first := ""
	fence := ""
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if m := fenceLine.FindStringSubmatch(line); m != nil {
			if fence == "" {
				fence = m[1]
			} else if m[1][0] == fence[0] && len(m[1]) >= len(fence) && strings.TrimSpace(m[2]) == "" {
				fence = ""
			}
		} else if fence == "" {
			if heading, ok := strings.CutPrefix(trimmed, "# "); ok && strings.TrimSpace(heading) != "" {
				return cut(strings.TrimSpace(heading))
			}
		}
		if first == "" && trimmed != "" {
			first = trimmed
		}
	}
	return cut(first)
}

func cut(text string) string {
	runes := []rune(text)
	if len(runes) <= maxTitle {
		return text
	}
	return string(runes[:maxTitle])
}

// echo is an item's text as a tick records it: at most maxItemEcho code
// points, a cut one ending in "…". A tick that stored a long item whole
// would write an entry past what a reader takes.
func echo(text string) string {
	runes := []rune(text)
	if len(runes) <= maxItemEcho {
		return text
	}
	return string(runes[:maxItemEcho-1]) + "…"
}

// render writes the text with each item's box set as done says and, when
// numbered, its number after the box: "- [x] #3 run the tests".
func render(text string, items []Item, numbered bool) string {
	lines := strings.Split(text, "\n")
	for _, item := range items {
		line := lines[item.line]
		box := " "
		if item.Done {
			box = "x"
			if line[item.mark] == 'X' {
				box = "X"
			}
		}
		after := line[item.mark+2:]
		if numbered {
			after = " #" + strconv.Itoa(item.N) + after
		}
		lines[item.line] = line[:item.mark] + box + "]" + after
	}
	return strings.Join(lines, "\n")
}

func isBlank(text string) bool {
	return strings.IndexFunc(text, func(r rune) bool { return !unicode.IsSpace(r) }) < 0
}
