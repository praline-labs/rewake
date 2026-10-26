// Package taskbrief keeps the room's briefs: a markdown text per task that
// main writes, bound to the workers it was sent to, ticked and commented by
// them (docs/roadmap/briefs.md). The word is not the briefing of package brief, the
// launch text; this package is named apart so neither is renamed.
package taskbrief

import (
	"errors"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"
)

// The records of one brief, under the room's briefs directory:
//
//	briefs/<id>/brief.json    the Record
//	briefs/<id>/log/<n>.json  an Entry, numbered from one counter
//	briefs/<id>/text/<e>.md   the text of edition e
//	briefs/closed/<id>/       a closed brief, moved whole

const (
	recordFile = "brief.json"
	logDir     = "log"
	textDir    = "text"
	tmpPrefix  = ".tmp-"
	archiveDir = "closed"
)

const (
	// MaxText bounds a text main writes, in bytes.
	MaxText = 64 << 10
	// MaxComment bounds a worker's comment, in code points.
	MaxComment = 1000
	// maxTitle cuts a title, in code points.
	maxTitle = 60
	// maxRecordBytes bounds brief.json and a log entry when read.
	maxRecordBytes = 64 << 10
)

// Binding is a run a brief was sent to: the right to read, tick and comment
// goes with the run, so a later run of the same name has none. It stays
// listed once the run ends.
type Binding struct {
	Name  string    `json:"name"`
	Epoch string    `json:"epoch"`
	Since time.Time `json:"since"`
}

// Record is brief.json, main's file: the id, when it was made, who it is
// bound to and, once closed, when.
type Record struct {
	ID      int        `json:"id"`
	Created time.Time  `json:"created"`
	Bound   []Binding  `json:"bound"`
	Closed  *time.Time `json:"closed,omitempty"`
}

// Kind is what a log entry records.
type Kind string

const (
	// KindEdit is main's new text, edition Entry.Edition.
	KindEdit Kind = "edit"
	// KindTick marks item Entry.Item of edition Entry.Edition done.
	KindTick Kind = "tick"
	// KindUntick marks it not done.
	KindUntick Kind = "untick"
	// KindComment is a worker's comment, read in edition Entry.Edition.
	KindComment Kind = "comment"
)

// Entry is one event of a brief's log, never edited or removed.
type Entry struct {
	N       int       `json:"n"`
	At      time.Time `json:"at"`
	From    string    `json:"from"`
	Epoch   string    `json:"epoch"`
	Kind    Kind      `json:"kind"`
	Edition int       `json:"edition"`
	// Item and ItemText are a tick's: the number in its edition and the text
	// it had there, for the reader of the log.
	Item     int    `json:"item,omitempty"`
	ItemText string `json:"itemText,omitempty"`
	// Text is a comment's.
	Text string `json:"text,omitempty"`
}

// Author is the run that writes: main for an edit, a bound run for a tick
// or a comment.
type Author struct {
	Name  string
	Epoch string
}

// FormatID is an id as a directory and a letter name it.
func FormatID(id int) string { return strconv.Itoa(id) }

// ParseID reads an id as typed: a positive integer written plainly, so one
// brief has one spelling.
func ParseID(text string) (int, bool) {
	id, err := strconv.Atoi(text)
	if err != nil || id < 1 || strconv.Itoa(id) != text {
		return 0, false
	}
	return id, true
}

// ErrNotFound is an id no brief has, open or closed.
var ErrNotFound = errors.New("no such brief")

// ErrBadText is a text main may not write.
var ErrBadText = errors.New("not a brief's text")

// ClosedError is a change of a closed brief.
type ClosedError struct{ ID int }

func (e *ClosedError) Error() string {
	return fmt.Sprintf("brief %d is closed: it takes no edit, tick or comment, and its bindings have ended", e.ID)
}

// NotBoundError is a run the brief was not given to.
type NotBoundError struct {
	ID   int
	Name string
}

func (e *NotBoundError) Error() string {
	return fmt.Sprintf("brief %d was not given to this run of %s: only main and the runs it was sent to with send --brief may read it, and only those runs tick and comment", e.ID, e.Name)
}

// StaleRevisionError is an edit made against a revision that is not the
// brief's any more: a tick or an edit came since main read it.
type StaleRevisionError struct{ ID, Given, Current int }

func (e *StaleRevisionError) Error() string {
	return fmt.Sprintf("brief %d is at revision %d, not %d: something changed since you read it; read it again with rewake brief show %d --markdown and edit that", e.ID, e.Current, e.Given, e.ID)
}

// StaleEditionError is a tick named in an edition main has since replaced.
type StaleEditionError struct{ ID, Given, Current int }

func (e *StaleEditionError) Error() string {
	return fmt.Sprintf("brief %d is at edition %d, not %d: main edited it since you read it, and the item numbers may have moved; re-read it with rewake brief show %d and tick by the new numbers", e.ID, e.Current, e.Given, e.ID)
}

// NoItemError is an item number the edition does not have.
type NoItemError struct{ ID, Item, Items int }

func (e *NoItemError) Error() string {
	if e.Items == 0 {
		return fmt.Sprintf("brief %d has no items to tick", e.ID)
	}
	return fmt.Sprintf("brief %d has items 1 to %d, not %d", e.ID, e.Items, e.Item)
}

// CheckText refuses a text main may not write: empty, past MaxText, or not
// UTF-8.
func CheckText(text string) error {
	switch {
	case len(text) > MaxText:
		return fmt.Errorf("%w: %d bytes, past the bound of %d", ErrBadText, len(text), MaxText)
	case !utf8.ValidString(text):
		return fmt.Errorf("%w: not UTF-8", ErrBadText)
	case isBlank(text):
		return fmt.Errorf("%w: it is empty", ErrBadText)
	}
	return nil
}

// CheckComment refuses a comment a worker may not write.
func CheckComment(text string) error {
	switch {
	case !utf8.ValidString(text):
		return errors.New("the comment is not UTF-8")
	case isBlank(text):
		return errors.New("the comment is empty")
	case utf8.RuneCountInString(text) > MaxComment:
		return fmt.Errorf("the comment has %d characters, past the bound of %d", utf8.RuneCountInString(text), MaxComment)
	}
	return nil
}

// Has says whether the run is bound to the brief.
func (r Record) Has(author Author) bool {
	for _, binding := range r.Bound {
		if binding.Name == author.Name && binding.Epoch == author.Epoch {
			return true
		}
	}
	return false
}
