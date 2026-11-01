package update

import (
	"context"
	"time"
)

// Check asks the registry for its latest release and, when it was not
// fetched yet, fetches its manifest and changelog — once per release, a few
// KB, so a room of any older build reading the shared record finds them; it writes the
// record under root and answers it with the check's error. A failed check
// keeps the last good answer and says why; a record that cannot be written —
// a root no launch made — leaves the next check due.
func Check(ctx context.Context, root string, npm NPM, now time.Time) (Record, error) {
	prev := ReadRecord(root)
	next, err := check(ctx, npm, prev)
	next.Checked = now.UTC()
	if err != nil {
		next = prev
		next.Checked, next.Error = now.UTC(), err.Error()
	}
	_ = WriteRecord(root, next)
	return next, err
}

func check(ctx context.Context, npm NPM, prev Record) (Record, error) {
	latest, err := npm.Latest(ctx)
	if err != nil {
		return Record{}, err
	}
	next := Record{Latest: latest}
	if prev.Latest == latest && prev.Manifest != nil {
		next.Manifest, next.Changelog = prev.Manifest, prev.Changelog
		return next, nil
	}
	m, changelog, err := npm.Fetch(ctx, latest)
	if err != nil {
		return Record{}, err
	}
	next.Manifest, next.Changelog = &m, changelog
	return next, nil
}

// watchPoll is how often a watcher reads the record to see a check due; the
// record, shared by every room of the base, keeps them from asking twice.
const watchPoll = 10 * time.Minute

// firstWait lets a launch make the root, and settle, before its first check.
const firstWait = 30 * time.Second

// Watch checks whenever the record under root is due, until ctx ends: main's
// host runs it for as long as main does.
func Watch(ctx context.Context, root string, npm NPM) {
	wait := firstWait
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = watchPoll
		if now := time.Now(); ReadRecord(root).Due(now) {
			_, _ = Check(ctx, root, npm, now)
		}
	}
}
