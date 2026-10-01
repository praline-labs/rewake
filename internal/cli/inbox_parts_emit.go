package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/state"
)

// emitReadLocked prints a response of a frozen read starting at one part, and
// records which call carried it before printing. joined says the words found
// this read already made. The caller holds the record.
func emitReadLocked(ctx *Context, site readSite, record *receipt.Record, letter, part int, joined bool) error {
	batch := record.Read
	if batch == nil {
		// Every way into a read comes here, so a refused one is replayed
		// here, whichever words asked again.
		if record.Outcome != nil {
			return replay(ctx, &operation{site: site, record: *record})
		}
		return failf("the read of the receipt %s froze nothing yet; run its words again", record.Token)
	}
	if len(batch.Letters) == 0 {
		_, err := io.WriteString(ctx.Stdout, renderNothing(batch.JSON, site.self.Name, *record, joined))
		return err
	}
	if letter >= len(batch.Letters) || part >= len(batch.Letters[letter].Parts) {
		return failf("%s names no part of this read; take the token from the words the read printed", nextToken(record.Token, letter, part))
	}
	replay := joined && shownAny(batch)
	shown := receipt.Shown{Transport: receipt.Shell}
	if ctx.scope != nil {
		shown = receipt.Shown{CallID: ctx.scope.ticket.CallID, Transport: ctx.scope.ticket.Transport, CalledBoot: ctx.scope.ticket.CalledBoot}
	}
	var segments []segment
	var text string
	var late error
	wait, cancel := context.WithTimeout(context.Background(), readerLockWait)
	defer cancel()
	err := state.WithMailboxLock(wait, site.dir, site.self.Name, func() error {
		// Under the lock, just before anything is shown or claimed.
		if late = beforeCommit(ctx, record.Token, "the letters were shown"); late != nil {
			return nil
		}
		// A read frozen before the mailbox stopped shows and claims nothing
		// after it (8-stop).
		if late = mailboxStopped(site.dir, site.self.Name); late != nil {
			return nil
		}
		changed, err := refresh(site, batch, letter)
		if err != nil {
			return err
		}
		if changed {
			// Another version from its first part: the one asked for
			// belongs to a text that is no longer the letter's.
			part = 0
		}
		segments = []segment{{letter, part}}
		if part == len(batch.Letters[letter].Parts)-1 {
			// Whole short letters that follow go along while they fit; one
			// that could not be looked up waits for a response of its own.
			for next := letter + 1; next < len(batch.Letters); next++ {
				if _, err := refresh(site, batch, next); err != nil || len(batch.Letters[next].Parts) != 1 {
					break
				}
				trial := append(append([]segment{}, segments...), segment{next, 0})
				if bodyBytes(batch, trial) > bridge.BodyCap || !bridge.Fits(renderRead(batch.JSON, site.self.Name, *record, trial, replay), strings.Repeat("x", readReserve)) {
					break
				}
				segments = trial
			}
		}
		text = renderRead(batch.JSON, site.self.Name, *record, segments, replay)
		if !bridge.Fits(text, "") {
			// Sized when frozen, so this is a defect; a part the bound
			// would change must not count as shown.
			return errors.New("the part does not fit one tool result")
		}
		if ctx.scope != nil {
			// The whole answer, by digest: a part counts as shown only on a
			// result that is exactly this text (rule 6). The call prints it
			// and nothing else.
			shown.Answer = bridge.AnswerDigest([]byte(text))
		}
		for _, segment := range segments {
			current := &batch.Letters[segment.letter]
			if !current.Read && !tombstone(*current) && inbox.StillUnread(site.dir, site.self.Name, current.ID) {
				if err := inbox.ClaimRead(site.dir, site.self.Name, current.ID, record.Token); err != nil {
					return err
				}
			}
			current.Parts[segment.part].Shown = append(current.Parts[segment.part].Shown, shown)
		}
		return receipt.Save(site.dir, site.self.Name, *record)
	})
	if late != nil {
		return late
	}
	if errors.Is(err, state.ErrMailboxBusy) {
		return failf("the inbox of %s is busy right now, so nothing was shown; run the same words again in a moment", site.self.Name)
	}
	if err != nil {
		return failf("could not record the read before showing it, so nothing was shown: %v", err)
	}
	if _, err := io.WriteString(ctx.Stdout, text); err != nil {
		return failf("could not print the letters; they stay unread and show again: %v", err)
	}
	if ctx.scope == nil {
		// A shell has no observer to confirm what reached the agent; its
		// trust stays what it always was: printed is read.
		printed := map[segment]bool{}
		for _, segment := range segments {
			printed[segment] = true
		}
		return settleLetters(site, record, func(letter, part int, shown receipt.Shown) bool {
			return printed[segment{letter, part}] && shown.Transport == receipt.Shell
		})
	}
	return nil
}

func shownAny(batch *receipt.ReadBatch) bool {
	for _, letter := range batch.Letters {
		if letter.Started() {
			return true
		}
	}
	return false
}

