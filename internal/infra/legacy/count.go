package legacy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// Counts is what a clearing says it removed: rooms, unread letters and
// undelivered reports of a 1.x tree. Unknown is a count that could not be
// established, never a guess (D9).
type Counts struct {
	Rooms       int `json:"rooms"`
	Unread      int `json:"unread"`
	Undelivered int `json:"undelivered"`
}

// Unknown marks a count that could not be established.
const Unknown = -1

// The counter reads 1.x mailboxes and turn journals of a tree that passed its
// validation, before it is deleted; it is observation only and decides
// nothing — admission never asks it. Every read it cannot make turns its
// count unknown rather than zero.
//
// legacy(rewake <2026-10-12): 1.x mailboxes and turn journals are read to count the unread letters and undelivered reports a clearing removes, and decide nothing; remove when no session started by an earlier build is registered
func count(tree string) Counts {
	rooms, err := plainDirs(tree)
	if err != nil {
		return Counts{Rooms: Unknown, Unread: Unknown, Undelivered: Unknown}
	}
	total := Counts{Rooms: len(rooms)}
	for _, room := range rooms {
		unread, undelivered := countRoom(filepath.Join(tree, room))
		total.Unread = add(total.Unread, unread)
		total.Undelivered = add(total.Undelivered, undelivered)
	}
	return total
}

func add(a, b int) int {
	if a == Unknown || b == Unknown {
		return Unknown
	}
	return a + b
}

// countRoom counts the unread letters and undelivered reports of one room's
// mailboxes, inbox/<name>.
func countRoom(room string) (int, int) {
	inbox := filepath.Join(room, "inbox")
	names, err := plainDirs(inbox)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0
	}
	if err != nil {
		return Unknown, Unknown
	}
	unread, undelivered := 0, 0
	for _, name := range names {
		mailbox := filepath.Join(inbox, name)
		unread = add(unread, countUnread(mailbox))
		undelivered = add(undelivered, countUndelivered(inbox, mailbox))
	}
	return unread, undelivered
}

// countUnread counts the letters of a mailbox not yet read: waiting in it or
// delivered into unread/. A delivered letter is linked from its waiting copy,
// so a letter is counted once by its id, however many links it has. A
// tombstone and a letter whose status says it was read, failed or withdrawn
// are not counted.
func countUnread(mailbox string) int {
	ids := map[string]bool{}
	for _, dir := range []string{mailbox, filepath.Join(mailbox, "unread")} {
		entries, err := listDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Unknown
		}
		for _, entry := range entries {
			id, ok := strings.CutSuffix(entry.Name(), ".json")
			if !ok || strings.HasPrefix(id, ".") || ids[id] {
				continue
			}
			counted, err := isUnread(filepath.Join(dir, entry.Name()), filepath.Join(mailbox, id+".status"))
			if err != nil {
				return Unknown
			}
			if counted {
				ids[id] = true
			}
		}
	}
	return len(ids)
}

