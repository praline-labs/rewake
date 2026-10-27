package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Review says a checkout was made for a reviewer given a brief: detached at
// the commit main named, so the writer moving on to its next work does not
// move what the reviewer reads. It is the review's, not a session's: the
// record names no Session, so a space never takes it for its worker's own
// checkout, and the reviewer's run is kept here to remove it when that run
// ends.
type Review struct {
	Brief    string `json:"brief"`
	Reviewer Owner  `json:"reviewer"`
}

// ReviewName is the name of a review's checkout: the brief, the reviewer and
// the commit, so a second review of a new commit gets a checkout of its own
// beside the first, and the same commit given again finds the one it has.
func ReviewName(brief, reviewer, commit string) string {
	return "review-" + brief + "-" + reviewer + "-" + short(commit)[:min(7, len(commit))]
}

// Resolve names the commit rev means in the repository holding from.
func Resolve(from, rev string) (string, error) {
	if strings.HasPrefix(rev, "-") {
		return "", &UnusableError{Reason: fmt.Sprintf("%q names no commit", rev)}
	}
	commit, err := gitOutput(from, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	if err != nil || commit == "" {
		return "", &UnusableError{Reason: fmt.Sprintf("%q names no commit in the repository at %s", rev, from)}
	}
	return commit, nil
}

// CreateReview adds a detached checkout of commit, from the repository
// holding from, for review, and records it, holding the repository's lock as
// Create does. No branch is made: a reviewer reads, and nothing it does there
// is meant to land. A checkout of the same name already made for this
// reviewer's run is answered as it is.
func CreateReview(root, from, commit string, review Review) (Record, error) {
	name := ReviewName(review.Brief, review.Reviewer.Name, commit)
	if !ValidName(name) {
		return Record{}, &UnusableError{Reason: fmt.Sprintf("the review checkout's name %q is not usable: %s", name, NameRule)}
	}
	source, err := inspect(from)
	if err != nil {
		return Record{}, err
	}
	for _, top := range append([]string{source.Source}, mainTops(source.CommonDir)...) {
		if top != "" && inside(resolved(root), resolved(top)) {
			return Record{}, &UnusableError{Reason: fmt.Sprintf("the worktree directory %s is inside the repository at %s; set %s to a directory outside it", root, top, RootEnv)}
		}
	}
	directory := filepath.Join(root, source.Repository)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Record{}, fmt.Errorf("cannot create %s: %w", root, err)
	}
	record := source
	record.Name, record.Commit, record.Subdir = name, commit, ""
	record.Path = filepath.Join(directory, flatName(name))
	record.CreatedAt = time.Now().UTC()
	record.Review = &review
	err = withRepositoryLock(directory, func() error {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("cannot create %s: %w", directory, err)
		}
		if held, err := read(recordPath(record)); err == nil {
			if held.Review != nil && held.Review.Reviewer == review.Reviewer && held.Commit == commit {
				record = held
				return nil
			}
			return &ExistsError{Record: held}
		}
		if _, err := os.Lstat(record.Path); err == nil {
			return &ExistsError{Record: record}
		}
		data, err := encode(record)
		if err != nil {
			return err
		}
		if err := publishRecord(recordPath(record), data); err != nil {
			if errors.Is(err, errRecordTaken) {
				return &ExistsError{Record: record}
			}
			return err
		}
		if err := git(source.Source, "worktree", "add", "--detach", record.Path, commit); err != nil {
			_ = os.Remove(recordPath(record))
			if info, statErr := os.Lstat(record.Path); statErr == nil && info.IsDir() {
				_ = os.Remove(record.Path)
			}
			_ = os.Remove(directory)
			return fmt.Errorf("git worktree add --detach failed: %w", err)
		}
		record = keepIncluded(include(record))
		return nil
	})
	if err != nil {
		return Record{}, err
	}
	return record, nil
}

// Reviews lists the review checkouts under root that pick accepts.
func Reviews(root string, pick func(Record) bool) ([]Record, error) {
	records, err := List(root)
	if err != nil {
		return nil, err
	}
	var found []Record
	for _, record := range records {
		if record.Review != nil && pick(record) {
			found = append(found, record)
		}
	}
	return found, nil
}

// ReleaseReviews removes the review checkouts pick names, each only when
// Release would — nothing git status shows, no commit only it holds — and
// answers those that stay with the reason.
func ReleaseReviews(root string, pick func(Record) bool) (map[string]string, error) {
	records, err := Reviews(root, pick)
	if err != nil {
		return nil, err
	}
	kept := map[string]string{}
	for _, record := range records {
		if stays, _ := Release(record); stays != "" {
			kept[record.Path] = stays
		}
	}
	return kept, nil
}
