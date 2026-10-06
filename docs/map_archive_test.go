package docs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The archive of 1.x (docs/v2/stage3.md#documents): a document of code a step
// removes moves into archiveDir unchanged, and its index holds a table of every
// moved document — where it was, the step that moved it, and the SHA-256 of its
// bytes. A moved document keeps its bytes, so its links still read as written
// from where it was; and a record keeps its bytes too, so its links into what
// moved are resolved through the table. Every other document is rewritten.
const archiveDir = "archive-1.x"

// recordNames are the records of revision-docs.md#records--the-archive-of-1x
// that live outside roadmap/ and the archive: never edited, so their links into
// a moved document resolve through the table.
var recordNames = []string{
	"transport-milestones-2026-09-17.md", "claude-parity-2026-09-21.md", "intermittent-bugs.md",
	"thread-lock-probes.md", "thread-ownership-investigation.md", "server-observation.md",
	"inbox-acceptance.md", "native-mailbox-acceptance.md", "native-mailbox-check.md",
	"native-mailbox-ui-check.md", "native-terminal-progress.md", "turn-end-recovery-findings.md",
	"check-runner.md", "check-runner-proposal.md", "check-runner-scenarios.md",
	"mail-bridge-live.md", "work-queue.md",
}

func isRecord(document string) bool {
	document = filepath.ToSlash(document)
	return strings.HasPrefix(document, "roadmap/") || strings.HasPrefix(document, archiveDir+"/") ||
		strings.HasPrefix(document, "reviews") && !strings.Contains(document, "/") ||
		slices.Contains(recordNames, document)
}

// archived is one row of the archive's table.
type archived struct {
	from string // the document's path before the move, relative to docs/
	step string
	sum  string
}

// archiveRow is | [name](name) | `path before` | S<n> | `sha256` |.
var archiveRow = regexp.MustCompile("^\\| \\[[^]]*\\]\\(([^)]+)\\) \\| `([^`]+)` \\| (S\\d+) \\| `([0-9a-f]{64})` \\|$")

// archiveTable reads the table, keyed by the path a document had before it
// moved; no archive yet is an empty table.
func archiveTable() (map[string]archived, error) {
	text, err := os.ReadFile(filepath.Join(archiveDir, indexName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	table := map[string]archived{}
	for _, line := range strings.Split(string(text), "\n") {
		match := archiveRow.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if filepath.Clean(match[1]) != filepath.Clean(match[2]) {
			return nil, fmt.Errorf("docs/%s/%s: the row of %s links %s: a moved document keeps its path inside the archive", archiveDir, indexName, match[2], match[1])
		}
		table[filepath.Clean(match[2])] = archived{from: filepath.Clean(match[2]), step: match[3], sum: match[4]}
	}
	return table, nil
}

// resolve answers the file a relative link of a document names. An archived
// document's links resolve from the directory it was moved from; a record's,
// and an archived document's, links to a moved path then go through the table.
// The archive's index was written there, so its links resolve as anyone's.
func resolve(document, file string, table map[string]archived) string {
	dir := filepath.Dir(document)
	from, moved := strings.CutPrefix(filepath.ToSlash(document), archiveDir+"/")
	if moved && from != indexName {
		dir = filepath.Dir(filepath.FromSlash(from))
	}
	resolved := filepath.Clean(filepath.Join(dir, file))
	if _, err := os.Stat(resolved); err == nil || !isRecord(document) {
		return resolved
	}
	if _, ok := table[resolved]; ok {
		return filepath.Join(archiveDir, resolved)
	}
	return resolved
}

// TestTheArchiveKeepsItsBytes holds the archive to its table: every archived
// file has a row and the bytes the row records, every row its file, and no
// moved document is still at the path it left.
func TestTheArchiveKeepsItsBytes(t *testing.T) {
	for _, problem := range archiveProblems() {
		t.Error(problem)
	}
}

func archiveProblems() []string {
	table, err := archiveTable()
	if err != nil {
		return []string{err.Error()}
	}
	var problems []string
	seen := map[string]bool{}
	err = filepath.WalkDir(archiveDir, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && path == archiveDir {
			return filepath.SkipDir
		}
		if err != nil || entry.IsDir() || path == filepath.Join(archiveDir, indexName) {
			return err
		}
		from, _ := filepath.Rel(archiveDir, path)
		row, ok := table[from]
		if !ok {
			problems = append(problems, fmt.Sprintf("docs/%s is in no row of docs/%s/%s: record where it was, the step and its SHA-256", path, archiveDir, indexName))
			return nil
		}
		seen[from] = true
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if sum := sha256.Sum256(text); hex.EncodeToString(sum[:]) != row.sum {
			problems = append(problems, fmt.Sprintf("docs/%s differs from the bytes its row records: an archived document is never edited, restore it", path))
		}
		return nil
	})
	if err != nil {
		problems = append(problems, err.Error())
	}
	for from := range table {
		if !seen[from] {
			problems = append(problems, fmt.Sprintf("docs/%s/%s lists %s, which is not in docs/%s", archiveDir, indexName, from, archiveDir))
		}
		if _, err := os.Stat(from); err == nil {
			problems = append(problems, fmt.Sprintf("docs/%s is archived and still at its path: a moved document leaves it", from))
		}
	}
	slices.Sort(problems)
	return problems
}
