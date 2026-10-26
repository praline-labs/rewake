package taskbrief

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// What a view reads. Nothing here writes or takes a lock: each file is whole
// by its atomic write, and the set of them is no snapshot, which a reader of
// a brief being ticked sees as one tick sooner or later.

// Brief is one brief as read: its record, its log and its current text.
type Brief struct {
	Record
	// Edition counts main's edits, 1 for a new brief; Text is its text, as
	// main wrote it.
	Edition int
	Text    string
	// Revision is the number of the latest edit, tick or untick; a comment
	// leaves it. Main's edit names it.
	Revision int
	Log      []Entry
	items    []Item
}

// Problem is an entry of the briefs directory that does not read as a brief.
type Problem struct {
	Path string `json:"path"`
	Why  string `json:"why"`
}

// IsClosed says whether main closed the brief. A close records it before
// it moves the brief to the archive, so a brief still in the open directory
// may be closed.
func (b Brief) IsClosed() bool { return b.Closed != nil }

// Title is the current text's first "# " heading, else its first line.
func (b Brief) Title() string { return title(b.Text) }

// Items are the current edition's items, each done by the latest tick or
// untick of this edition, else as main's text marks it.
func (b Brief) Items() []Item { return slices.Clone(b.items) }

// Done counts the items done.
func (b Brief) Done() int {
	done := 0
	for _, item := range b.items {
		if item.Done {
			done++
		}
	}
	return done
}

// Comments are the workers' comments, oldest first.
func (b Brief) Comments() []Entry {
	var comments []Entry
	for _, entry := range b.Log {
		if entry.Kind == KindComment {
			comments = append(comments, entry)
		}
	}
	return comments
}

// Markdown is the current text with the ticks applied, what main edits and
// the shell renders.
func (b Brief) Markdown() string { return render(b.Text, b.items, false) }

// Numbered is Markdown with each item's number after its box, what a worker
// reads to name the item it ticks.
func (b Brief) Numbered() string { return render(b.Text, b.items, true) }

// Load reads one brief, open or closed.
func Load(dir string, id int) (Brief, error) {
	path, err := find(dir, id)
	if err != nil {
		return Brief{}, err
	}
	return loadAt(path, id)
}

// find is the directory of brief id: open, else archived.
func find(dir string, id int) (string, error) {
	for _, path := range []string{openPath(dir, id), filepath.Join(state.ClosedBriefsPath(dir), FormatID(id))} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%s is not a directory", path)
		}
		return path, nil
	}
	return "", ErrNotFound
}

func openPath(dir string, id int) string { return filepath.Join(state.BriefsPath(dir), FormatID(id)) }

func loadAt(path string, id int) (Brief, error) {
	var brief Brief
	if err := readJSON(filepath.Join(path, recordFile), &brief.Record); err != nil {
		return Brief{}, err
	}
	if brief.ID != id {
		return Brief{}, fmt.Errorf("%s names brief %d, not %d", filepath.Join(path, recordFile), brief.ID, id)
	}
	log, err := readLog(filepath.Join(path, logDir))
	if err != nil {
		return Brief{}, err
	}
	brief.Log = log
	for _, entry := range log {
		switch entry.Kind {
		case KindEdit:
			brief.Edition = max(brief.Edition, entry.Edition)
			brief.Revision = entry.N
		case KindTick, KindUntick:
			brief.Revision = entry.N
		}
	}
	if brief.Edition == 0 {
		return Brief{}, fmt.Errorf("%s holds no edit", filepath.Join(path, logDir))
	}
	text, err := state.ReadRegular(textPath(path, brief.Edition), MaxText)
	if err != nil {
		return Brief{}, err
	}
	brief.Text = string(text)
	brief.items = scan(brief.Text)
	for _, entry := range log {
		if (entry.Kind == KindTick || entry.Kind == KindUntick) && entry.Edition == brief.Edition && entry.Item >= 1 && entry.Item <= len(brief.items) {
			brief.items[entry.Item-1].Done = entry.Kind == KindTick
		}
	}
	return brief, nil
}

func textPath(path string, edition int) string {
	return filepath.Join(path, textDir, FormatID(edition)+".md")
}

func readLog(path string) ([]Entry, error) {
	entries, err := state.ReadPlainDir(path)
	if err != nil {
		return nil, err
	}
	var log []Entry
	for _, file := range entries {
		name := file.Name()
		if strings.HasPrefix(name, ".") {
			continue // a write's temporary file
		}
		stem, ok := strings.CutSuffix(name, ".json")
		n, valid := ParseID(stem)
		if !ok || !valid {
			return nil, fmt.Errorf("%s is not a log entry", filepath.Join(path, name))
		}
		var entry Entry
		if err := readJSON(filepath.Join(path, name), &entry); err != nil {
			return nil, err
		}
		if entry.N != n || entry.Edition < 1 {
			return nil, fmt.Errorf("%s is entry %d of edition %d", filepath.Join(path, name), entry.N, entry.Edition)
		}
		switch entry.Kind {
		case KindEdit, KindTick, KindUntick, KindComment:
		default:
			return nil, fmt.Errorf("%s records %q, no kind of entry", filepath.Join(path, name), entry.Kind)
		}
		log = append(log, entry)
	}
	slices.SortFunc(log, func(a, b Entry) int { return a.N - b.N })
	return log, nil
}

func readJSON(path string, into any) error {
	raw, err := state.ReadRegular(path, maxRecordBytes)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("%s does not read: %w", path, err)
	}
	return nil
}

// List reads the open briefs and, with closed, the archived ones too: open
// ones first, each part newest first. A room with no briefs directory has
// none. A brief that does not read is a Problem beside the others, never a
// reason to show none.
func List(dir string, closed bool) ([]Brief, []Problem, error) {
	open, problems, err := listIn(dir, state.BriefsPath(dir))
	if err != nil {
		return nil, nil, err
	}
	if !closed {
		return open, problems, nil
	}
	archived, more, err := listIn(state.BriefsPath(dir), state.ClosedBriefsPath(dir))
	if err != nil {
		return nil, nil, err
	}
	return append(open, archived...), append(problems, more...), nil
}

// listIn reads the briefs of path, a directory directly inside parent.
func listIn(parent, path string) ([]Brief, []Problem, error) {
	entries, err := state.ReadDirBelow(parent, filepath.Base(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var briefs []Brief
	var problems []Problem
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || (path == state.BriefsPath(parent) && name == archiveDir) {
			continue // the lock, a new brief not yet in place, the archive
		}
		id, ok := ParseID(name)
		if !ok {
			problems = append(problems, Problem{Path: filepath.Join(path, name), Why: "not a brief"})
			continue
		}
		brief, err := loadAt(filepath.Join(path, name), id)
		if err != nil {
			problems = append(problems, Problem{Path: filepath.Join(path, name), Why: err.Error()})
			continue
		}
		briefs = append(briefs, brief)
	}
	slices.SortFunc(briefs, func(a, b Brief) int { return b.ID - a.ID })
	return briefs, problems, nil
}

// ids are the ids taken in a directory of briefs, a missing one taking none.
func ids(path string) ([]int, error) {
	entries, err := state.ReadPlainDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var taken []int
	for _, entry := range entries {
		if id, ok := ParseID(entry.Name()); ok {
			taken = append(taken, id)
		}
	}
	return taken, nil
}