// refresh brings a letter no part of which was shown yet to what the mailbox
// holds now, whichever part is asked for first, and answers whether it
// changed. A withdrawal may win until then, and the reader sees the tombstone;
// a letter no longer unread at all — read through another call, or taken out
// by its delivery's end — shows a line saying so, and is not marked again. An
// error says the letter could not be looked up, and then nothing of it may be
// shown: the frozen text may be one withdrawn since. The caller holds the
// mailbox lock.
func refresh(site readSite, batch *receipt.ReadBatch, index int) (bool, error) {
	letter := &batch.Letters[index]
	if letter.Started() || letter.Read {
		return false, nil
	}
	current, found, err := unreadCopy(site, letter.ID)
	if err != nil {
		return false, fmt.Errorf("could not look up %s before showing it: %w", letter.ID, err)
	}
	if !found {
		*letter = goneLetter(batch.JSON, *letter)
		return true, nil
	}
	views := viewedMessages(site.dir, []inbox.Message{current})
	fresh, err := freezeLetter(batch.JSON, views[0], current)
	if err != nil {
		return false, fmt.Errorf("could not take %s as it stands now: %w", letter.ID, err)
	}
	if fresh.Version == letter.Version {
		return false, nil
	}
	*letter = fresh
	return true, nil
}

// goneLetter is what a frozen letter shows once it is no longer unread.
func goneLetter(asJSON bool, letter receipt.Letter) receipt.Letter {
	var message inbox.Message
	_ = json.Unmarshal(letter.Message, &message)
	gone := receipt.Letter{ID: letter.ID, Version: "gone", Message: letter.Message, Parts: []receipt.Part{{}}, Read: true}
	if asJSON {
		encoded, _ := json.Marshal(letterHeadModel{ID: message.ID, From: message.From, Kind: inbox.KindOf(message), CreatedAt: message.CreatedAt, Gone: true})
		gone.Head = string(encoded)
	} else {
		gone.Head = fmt.Sprintf("Rewake: %s from %s is no longer unread — read through another call, or its delivery ended — so its text is not shown here.", message.ID, message.From)
	}
	return gone
}

// unreadCopy is a letter as the mailbox holds it now, a withdrawn one as its
// tombstone.
func unreadCopy(site readSite, id string) (inbox.Message, bool, error) {
	return inbox.UnreadCopy(site.dir, site.self.Name, site.epoch, id)
}

func tombstone(letter receipt.Letter) bool {
	var message inbox.Message
	return json.Unmarshal(letter.Message, &message) == nil && message.Withdrawn != nil
}

func bodyBytes(batch *receipt.ReadBatch, segments []segment) int {
	total := 0
	for _, segment := range segments {
		part := batch.Letters[segment.letter].Parts[segment.part]
		total += part.End - part.Start
	}
	return total
}

// following is the position after a response, or false at the end.
func following(batch *receipt.ReadBatch, last segment) (segment, bool) {
	if last.part+1 < len(batch.Letters[last.letter].Parts) {
		return segment{last.letter, last.part + 1}, true
	}
	if last.letter+1 < len(batch.Letters) {
		return segment{last.letter + 1, 0}, true
	}
	return segment{}, false
}

// renderNothing is the answer of a read that found no letters: the same
// nothing to the same words for the rest of the turn.
func renderNothing(asJSON bool, session string, record receipt.Record, joined bool) string {
	if asJSON {
		encoded, _ := json.Marshal(readPartModel{Session: session, Receipt: record.Token, Replay: joined, Letters: []letterPartModel{}})
		return string(encoded) + "\n"
	}
	if joined {
		return "Rewake: these words ran earlier in this turn and found no new messages; they answer the same now. For letters that came since: rewake inbox --peek, then rewake inbox --message <id>.\n"
	}
	return "Rewake: no new messages; receipt " + record.Token + ".\n"
}

// renderRead prints one response of a read.
func renderRead(asJSON bool, session string, record receipt.Record, segments []segment, replay bool) string {
	batch := record.Read
	next, more := following(batch, segments[len(segments)-1])
	if asJSON {
		model := readPartModel{Session: session, Receipt: record.Token, Replay: replay}
		for _, segment := range segments {
			letter := batch.Letters[segment.letter]
			part := letter.Parts[segment.part]
			model.Letters = append(model.Letters, letterPartModel{
				Letter: json.RawMessage(letter.Head), Part: segment.part + 1, Parts: len(letter.Parts),
				Start: part.Start, End: part.End, Total: len(letter.Body), Text: letter.Body[part.Start:part.End],
			})
		}
		if more {
			model.NextWords = nextWords(record.Token, next.letter, next.part)
		}
		encoded, _ := json.Marshal(model)
		return string(encoded) + "\n"
	}
	var lines []string
	if replay {
		lines = append(lines, "Rewake: these words ran earlier in this turn, so this is their read again; for letters that came since: rewake inbox --peek, then rewake inbox --message <id>.", "")
	}
	for index, segment := range segments {
		letter := batch.Letters[segment.letter]
		part := letter.Parts[segment.part]
		if index > 0 {
			lines = append(lines, "")
		}
		if segment.part == 0 {
			lines = append(lines, letter.Head)
		} else {
			lines = append(lines, fmt.Sprintf("Rewake: %s continues.", letter.ID))
		}
		if part.End > part.Start {
			lines = append(lines, letter.Body[part.Start:part.End])
		}
		// Every part names itself, a whole letter as much as a slice: the
		// wrapper and a reader match what arrived by these, not by order.
		place := fmt.Sprintf("Rewake: %s, part %d of %d, bytes %d–%d of %d", letter.ID, segment.part+1, len(letter.Parts), part.Start, part.End, len(letter.Body))
		if segment.part+1 < len(letter.Parts) {
			place += "; it is not read until every part has reached you"
		}
		lines = append(lines, place+".")
	}
	if more {
		lines = append(lines, "Rewake: the next: rewake "+strings.Join(nextWords(record.Token, next.letter, next.part), " "))
	} else {
		lines = append(lines, "Rewake: end of this read, receipt "+record.Token+".")
	}
	return strings.Join(lines, "\n") + "\n"
}
