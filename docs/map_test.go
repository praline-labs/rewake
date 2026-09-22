// Package docs holds the test that keeps the map of the documentation true.
//
// It lives beside the files it checks, the way scripts/ holds the tests of its
// shell scripts: the working directory of the test is the directory it reads,
// no package of the product has to know that documents exist, and the five
// checks run it with everything else.
package docs

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// mapFile is the map of this directory. A subdirectory has an index of its
// own, named indexName, which the map links instead of repeating it.
const (
	mapFile   = "README.md"
	indexName = "README.md"
)

// link finds a Markdown link: [text](target), [text](target#anchor) or
// [text](#anchor). Addresses with a scheme are not files and are left alone.
var link = regexp.MustCompile(`\]\(([^)\s#]*)(?:#([^)\s]*))?\)`)

// TestEveryDocumentIsOnTheMap requires every document to be reachable from
// the map: a document in docs/ is linked from the map; a document in a
// subdirectory, at any depth, is linked from that directory's index or from
// the map; and every such index is linked from the map. A document nobody
// lists is found only by opening files at random, which is what the map
// exists to end — and a new subdirectory must not be a way around it.
func TestEveryDocumentIsOnTheMap(t *testing.T) {
	fromMap := targets(t, mapFile)
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".md") || path == mapFile {
			return err
		}
		dir := filepath.Dir(path)
		if dir == "." {
			if !slices.Contains(fromMap, path) {
				t.Errorf("docs/%s is not listed in docs/%s: add a line saying what it contains and when to open it, or remove the document if nothing needs it any more", path, mapFile)
			}
			return nil
		}
		index := filepath.Join(dir, indexName)
		if path == index {
			if !slices.Contains(fromMap, index) {
				t.Errorf("docs/%s is not linked from docs/%s: a directory's index has to be on the map, or its documents are on no map", index, mapFile)
			}
			return nil
		}
		if _, statErr := os.Stat(index); statErr != nil {
			if !slices.Contains(fromMap, path) {
				t.Errorf("docs/%s has no docs/%s to list it and is not on docs/%s: add it to the map, or give its directory an index linked from the map", path, index, mapFile)
			}
			return nil
		}
		if !slices.Contains(targets(t, index), path) && !slices.Contains(fromMap, path) {
			t.Errorf("docs/%s is listed neither in docs/%s nor in docs/%s: add a line for it to its directory's index", path, index, mapFile)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestEveryLinkResolves requires every relative link in every document to
// name a file that exists and, when it carries an anchor, a heading that file
// has. A map that points at a renamed document, or at a section that was
// retitled, teaches the wrong place with confidence.
func TestEveryLinkResolves(t *testing.T) {
	anchors := map[string][]string{}
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		for _, target := range links(t, path) {
			if _, statErr := os.Stat(target.file); statErr != nil {
				t.Errorf("docs/%s links %s, which does not exist: correct the link or the file name", path, target.file)
				continue
			}
			if target.anchor == "" || !strings.HasSuffix(target.file, ".md") {
				continue
			}
			if _, ok := anchors[target.file]; !ok {
				anchors[target.file] = headings(t, target.file)
			}
			if !slices.Contains(anchors[target.file], target.anchor) {
				t.Errorf("docs/%s links %s#%s, and %s has no heading with that anchor: correct the anchor to the heading's current title", path, target.file, target.anchor, target.file)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type target struct{ file, anchor string }

// targets are the files one document links, resolved against docs/.
func targets(t *testing.T, document string) []string {
	t.Helper()
	var files []string
	for _, one := range links(t, document) {
		files = append(files, one.file)
	}
	return files
}

func links(t *testing.T, document string) []target {
	t.Helper()
	text, err := os.ReadFile(document)
	if err != nil {
		t.Fatalf("docs/%s is missing: the map of the documentation is docs/%s, see AGENTS.md", document, mapFile)
	}
	var found []target
	for _, match := range link.FindAllStringSubmatch(withoutCode(string(text)), -1) {
		file, anchor := match[1], match[2]
		if strings.Contains(file, "://") || strings.HasPrefix(file, "mailto:") {
			continue
		}
		if file == "" {
			file = document
		} else {
			file = filepath.Clean(filepath.Join(filepath.Dir(document), file))
		}
		found = append(found, target{file: file, anchor: anchor})
	}
	return found
}

// withoutCode blanks what a reader never sees as a link: fenced code blocks,
// inline code and HTML comments. A document linked only from one of those is
// on no map a reader can follow.
func withoutCode(text string) string {
	text = htmlComment.ReplaceAllStringFunc(text, blankKeepingLines)
	return inlineCode.ReplaceAllString(withoutFences(text), "")
}

var (
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	inlineCode  = regexp.MustCompile("`[^`\n]*`")
	// heading is what GitHub renders as a heading: one to six hashes and a
	// space. "#notaheading" is text, and an anchor to it goes nowhere.
	heading = regexp.MustCompile(`^#{1,6} `)
)

// blankKeepingLines replaces a match with its newlines only, so the text
// after it keeps its line structure.
func blankKeepingLines(match string) string {
	return strings.Repeat("\n", strings.Count(match, "\n"))
}

func withoutFences(text string) string {
	var kept strings.Builder
	fenced := false
	for _, line := range strings.SplitAfter(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			kept.WriteString("\n")
			continue
		}
		if fenced {
			kept.WriteString("\n")
			continue
		}
		kept.WriteString(line)
	}
	return kept.String()
}

// headings are the anchors a document's headings get, by the rule GitHub
// uses: lower case, punctuation other than hyphens and underscores dropped,
// spaces turned into hyphens, and a repeated slug numbered -1, -2 and so on.
func headings(t *testing.T, document string) []string {
	t.Helper()
	file, err := os.Open(document)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	seen := map[string]int{}
	var anchors []string
	fenced := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced || !heading.MatchString(line) {
			continue
		}
		title := strings.TrimSpace(strings.TrimLeft(line, "#"))
		slug := slugOf(title)
		if n := seen[slug]; n > 0 {
			anchors = append(anchors, fmt.Sprintf("%s-%d", slug, n))
		} else {
			anchors = append(anchors, slug)
		}
		seen[slug]++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return anchors
}

func slugOf(title string) string {
	var slug strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			slug.WriteRune(r)
		case r == ' ':
			slug.WriteRune('-')
		}
	}
	return slug.String()
}

func TestTheSlugRuleMatchesGitHub(t *testing.T) {
	for title, want := range map[string]string{
		"Evidence and acceptance":            "evidence-and-acceptance",
		"Sandbox (Linux)":                    "sandbox-linux",
		"`--grant-git`: who may attach it":   "--grant-git-who-may-attach-it",
		"Milestone 6. Ready for daily use":   "milestone-6-ready-for-daily-use",
		"What stays open — and what was not": "what-stays-open--and-what-was-not",
	} {
		if got := slugOf(title); got != want {
			t.Errorf("slug of %q is %q, want %q", title, got, want)
		}
	}
}
