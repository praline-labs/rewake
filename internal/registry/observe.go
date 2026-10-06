package registry

import (
	"errors"
	"os"
	"strings"

	"github.com/praline-labs/rewake/internal/state"
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

// WorkDirs lists where the live sessions of every room under a state root
// work, without cleaning up after ended ones. A grant that reaches one of them
// reaches that session's own configuration too (docs/grants.md#the-broad-tier).
func WorkDirs(root string) []string {
	rooms, _ := state.RoomDirs(root)
	var dirs []string
	for _, room := range rooms {
		sessions, _ := ListReadOnly(room)
		for _, session := range sessions {
			if session.CWD != "" {
				dirs = append(dirs, session.CWD)
			}
		}
	}
	return dirs
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
