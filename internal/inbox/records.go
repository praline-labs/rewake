package inbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/state"
)

// Every file a mailbox holds is of one kind on this list
// (docs/mailbox-records.md). The barrier, and
// every call that would change the mailbox, walks the whole tree first and
// reads each file by its kind: one that matches no kind, is not a regular
// file, or does not read as its kind says stops the mailbox before any effect,
// and so does a directory anywhere but on the way to a kind's files
// (8-stop). An unknown found only by the effect that needed it comes too late,
// since effects before it went through; so nothing is left for an effect to
// find first. A writer that adds a path adds its kind here, or its first file
// stops every mailbox it lands in, and the test over the package's writers
// says so before that happens (records_test.go).

// recordKind is one kind of file a mailbox holds.
type recordKind struct {
	// pattern is the file's path under the mailbox, as filepath.Match
	// takes it; the first kind that matches is the file's.
	pattern string
	// what the file is.
	what string
	// parse checks the content of one file of the kind, which the reading
	// reads itself: no kind can leave its file unread. Nil for a file whose
	// content says nothing.
	parse func(path string, raw []byte) error
	// opened says the reading reads the file even with nothing to parse:
	// its being readable is what it says. A lock is not opened, and neither
	// is the stop record, which the barrier reads itself (stop.go).
	opened bool
	// unknown is what one that cannot be read leaves unknown.
	unknown string
}

// unfinishedWrite is the name state.WriteAtomic gives a file before it is
// renamed into place, in whatever directory: a write that never finished,
// never a record.
const unfinishedWrite = ".tmp-*"

var recordKinds = []recordKind{
	{".lock", "the mailbox lock", nil, false, ""},
	{stopFile, "the stop this mailbox is in", nil, false, "why it stopped; the barrier reads it itself (stop.go)"},
	{"*.json", "a letter not announced yet", parseJSON[Message], true, "which letter it is, and whether it is owed a report"},
	{"*.status", "a letter's delivery status", parseJSON[Status], true, "whether the letter arrived"},
	{"unread/*.json", "a letter announced and not read", parseJSON[Message], true, "which letter it is, and whether it is owed a report"},
	{"done/*.json", "a letter read or refused", parseJSON[Message], true, "which letter it is, and whether it was answered"},
	{"awaiting/*/.read-clock", "a run's read clock", parseClock, true, "the order of the run's reads and holds"},
	{"awaiting/*/.read-high", "the highest place taken on a run's read clock", parseHigh, true, "the order of the run's reads and holds"},
	{"awaiting/*/*", "who a run owes a report", parseWait, true, "what is owed, and to whom"},
	{"answering/*", "a question a send waits on", nil, true, "whether a send waits for its answer"},
	{"received/*", "the report printed for a question", nil, true, "whether a question received its answer"},
	{"retention/*", "when a reserved report is released", parseJSON[answerLifetime], true, "when the report is released"},
	{"turns/*", "an earlier build's turn receipt", parseJSON[convertedReceipt], true, "what an earlier build's turn end published"},
	{"journal/" + conversionFile, "the conversion of the earlier build's receipts", parseJSON[conversionJournal], true, "what the earlier build published, and the person's decisions"},
	{"journal/*" + doneSuffix, "a completed turn journal", parseJournalFile, true, "where the next end's window opens"},
	{"journal/*", "a turn journal", parseJournalFile, true, "what a turn end publishes, takes and clears"},
	{"pending/kept.json", "a held answer", parseJSON[keptRecord], true, "which answer a turn end takes"},
	{"pending/interim.json", "a run's last word on the work", parseJSON[interimRecord], true, "whether the work goes on"},
	{"pending/marks/*/*", "a pending mark", parseMark, true, "whether a turn end was interim"},
	{"once/*/*", "a publication mark", parseOnce, true, "whether the letter it names was written"},
	{"threads/*", "the conversation a letter was delivered into", nil, true, "where the letter was delivered"},
	{"claims/*", "a read in parts that claimed a letter", nil, true, "whether a read is showing the letter"},
	{"receipts/*/.*.lock", "a call's receipt lock", nil, false, ""},
	// A shell observation tells the run's wrapper how the shell reached the
	// mail; it decides nothing in the mailbox, so it is never opened, and one
	// that cannot be read costs the observation, never the mail.
	{"receipts/*/channel/*", "a shell observation of the mail channel", nil, false, ""},
	{"receipts/*/key-*", "the key a repeated call joins by", receipt.CheckKey, true, "which call a repeat joins"},
	{"receipts/*/calls/*", "the operation a tool call holds", receipt.CheckBinding, true, "which operation a tool call held"},
	{"receipts/*/*.json", "a call's receipt", receipt.CheckRecord, true, "what a call did and answered"},
}

