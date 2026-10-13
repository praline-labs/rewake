package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The views of the registry forgive: List and ListReadOnly skip a record that
// does not load, and WorkDirs discards its errors, since a view that showed
// nothing because one record was damaged would hide every other session. A
// decision may not forgive (docs/v2/stage3-decision-build.md, Q3 and Q11):
// the notes it owes are frozen from the room's main when it is written, and
// the person's path stands on finding the caller below no session. Either
// read through a forgiving view would take a record it could not read for a
// session that is not there. Inventory is the strict read: every record of
// the room or an error naming the one that did not read.

// maxRecordBytes bounds a session record a strict read takes: a record is a
// few hundred bytes, and one past this is not one rewake wrote.
const maxRecordBytes = 1 << 20

// Inventoried is one session record of a room read strictly, and what is
// known of the run it names.
type Inventoried struct {
	Session Session
	// Run is the run's wrapper as observed: running, ended, or unknown — a
	// pid of another namespace, a stat that did not read. Unknown is never
	// taken for either.
	Run proc.IdentityEvidence
}

// InventoryError is a registry a strict read could not take whole: what was
// there is unknown.
type InventoryError struct {
	Path string
	Err  error
}

func (e *InventoryError) Error() string {
	return fmt.Sprintf("the session records %s cannot be read whole, so which sessions run is unknown: %v", e.Path, e.Err)
}

func (e *InventoryError) Unwrap() error { return e.Err }

// Inventory reads every session record of the room dir, without the cleanup
// a lookup does and without skipping any: a record that does not read, an
// entry of the registry that is no record, or a registry that cannot be
// listed is an error. Only a write that never finished — a name with a dot
// first — and the name locks beside the records are passed over. A registry
// is absent only when the room or its registry is missing for real: a link
// at either level, dangling or not, is an error, since an empty answer would
// tell a decision that no session runs.
func Inventory(dir string) ([]Inventoried, error) {
	root := state.SessionsPath(dir)
	entries, err := state.ReadDirBelow(dir, filepath.Base(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, &InventoryError{Path: root, Err: err}
	}
	var found []Inventoried
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		name, isRecord := strings.CutSuffix(entry.Name(), ".json")
		if !isRecord || !state.ValidName(name) {
			return nil, &InventoryError{Path: path, Err: errors.New("an entry that is no session record")}
		}
		session, err := strictRecord(path, name)
		if err != nil {
			return nil, &InventoryError{Path: path, Err: err}
		}
		found = append(found, Inventoried{Session: session, Run: observeSession(session)})
	}
	return found, nil
}

// strictRecord reads one record as a regular file, never through a link,
// and holds it to its name.
func strictRecord(path, name string) (Session, error) {
	raw, err := state.ReadRegular(path, maxRecordBytes)
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return Session{}, fmt.Errorf("the record does not read: %w", err)
	}
	if session.Name != name || session.ServicePID <= 0 || session.ServiceStart == 0 {
		return Session{}, errors.New("the record does not name its session and its process")
	}
	return session, nil
}

// observeSession is what can be known of a record's run from this process:
// a run of another boot has ended; a pid of a namespace this reader cannot
// see, or with this reader's own namespace unknown, is unknown.
func observeSession(session Session) proc.IdentityEvidence {
	if session.Boot != "" {
		current, err := currentBoot()
		if err != nil {
			return proc.IdentityUnknown
		}
		if current != session.Boot {
			return proc.IdentityEnded
		}
	}
	if !session.Judgeable() {
		return proc.IdentityUnknown
	}
	return observe(session.ServicePID, session.ServiceStart)
}
