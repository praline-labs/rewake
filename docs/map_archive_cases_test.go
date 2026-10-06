package docs

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The cases below run the link and archive rules on a synthetic tree: a map, a
// document that stays, a record, and one document moved from sub/ in S2.

const (
	movedText  = "# Moved\n\nIt linked [what stays](../stays.md) from sub/.\n"
	movedTable = "# The archive\n\n| Document | Moved from | Step | SHA-256 |\n|---|---|---|---|\n"
)

func archiveRowOf(from, text string) string {
	sum := sha256.Sum256([]byte(text))
	return "| [" + filepath.Base(from) + "](" + from + ") | `" + from + "` | S2 | `" + hex.EncodeToString(sum[:]) + "` |\n"
}

// archiveTree builds the tree in a temporary directory and makes it the
// working directory; files overrides or adds documents, an empty text removes one.
func archiveTree(t *testing.T, files map[string]string) {
	t.Helper()
	tree := map[string]string{
		"README.md":                "# Map\n\n[stays](stays.md), [the archive](archive-1.x/README.md), [records](roadmap/README.md)\n",
		"stays.md":                 "# Stays\n",
		"roadmap/README.md":        "# Records\n\n[an entry](entry.md)\n",
		"roadmap/entry.md":         "# Entry\n\nIt read [the moved one](../sub/moved.md#moved).\n",
		"archive-1.x/sub/moved.md": movedText,
		"archive-1.x/" + indexName: movedTable + archiveRowOf("sub/moved.md", movedText),
	}
	for name, text := range files {
		tree[name] = text
	}
	t.Chdir(t.TempDir())
	for name, text := range tree {
		if text == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func treeProblems(t *testing.T) []string {
	t.Helper()
	return append(linkProblems(t), archiveProblems()...)
}

func TestTheSyntheticArchivePasses(t *testing.T) {
	archiveTree(t, nil)
	if got := treeProblems(t); len(got) > 0 {
		t.Fatal(got)
	}
}

func TestAnActiveDocumentsLinkIntoAMovedFileFails(t *testing.T) {
	archiveTree(t, map[string]string{"stays.md": "# Stays\n\nSee [the moved one](sub/moved.md).\n"})
	oneProblem(t, treeProblems(t), "docs/stays.md links sub/moved.md, which does not exist")
}

func TestAnArchivedFileChangedByOneByteFails(t *testing.T) {
	archiveTree(t, map[string]string{"archive-1.x/sub/moved.md": movedText + " "})
	oneProblem(t, treeProblems(t), "docs/archive-1.x/sub/moved.md differs from the bytes its row records")
}

func TestARecordsLinkToAMovedPathOutsideTheTableFails(t *testing.T) {
	archiveTree(t, map[string]string{"archive-1.x/" + indexName: movedTable})
	got := treeProblems(t)
	if len(got) != 2 || !strings.Contains(strings.Join(got, "\n"), "docs/roadmap/entry.md links sub/moved.md, which does not exist") ||
		!strings.Contains(strings.Join(got, "\n"), "docs/archive-1.x/sub/moved.md is in no row") {
		t.Fatalf("got %q, want the record's link and the archived file without a row", got)
	}
}

func TestARowWithoutItsFileOrAFileLeftBehindFails(t *testing.T) {
	archiveTree(t, map[string]string{"archive-1.x/" + indexName: movedTable + archiveRowOf("sub/moved.md", movedText) + archiveRowOf("gone.md", "x")})
	oneProblem(t, archiveProblems(), "docs/archive-1.x/README.md lists gone.md, which is not in docs/archive-1.x")
	archiveTree(t, map[string]string{"sub/moved.md": movedText})
	oneProblem(t, archiveProblems(), "docs/sub/moved.md is archived and still at its path")
}

func TestARowLinkingElsewhereThanItsPathFails(t *testing.T) {
	sum := sha256.Sum256([]byte(movedText))
	row := "| [moved.md](moved.md) | `sub/moved.md` | S2 | `" + hex.EncodeToString(sum[:]) + "` |\n"
	archiveTree(t, map[string]string{"archive-1.x/" + indexName: movedTable + row})
	oneProblem(t, archiveProblems(), "a moved document keeps its path inside the archive")
}

func TestOnlyRecordsAreHelped(t *testing.T) {
	for document, want := range map[string]bool{
		"roadmap/x.md": true, "archive-1.x/a/b.md": true, "reviews-later.md": true, "work-queue.md": true,
		"flow.md": false, "v2/reviews.md": false, "rules/effects.md": false,
	} {
		if got := isRecord(document); got != want {
			t.Errorf("isRecord(%q) = %v, want %v", document, got, want)
		}
	}
}