func isUnread(letter, statusPath string) (bool, error) {
	data, err := readFile(letter)
	if err != nil {
		return false, err
	}
	var message struct {
		Withdrawn json.RawMessage `json:"withdrawn"`
	}
	if err := json.Unmarshal(data, &message); err != nil {
		return false, fmt.Errorf("%s is no letter: %w", letter, err)
	}
	if len(message.Withdrawn) > 0 && string(message.Withdrawn) != "null" {
		return false, nil
	}
	data, err = readFile(statusPath)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var status struct {
		State     string `json:"state"`
		Withdrawn bool   `json:"withdrawn"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return false, fmt.Errorf("%s is no status: %w", statusPath, err)
	}
	return status.State != "read" && status.State != "failed" && !status.Withdrawn, nil
}

// countUndelivered counts the reports a mailbox's turn journals were to
// publish and are proven never published. A report the journal does not list
// as published or moot is looked for where it went: its publication mark
// saying published, or the letter in its recipient's mailbox, is delivered;
// no mark, or an intent with no letter, is undelivered; a mark or a mailbox
// that cannot be read is unknown, not absent. A conversion record with
// unknown entries, or any turn record of the build before, is unknown too.
func countUndelivered(inbox, mailbox string) int {
	if err := conversionKnown(filepath.Join(mailbox, "journal", "conversion")); err != nil {
		return Unknown
	}
	if turns, err := listDir(filepath.Join(mailbox, "turns")); err == nil && len(turns) > 0 || err != nil && !errors.Is(err, os.ErrNotExist) {
		return Unknown
	}
	entries, err := listDir(filepath.Join(mailbox, "journal"))
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		return Unknown
	}
	undelivered := 0
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".done") || name == "conversion" {
			continue
		}
		n, err := journalUndelivered(inbox, filepath.Join(mailbox, "journal", name))
		if err != nil {
			return Unknown
		}
		undelivered += n
	}
	return undelivered
}

type report struct {
	ID      string `json:"id"`
	To      string `json:"to"`
	ToEpoch string `json:"toEpoch"`
}

func journalUndelivered(inbox, path string) (int, error) {
	data, err := readFile(path)
	if err != nil {
		return 0, err
	}
	var journal struct {
		Reports   []report
		Published []string
		Moot      []string
		Done      bool
	}
	if err := json.Unmarshal(data, &journal); err != nil {
		return 0, fmt.Errorf("%s is no turn journal: %w", path, err)
	}
	if journal.Done {
		return 0, nil
	}
	settled := map[string]bool{}
	for _, id := range append(journal.Published, journal.Moot...) {
		settled[id] = true
	}
	n := 0
	for _, r := range journal.Reports {
		if settled[r.ID] {
			continue
		}
		delivered, err := reportDelivered(inbox, r)
		if err != nil {
			return 0, err
		}
		if !delivered {
			n++
		}
	}
	return n, nil
}

func reportDelivered(inbox string, r report) (bool, error) {
	for _, part := range []string{r.ID, r.To, r.ToEpoch} {
		if part == "" || strings.ContainsAny(part, `/\`) || strings.HasPrefix(part, ".") {
			return false, fmt.Errorf("a report names no letter to look for: %q", part)
		}
	}
	recipient := filepath.Join(inbox, r.To)
	mark, err := readFile(filepath.Join(recipient, "once", r.ToEpoch, r.ID))
	switch {
	case err == nil && string(mark) == "published":
		return true, nil
	case err == nil && string(mark) != "intent":
		return false, fmt.Errorf("the publication mark of %s says neither intent nor published", r.ID)
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return false, err
	}
	for _, dir := range []string{recipient, filepath.Join(recipient, "unread"), filepath.Join(recipient, "done")} {
		info, err := os.Lstat(filepath.Join(dir, r.ID+".json"))
		switch {
		case err == nil && info.Mode().IsRegular():
			return true, nil
		case err == nil:
			return false, fmt.Errorf("%s is not a letter", filepath.Join(dir, r.ID+".json"))
		case !errors.Is(err, os.ErrNotExist):
			return false, err
		}
	}
	return false, nil
}

func conversionKnown(path string) error {
	data, err := readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var conversion struct{ Unknown []string }
	if err := json.Unmarshal(data, &conversion); err != nil {
		return err
	}
	if len(conversion.Unknown) > 0 {
		return errors.New("the conversion left reports unknown")
	}
	return nil
}

// plainDirs names the directories directly in dir, skipping dotted names; a
// link or any other kind of entry in it is an error.
func plainDirs(dir string) ([]string, error) {
	entries, err := listDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !entry.IsDir() {
			return nil, fmt.Errorf("%s is not a directory", filepath.Join(dir, entry.Name()))
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

// listDir reads a directory that is one itself, not a link to one, and not a
// FIFO put in its place, which the open would wait on: the open itself takes
// nothing else, whatever was checked before it.
func listDir(dir string) ([]os.DirEntry, error) {
	if err := state.AskRead(dir); err != nil {
		return nil, err
	}
	return state.ReadPlainDir(dir)
}
