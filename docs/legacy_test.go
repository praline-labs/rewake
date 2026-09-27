package docs

import (
	"bufio"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The document that lists the marks, and the tree they are read from.
const (
	legacyFile      = "legacy.md"
	marksHeading    = "## Current marks"
	oldestHeading   = "## Oldest supported versions"
	legacyModuleDir = ".."
)

// legacyWord is what every mark starts with. A comment that holds it anywhere
// is taken for a mark, so a malformed one cannot hide from the search.
const legacyWord = "legacy("

// legacyMark is the one form a mark takes: what it keeps working and below which
// bound, why, and when it goes. Codex and Claude Code are bounded by a version of
// the harness, rewake's own records by the date of the change that replaced
// their form.
var legacyMark = regexp.MustCompile(`^legacy\((codex|claude|rewake) <([0-9A-Za-z.-]+)\): (\S.*); remove when (\S.*)$`)

var (
	semanticVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	markDate        = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

// mark is one mark found in the code, by the file it is in.
type mark struct {
	file, subject, bound string
}

func (m mark) head() string { return m.subject + " <" + m.bound }

// TestEveryLegacyMarkIsWellFormedAndListed reads every mark in the module's Go
// files and holds it to three things. Its form: the search the document gives
// finds a mark by its first word, and one written any other way is support for
// an old version nobody will find when it is time to remove it. The table: the
// document lists each file with its marks, so a reader sees what is kept for
// whom without running the search, and a table nobody keeps equal to the code
// teaches marks that are gone. And the harness bound against the oldest
// supported version: once that version has passed the bound, the mark is due,
// and this test says so rather than waiting for somebody to look.
func TestEveryLegacyMarkIsWellFormedAndListed(t *testing.T) {
	found := legacyMarks(t)
	listed := markTable(t)
	oldest := oldestSupported(t)

	counted := map[[2]string]int{}
	for _, m := range found {
		counted[[2]string{m.file, m.head()}]++
		switch m.subject {
		case "rewake":
			// The day of the commit, as its author's clock read it, which may be
			// a zone ahead of the machine running the test: a day's slack keeps
			// a mark written just after midnight elsewhere from reading as the
			// future, while a mark dated tomorrow or later is still caught.
			day, err := time.Parse(time.DateOnly, m.bound)
			if !markDate.MatchString(m.bound) || err != nil {
				t.Errorf("%s: the bound of a rewake mark is the date of the change, YYYY-MM-DD, not %q", m.file, m.bound)
			} else if day.After(time.Now().Add(24 * time.Hour)) {
				t.Errorf("%s: the mark's date %s is in the future", m.file, m.bound)
			}
		default:
			if !semanticVersion.MatchString(m.bound) {
				t.Errorf("%s: the bound of a %s mark is a version, x.y.z, not %q", m.file, m.subject, m.bound)
				continue
			}
			floor, ok := oldest[m.subject]
			if !ok {
				t.Errorf("%s: a %s mark, but docs/%s names no oldest supported %s under %q", m.file, m.subject, legacyFile, m.subject, oldestHeading)
			} else if compareVersions(floor, m.bound) >= 0 {
				t.Errorf("%s: the mark %q is due: the oldest supported %s is %s; remove what it keeps, as docs/%s says", m.file, m.head(), m.subject, floor, legacyFile)
			}
		}
	}
	for key, count := range counted {
		if listed[key] != count {
			t.Errorf("%s carries %d mark(s) %q, docs/%s lists %d under %q", key[0], count, key[1], legacyFile, listed[key], marksHeading)
		}
	}
	for key, count := range listed {
		if counted[key] == 0 {
			t.Errorf("docs/%s lists %d mark(s) %q in %s, which carries none: remove the row", legacyFile, count, key[1], key[0])
		}
	}
}

// legacyMarks reads the comments of every Go file in the module and returns the
// marks in them, failing on any comment line that names the word but is not a
// mark in the one form.
func legacyMarks(t *testing.T) []mark {
	t.Helper()
	var marks []mark
	err := filepath.WalkDir(legacyModuleDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != legacyModuleDir && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(legacyModuleDir, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		for _, group := range file.Comments {
			for _, comment := range group.List {
				for _, line := range strings.Split(comment.Text, "\n") {
					line = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSuffix(line, "*/"), "//"), "/*"))
					if !strings.Contains(line, legacyWord) {
						continue
					}
					parts := legacyMark.FindStringSubmatch(line)
					if parts == nil {
						t.Errorf("%s: %q is not a mark in the form of docs/%s: legacy(<codex|claude|rewake> <bound>): <why>; remove when <condition>", relative, line, legacyFile)
						continue
					}
					marks = append(marks, mark{file: relative, subject: parts[1], bound: parts[2]})
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return marks
}

// markTable reads the rows under marksHeading: a file, a mark's head and how
// many marks of that head the file carries.
func markTable(t *testing.T) map[[2]string]int {
	t.Helper()
	rows := map[[2]string]int{}
	for _, cells := range tableUnder(t, marksHeading) {
		if len(cells) < 3 {
			t.Errorf("docs/%s: a row under %q has %d cells, want the file, the mark and the count", legacyFile, marksHeading, len(cells))
			continue
		}
		file, head := strings.Trim(cells[0], "`"), strings.Trim(cells[1], "`")
		count, err := strconv.Atoi(cells[2])
		if err != nil {
			t.Errorf("docs/%s: the count %q of %s is not a number", legacyFile, cells[2], file)
			continue
		}
		rows[[2]string{file, head}] += count
	}
	return rows
}

// oldestSupported reads the rows under oldestHeading: a harness and the oldest
// version of it rewake supports.
func oldestSupported(t *testing.T) map[string]string {
	t.Helper()
	oldest := map[string]string{}
	for _, cells := range tableUnder(t, oldestHeading) {
		if len(cells) < 2 {
			continue
		}
		version := strings.Trim(cells[1], "`")
		if semanticVersion.MatchString(version) {
			oldest[strings.Trim(cells[0], "`")] = version
		}
	}
	return oldest
}

// tableUnder returns the cells of the body rows of the first table under a
// heading, the header and its separator left out.
func tableUnder(t *testing.T, heading string) [][]string {
	t.Helper()
	file, err := os.Open(legacyFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	var rows [][]string
	inside, seenHeader := false, false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "## "):
			inside = line == heading
			seenHeader = false
		case !inside || !strings.HasPrefix(line, "|"):
		case !seenHeader:
			seenHeader = true
		case strings.HasPrefix(line, "|-") || strings.HasPrefix(line, "| -"):
		default:
			var cells []string
			for _, cell := range strings.Split(strings.Trim(line, "|"), "|") {
				cells = append(cells, strings.TrimSpace(cell))
			}
			rows = append(rows, cells)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return rows
}

// compareVersions orders two x.y.z versions numerically.
func compareVersions(a, b string) int {
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for index := range left {
		x, _ := strconv.Atoi(left[index])
		y, _ := strconv.Atoi(right[index])
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