// kindOf is the kind a path under the mailbox is of.
func kindOf(rel string) (recordKind, bool) {
	if matched, _ := filepath.Match(unfinishedWrite, filepath.Base(rel)); matched {
		return recordKind{pattern: unfinishedWrite, what: "a write that never finished"}, true
	}
	for _, kind := range recordKinds {
		if matched, _ := filepath.Match(kind.pattern, rel); matched {
			return kind, true
		}
	}
	return recordKind{}, false
}

// holdsRecords says rel is a directory some kind's files lie under: one a
// pattern passes through. A directory anywhere else stands where a record
// belongs, or where nothing does, and hides what should be there.
func holdsRecords(rel string) bool {
	for _, kind := range recordKinds {
		parts := strings.Split(kind.pattern, "/")
		for i := 1; i < len(parts); i++ {
			if matched, _ := filepath.Match(strings.Join(parts[:i], "/"), rel); matched {
				return true
			}
		}
	}
	return false
}

// readMailbox walks the mailbox and reads every file by its kind; the first
// that cannot be told stops it, named with what it leaves unknown. A file
// that left since it was listed moved on, and is not one.
func (w world) readMailbox(name string) error {
	root := state.InboxPath(w.dir, name)
	return w.walkMailbox(name, root, root)
}

func (w world) walkMailbox(name, root, directory string) error {
	entries, err := w.readDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("the mailbox record %s cannot be listed, so what it holds is unknown: %w", directory, err)
	}
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		kind, ok := kindOf(rel)
		if entry.IsDir() {
			switch {
			case holdsRecords(rel):
				if err := w.walkMailbox(name, root, path); err != nil {
					return err
				}
				continue
			case ok:
				return fmt.Errorf("%s, where %s belongs, is a directory, so what it says is unknown", path, kind.what)
			}
			return fmt.Errorf("%s is no directory a mailbox holds, so what it holds is unknown; nothing changes in %s until it is removed", path, name)
		}
		if !ok {
			return fmt.Errorf("%s is no kind of record a mailbox holds, so what it says is unknown; nothing changes in %s until it is removed", path, name)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%s, %s, is not a regular file, so what it says is unknown", path, kind.what)
		}
		if !kind.opened {
			continue
		}
		raw, err := w.readFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err == nil && kind.parse != nil {
			err = unknownRead(path, kind.parse(path, raw))
		}
		if err != nil {
			return fmt.Errorf("%s, %s, cannot be read, so %s is unknown: %w", path, kind.what, kind.unknown, err)
		}
	}
	return nil
}

// UnlistedRecords names the files of a mailbox that are of no kind on the
// list, and the directories no kind names or holds, without reading any: what a writer left that the barrier would stop
// on as unknown.
func UnlistedRecords(dir, name string) ([]string, error) {
	root := state.InboxPath(dir, name)
	var unlisted []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, listed := kindOf(rel)
		if entry.IsDir() {
			// A directory where a record belongs is a known path in the
			// wrong shape, which the reading stops on; this names paths.
			if rel != "." && !holdsRecords(rel) && !listed {
				unlisted = append(unlisted, rel+"/")
			}
			return nil
		}
		if !listed {
			unlisted = append(unlisted, rel)
		}
		return nil
	})
	return unlisted, err
}

func parseJSON[T any](_ string, raw []byte) error {
	var into T
	return json.Unmarshal(raw, &into)
}

func parseJournalFile(path string, raw []byte) error {
	_, err := parseJournal(path, raw)
	return err
}

func parseOnce(path string, raw []byte) error {
	if string(raw) != onceIntent && string(raw) != oncePublished {
		return fmt.Errorf("the publication mark %s says neither %s nor %s", path, onceIntent, oncePublished)
	}
	return nil
}

// parseClock reads a read clock as openReadClock takes it: one word, or
// nothing yet.
func parseClock(_ string, raw []byte) error {
	if size := len(raw); size != 0 && size != 8 {
		return fmt.Errorf("a read clock of %d bytes", size)
	}
	return nil
}

// parseHigh reads the high place as readHigh takes it: a number, in at most
// 32 bytes.
func parseHigh(_ string, raw []byte) error {
	if len(raw) > 32 {
		return errors.New("invalid read-boundary high watermark")
	}
	_, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
	return err
}

// parseWait reads a wait as ReadWaiters does; a file whose name is no
// session's is none ReadWaiters would read, so its wait would go unanswered.
func parseWait(path string, raw []byte) error {
	peer := filepath.Base(path)
	if !state.ValidName(peer) {
		return fmt.Errorf("%q names no session", peer)
	}
	_, err := parseWaiter(peer, string(raw))
	return err
}

// parseMark reads a pending mark as MarkWithin does.
func parseMark(path string, raw []byte) error {
	at, ok := parseMarkName(filepath.Base(path))
	if !ok {
		return errors.New("its name says no time")
	}
	var record markRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return err
	}
	if record.Epoch != filepath.Base(filepath.Dir(path)) || record.At != at || strings.TrimSpace(record.Text) == "" {
		return errors.New("it does not say what its name says")
	}
	return nil
}
