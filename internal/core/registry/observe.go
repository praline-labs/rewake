package registry

import (
	"errors"
	"os"
	"strings"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// LookupReadOnly checks liveness without deleting stale records. Optional state
// readers may hold another mailbox lock and must never enter name-lock cleanup.
func LookupReadOnly(dir, name string) (Session, error) {
	session, err := Load(dir, name)
	if err != nil {
		return Session{}, err
	}
	if !session.Alive() {
		return Session{}, ErrNotFound
	}
	return session, nil
}

// UnreadableRecord is a session record that cannot be read: whether its
// session runs is unknown.
type UnreadableRecord struct {
	Name string
	Err  error
}

// UnreadableRecords names the session records of a room that cannot be
// read, without the cleanup List does: a view of the room shows them as
// unknown rather than leaving out a session that may be running.
func UnreadableRecords(dir string) ([]UnreadableRecord, error) {
	entries, err := os.ReadDir(state.SessionsPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var unreadable []UnreadableRecord
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".json")
		if entry.IsDir() || name == entry.Name() || strings.HasPrefix(name, ".") || !state.ValidName(name) {
			continue
		}
		if _, err := Load(dir, name); err != nil && !errors.Is(err, ErrNotFound) {
			unreadable = append(unreadable, UnreadableRecord{Name: name, Err: err})
		}
	}
	return unreadable, nil
}
