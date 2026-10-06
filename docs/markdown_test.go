package docs

// This file is the one Markdown reader of the checks that read rules, steps and
// commands out of documents. docs/markdown_test.go and
// internal/layout_markdown_test.go hold it byte for byte, apart from the
// package clause, and TestTheMarkdownReaderIsOne fails when they differ: two
// test packages cannot import each other, and two readers would drift apart.

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// mdKind is what a line of a document is to these checks.
type mdKind int

const (
	proseLine mdKind = iota // text a reader sees as the document
	fenceLine               // the opening or closing delimiter of a fenced block
	codeLine                // inside a fenced block: an example, never a rule, a step or a heading
)

type mdLine struct {
	text string
	kind mdKind
}

var (
	fenceRun   = regexp.MustCompile("^( *)(`{3,}|~{3,})(.*)$")
	atxHeading = regexp.MustCompile(`^ *#{1,6}(?:[ \t]+|$)(.*)$`)
	setextLine = regexp.MustCompile(`^ *(=+|-+)[ \t]*$`)
)

// mdLines marks each line of text by CommonMark's rule for fenced blocks: a run
// of three or more backticks or tildes opens one, and only a run of the same
// character, at least as long and with nothing after it, closes it, so a
// shorter run or the other character inside is code. Whatever the reader cannot
// classify without modeling the containers around it is refused rather than
// read through: a fence indented four or more, a closing run indented other than
// its opening, a line less indented than an indented fence, a backtick run whose
// info string has a backtick, a tab before a run, a fence never closed, an HTML
// comment, a setext heading. Each of those could turn an example into what the
// check reads as the document, or the document into an example.
func mdLines(text string) ([]mdLine, error) {
	var lines []mdLine
	open, indent, opened := "", 0, 0
	previous := ""
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		rest := strings.TrimLeft(line, " \t")
		looksFence := strings.HasPrefix(rest, "```") || strings.HasPrefix(rest, "~~~")
		if looksFence && strings.Contains(line[:len(line)-len(rest)], "\t") {
			return nil, fmt.Errorf("line %d: a tab before a fence, which this check cannot place: indent it with spaces", n)
		}
		match := fenceRun.FindStringSubmatch(line)
		if open != "" {
			if match != nil && match[2][0] == open[0] && len(match[2]) >= len(open) && strings.TrimSpace(match[3]) == "" {
				if len(match[1]) == indent {
					open, previous = "", ""
					lines = append(lines, mdLine{line, fenceLine})
					continue
				}
				if len(match[1]) < indent+4 {
					return nil, fmt.Errorf("line %d closes the fence of line %d at another indentation: indent both alike", n, opened)
				}
			}
			if indent > 0 && strings.TrimSpace(line) != "" && indentOf(line) < indent {
				return nil, fmt.Errorf("line %d is less indented than the fence of line %d, which may end the list the fence is in: close the fence first", n, opened)
			}
			lines = append(lines, mdLine{line, codeLine})
			continue
		}
		switch {
		case strings.Contains(line, "<!--"):
			return nil, fmt.Errorf("line %d: an HTML comment hides text from a reader and not from this check: remove it", n)
		case setextLine.MatchString(line) && strings.TrimSpace(previous) != "" && !strings.HasPrefix(strings.TrimSpace(previous), "|"):
			return nil, fmt.Errorf("line %d underlines line %d into a heading this check does not read: write the heading with #", n, n-1)
		case match == nil:
			lines = append(lines, mdLine{line, proseLine})
			// Only a paragraph's line can be underlined into a heading.
			if previous = line; atxHeading.MatchString(line) {
				previous = ""
			}
			continue
		case len(match[1]) > 3:
			return nil, fmt.Errorf("line %d: a fence indented %d spaces, which may be code or a list's: indent it at most three past its list", n, len(match[1]))
		case match[2][0] == '`' && strings.Contains(match[3], "`"):
			return nil, fmt.Errorf("line %d opens like a fence, and a backtick after its run makes it inline code: write the example in a fence", n)
		default:
			open, indent, opened = match[2], len(match[1]), n
			lines = append(lines, mdLine{line, fenceLine})
		}
	}
	if open != "" {
		return nil, fmt.Errorf("the fence of line %d is never closed", opened)
	}
	return lines, nil
}

// indentOf counts a line's leading columns, a tab reaching the next multiple of
// four as Markdown counts it.
func indentOf(line string) int {
	columns := 0
	for _, r := range line {
		switch r {
		case ' ':
			columns++
		case '\t':
			columns += 4 - columns%4
		default:
			return columns
		}
	}
	return columns
}

// headingTitle reads an ATX heading at any indentation: one taken for a heading
// that Markdown renders as text ends a section early, which fails a check rather
// than widening it.
func headingTitle(line string) (string, bool) {
	match := atxHeading.FindStringSubmatch(line)
	if match == nil {
		return "", false
	}
	return strings.TrimSpace(match[1]), true
}

var (
	stepRow      = regexp.MustCompile(`^\| (S\d+) \|`)
	tableDivider = regexp.MustCompile(`^\|(\s*:?-{3,}:?\s*\|)+$`)
)

// buildOrder reads the steps of the first table under the heading "The build
// order", in prose: a row elsewhere — another table, an example — names no step.
// The table starts at the left margin, since an indented one may be code, and
// ends at its first other line.
func buildOrder(text string) ([]string, error) {
	lines, err := mdLines(text)
	if err != nil {
		return nil, err
	}
	var steps []string
	under, rows := false, -1
	for _, line := range lines {
		if rows >= 0 && (line.kind != proseLine || !strings.HasPrefix(line.text, "|")) {
			break
		}
		if line.kind != proseLine {
			continue
		}
		if title, ok := headingTitle(line.text); ok {
			if under {
				break
			}
			under = title == "The build order"
			continue
		}
		trimmed := strings.TrimSpace(line.text)
		if !under || !strings.HasPrefix(trimmed, "|") {
			continue
		}
		if !strings.HasPrefix(line.text, "|") {
			return nil, fmt.Errorf("an indented table under The build order, which may be code: start it at the margin: %q", line.text)
		}
		switch rows++; {
		case rows == 0 && !strings.HasPrefix(trimmed, "| Step |"):
			return nil, fmt.Errorf("the first table under The build order is no table of steps: %q", trimmed)
		case rows == 1 && !tableDivider.MatchString(trimmed):
			return nil, fmt.Errorf("the build order's table has no divider row: %q", trimmed)
		case rows >= 2:
			match := stepRow.FindStringSubmatch(trimmed)
			if match == nil {
				return nil, fmt.Errorf("a row of the build order names no step: %q", trimmed)
			}
			if slices.Contains(steps, match[1]) {
				return nil, fmt.Errorf("the build order names %s twice", match[1])
			}
			steps = append(steps, match[1])
		}
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("no table of steps under the heading The build order")
	}
	return steps, nil
}
