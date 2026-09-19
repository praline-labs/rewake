package registry

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
