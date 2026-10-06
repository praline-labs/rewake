package inbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// A journaled heads-up is published at most once, however often a retry asks
// (docs/mail-bridge.md#receipts-retries-and-deadlines). PutOnce alone cannot
// promise that: finished mail is swept after a day, and a retry that came later
// would find no copy and write the letter again. So the recipient's mailbox
// keeps a mark per published id, for the run the letter was written for:
// "intent" before the letter is written, "published" after. The sweep turns an
// intent into published before it removes a letter the mark names, so an
// intent without a letter always means the letter was never written.

const (
	onceIntent    = "intent"
	oncePublished = "published"
)

// The steps of a once-publication's section another step is ordered against
// in a test: the recipient's run read live, and the first read of the
// publication's evidence.
const (
	PublicationAdmitted = "admitted"
	PublicationEvidence = "evidence"
)

// PublicationStep is told each step of a once-publication's section, inside
// the recipient's lock, by the pass that writes: a test pauses a publisher
// there while it ends the run or starts a sweep. Nil but in tests.
var PublicationStep func(step string, message Message)

func (w world) publicationStep(step string, message Message) {
	if !w.plan && PublicationStep != nil {
		PublicationStep(step, message)
	}
}

// readOnceMark answers the mark at path: "" when there is none, onceIntent or
// oncePublished. Anything else — a mark that cannot be read, or one that says
// neither — is an error: it proves no letter absent, so nothing may be written
// over it, published on it, or removed by it (docs/mail-bridge-cli.md, rule 8).
func (w world) readOnceMark(path string) (string, error) {
	mark, err := w.readFile(path)
	// Only a missing file proves no mark (rule 6): a path that cannot be
	// followed, through a file where a directory belongs, may hide one.
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if string(mark) != onceIntent && string(mark) != oncePublished {
		return "", unknownRecord(path, fmt.Errorf("the publication mark %s says neither %s nor %s", path, onceIntent, oncePublished))
	}
	return string(mark), nil
}

