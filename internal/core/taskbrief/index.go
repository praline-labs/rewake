package taskbrief

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// Where a brief lives (docs/briefs.md#where-a-brief-lives). A brief is kept in
// the room unless main put it in a folder of the project. The project's
// setting names one folder, Home.Folder, and its briefs are found there by
// their names, so a room wiped with the temporary directory loses none of
// them. A brief put in another folder with --dir is found through the room's
// index, id to folder, which a wipe forgets.

const (
	indexFile = "index.json"
	// folderPrefix names a brief's folder inside the directory it was put in.
	folderPrefix = "brief-"
)

// Home is where the room's briefs are looked for: the room, and the project's
// folder when its setting names one (absolute, or empty).
type Home struct {
	Room   string
	Folder string
}

// roomName is the name brief.json of a brief outside the room records.
func (h Home) roomName() string { return filepath.Base(h.Room) }

// index is briefs/index.json, the briefs kept in a folder other than the
// project's own.
type index struct {
	Briefs map[string]indexed `json:"briefs"`
}

// indexed is one such brief. Closed is set when main closed it with its
// folder gone, the one record of it left: the entry stays, so its id is not
// taken again in this room.
type indexed struct {
	Path   string `json:"path"`
	Closed bool   `json:"closed,omitempty"`
}

func indexPath(room string) string { return filepath.Join(state.BriefsPath(room), indexFile) }

// readIndex reads the index, a missing one being empty.
func readIndex(room string) (index, error) {
	idx := index{}
	err := readJSON(indexPath(room), &idx)
	if errors.Is(err, os.ErrNotExist) {
		return index{Briefs: map[string]indexed{}}, nil
	}
	if err != nil {
		return index{}, err
	}
	if idx.Briefs == nil {
		idx.Briefs = map[string]indexed{}
	}
	for key, entry := range idx.Briefs {
		if _, ok := ParseID(key); !ok || !filepath.IsAbs(entry.Path) || filepath.Clean(entry.Path) != entry.Path {
			return index{}, fmt.Errorf("%s names brief %q at %q, which is not an id and an absolute path", indexPath(room), key, entry.Path)
		}
	}
	return idx, nil
}

func writeIndex(room string, idx index) error {
	raw, err := encode(idx)
	if err != nil {
		return err
	}
	return state.WriteAtomic(indexPath(room), raw)
}

// ids are the indexed ids, sorted.
func (i index) ids() []int {
	var taken []int
	for key := range i.Briefs {
		id, _ := ParseID(key)
		taken = append(taken, id)
	}
	slices.Sort(taken)
	return taken
}

// folderPath is the folder of brief id inside a directory of the project.
func folderPath(parent string, id int) string {
	return filepath.Join(parent, folderPrefix+FormatID(id))
}

// foldersIn are the ids of the brief folders inside a directory of the
// project, whatever room they belong to, a missing directory holding none.
func foldersIn(parent string) ([]int, error) {
	if parent == "" {
		return nil, nil
	}
	entries, err := state.ReadPlainDir(parent)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var found []int
	for _, entry := range entries {
		if stem, ok := strings.CutPrefix(entry.Name(), folderPrefix); ok {
			if id, ok := ParseID(stem); ok {
				found = append(found, id)
			}
		}
	}
	return found, nil
}

// isOurs says whether a brief read in a folder outside the room belongs to
// this room: another room's is not ours, listed nowhere and never written.
func (h Home) isOurs(record Record) bool { return record.Room == h.roomName() }

// MovedError is a brief the index places in a folder that is gone or holds
// no brief: moved with its task, archived, deleted. Nothing else about it is
// known; closing it marks it closed in the index.
type MovedError struct {
	ID   int
	Path string
}

func (e *MovedError) Error() string {
	return fmt.Sprintf("brief %d moved: its folder %s is gone or holds no brief; rewake brief close %d marks it closed in the list", e.ID, e.Path, e.ID)
}
