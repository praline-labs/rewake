package worktree

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// Claim writes the session a checkout was made for into its record.
func Claim(record Record, owner Owner) (Record, error) {
	record.Session = &owner
	data, err := encode(record)
	if err != nil {
		return record, err
	}
	return record, state.WriteAtomic(recordPath(record), data)
}

// List returns every checkout recorded under root, by repository and name. A
// record that cannot be read is left out: it names nothing a command could act
// on, and one bad file should not hide the rest.
func List(root string) ([]Record, error) {
	paths, err := filepath.Glob(filepath.Join(root, "*", "*.json"))
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, path := range paths {
		record, err := read(path)
		if err != nil || recordPath(record) != path || filepath.Base(record.Path) != record.Name {
			// A record whose checkout is not the directory of its name
			// beside it names a path rewake did not make, and rm would
			// remove it.
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Ref() < records[j].Ref() })
	return records, nil
}

// Find returns the checkouts a command names: <repository>/<name> names one, a
// bare name every repository's checkout of that name.
func Find(root, ref string) ([]Record, error) {
	records, err := List(root)
	if err != nil {
		return nil, err
	}
	var found []Record
	for _, record := range records {
		if record.Ref() == ref || !strings.Contains(ref, "/") && record.Name == ref {
			found = append(found, record)
		}
	}
	return found, nil
}

func recordPath(record Record) string {
	return filepath.Join(filepath.Dir(record.Path), record.Name+".json")
}

func read(path string) (Record, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(raw, &record); err != nil {
		return Record{}, err
	}
	if !ValidName(record.Name) || !filepath.IsAbs(record.Path) || filepath.Clean(record.Path) != record.Path {
		return Record{}, fmt.Errorf("%s is not a worktree record", path)
	}
	return record, nil
}

func encode(record Record) ([]byte, error) {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