// oncePath is the mark of one letter.
func oncePath(dir, to, epoch, id string) (string, bool) {
	if !safeID(id) || epoch == "" || strings.ContainsAny(epoch, `/\`) || strings.HasPrefix(epoch, ".") {
		return "", false
	}
	return filepath.Join(state.InboxPath(dir, to), "once", epoch, id), true
}

// ErrRecipientEnded says the run a letter was addressed to was not the live
// run of its name when the recipient's lock was held: nothing was written.
var ErrRecipientEnded = errors.New("the run the letter was addressed to has ended")

// PublishOnce writes a letter into its recipient's mailbox unless an earlier
// call did, under the recipient's mailbox lock. It answers whether this call
// wrote it. The recipient's run is read inside that lock, before the
// evidence, and one that is not the letter's answers ErrRecipientEnded: a
// run read live before the lock could end, and its proof be swept, before
// the evidence is read (docs/v2/stage3-publication.md#the-contract). before
// runs under that lock just before anything is written, and an error from it
// writes nothing: a check made before the lock wait is stale by the time the
// lock is held.
func PublishOnce(ctx context.Context, dir string, message Message, before func() error) (bool, error) {
	path, ok := oncePath(dir, message.To, message.ToEpoch, message.ID)
	if !ok {
		return false, errors.New("a letter published once needs an id and the recipient's run")
	}
	wrote, w := false, live(dir)
	err := state.WithMailboxLock(ctx, dir, message.To, func() error {
		// Without the name lock's cleanup, which a holder of a mailbox lock
		// must not enter.
		current, err := registry.LookupReadOnly(dir, message.To)
		if errors.Is(err, registry.ErrNotFound) || err == nil && current.Epoch() != message.ToEpoch {
			return ErrRecipientEnded
		}
		if err != nil {
			return err
		}
		w.publicationStep(PublicationAdmitted, message)
		w.publicationStep(PublicationEvidence, message)
		// A mark that cannot be read is not an absent one.
		mark, err := w.readOnceMark(path)
		if err != nil {
			return err
		}
		if mark == oncePublished {
			return nil
		}
		found, err := w.present(message.To, message.ID)
		if err != nil {
			// A mailbox that cannot be searched may hold the letter: writing
			// it again could make two.
			return err
		}
		if mark == onceIntent && found {
			return state.WriteAtomic(path, []byte(oncePublished))
		}
		if before != nil {
			if err := before(); err != nil {
				return err
			}
		}
		if mark != onceIntent {
			if err := state.EnsureSubdir(filepath.Join(state.InboxPath(dir, message.To), "once")); err != nil {
				return err
			}
			if err := state.EnsureSubdir(filepath.Dir(path)); err != nil {
				return err
			}
			if err := state.WriteAtomic(path, []byte(onceIntent)); err != nil {
				return err
			}
		}
		if err := PutOnce(dir, message); err != nil {
			return err
		}
		wrote = true
		return state.WriteAtomic(path, []byte(oncePublished))
	})
	return wrote, err
}

// present says whether a mailbox holds a letter in any of its stages. An
// error means it could not tell: only "no such file" in every stage is
// absence, and something other than a file where the letter belongs hides
// whether it is there.
func present(dir, to, id string) (bool, error) { return live(dir).present(to, id) }

func (w world) present(to, id string) (bool, error) {
	var unknown error
	for _, directory := range []string{state.InboxPath(w.dir, to), state.UnreadPath(w.dir, to), state.DonePath(w.dir, to)} {
		path := filepath.Join(directory, id+".json")
		info, err := w.stat(path)
		switch {
		case err == nil && info.Mode().IsRegular():
			return true, nil
		case err == nil:
			unknown = unknownRecord(path, fmt.Errorf("%s, where a letter belongs, is not a regular file", path))
		case !errors.Is(err, os.ErrNotExist):
			unknown = err
		}
	}
	return false, unknown
}

// settleOnce records a letter as published before the sweep removes it, and
// answers whether the letter may go: not while an intent stands that could
// not be settled, nor beside a mark that is unknown. The caller holds the
// mailbox lock and has read there that epoch is the name's live run.
func settleOnce(dir, name, epoch, id string) bool {
	path, ok := oncePath(dir, name, epoch, id)
	if !ok {
		return true
	}
	mark, err := live(dir).readOnceMark(path)
	if err != nil {
		return false
	}
	if mark != onceIntent {
		return true
	}
	return state.WriteAtomic(path, []byte(oncePublished)) == nil
}

// Publication is what a recipient's mailbox shows of a letter published once.
type Publication int

const (
	// PublicationUnknown says neither the letter nor its mark is left.
	PublicationUnknown Publication = iota
	// PublicationWritten says the letter is there, or a mark that it was.
	PublicationWritten
	// PublicationAbsent says an intent is left without its letter, which the
	// sweep never leaves behind for a letter that was written.
	PublicationAbsent
)

// PublicationOf says whether a letter published once to a run reached its
// mailbox. It is how an operation pinned to a run that has ended learns what
// it did there. It reads under the mailbox lock, so the letter and its mark
// are seen as one sweep left them. An error says the mailbox could not be
// read now — busy, or a lookup that failed — which is no answer at all, and a
// later look may give one; PublicationUnknown says the mailbox was read and
// keeps nothing that tells.
func PublicationOf(ctx context.Context, dir, to, epoch, id string) (Publication, error) {
	path, ok := oncePath(dir, to, epoch, id)
	if !ok {
		return PublicationUnknown, nil
	}
	answer, w := PublicationUnknown, live(dir)
	err := state.WithMailboxLock(ctx, dir, to, func() error {
		found, err := w.present(to, id)
		if found {
			answer = PublicationWritten
			return nil
		}
		if err != nil {
			return err
		}
		mark, err := w.readOnceMark(path)
		switch {
		case err != nil:
			return err
		case mark == oncePublished:
			answer = PublicationWritten
		case mark == onceIntent:
			answer = PublicationAbsent
		}
		return nil
	})
	if err != nil {
		return PublicationUnknown, err
	}
	return answer, nil
}

// sweepOnce drops the marks of runs other than the live one: a retry pinned to
// an ended run is refused inside the recipient's lock before it looks. The
// caller holds the mailbox lock and has read there that live is the name's
// live run (sweepFinished).
func sweepOnce(dir, name, live string) {
	root := filepath.Join(state.InboxPath(dir, name), "once")
	runs, err := os.ReadDir(root)
	if err != nil {
		return
	}
	kept := keptByStopOf(dir, name)
	for _, run := range runs {
		if run.Name() != live && !kept.keeps(filepath.Join(root, run.Name())) {
			_ = os.RemoveAll(filepath.Join(root, run.Name()))
		}
	}
}
