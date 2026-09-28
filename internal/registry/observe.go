package registry

import "github.com/praline-labs/rewake/internal/state"

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
