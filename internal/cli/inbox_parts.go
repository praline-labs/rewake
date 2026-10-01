package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// A read through the tool (docs/mail-bridge.md#bounded-output-and-the-read-boundary).
// The first call freezes the batch — which letters, in which version — and
// every letter is cut into parts that fit one result. A call prints a part,
// or several whole short letters, and the words that fetch the next. Nothing
// becomes read by being printed: the wrapper, having seen a result reach the
// model whole, acknowledges it (AcknowledgeRead), and a letter is marked read
// once every part of its version was acknowledged.

// readSite is the run a read belongs to.
type readSite struct {
	dir   string
	self  registry.Session
	epoch string
}

// segment is one part of one letter in a response.
type segment struct{ letter, part int }

// letterHeadModel is a letter's heading under --json: what the text part does
// not carry, repeated in every part so each envelope stands alone.
type letterHeadModel struct {
	ID            string     `json:"id"`
	From          string     `json:"from"`
	Kind          inbox.Kind `json:"kind"`
	CreatedAt     time.Time  `json:"createdAt"`
	AddendumTo    string     `json:"addendumTo,omitempty"`
	Replaces      string     `json:"replaces,omitempty"`
	GrantDirs     []string   `json:"grantDirs,omitempty"`
	GrantGit      bool       `json:"grantGit,omitempty"`
	ThreadChanged bool       `json:"threadChanged,omitempty"`
	SenderState   string     `json:"senderState,omitempty"`
	// Gone says the letter was no longer unread when this read reached it,
	// and its text is not shown.
	Gone bool `json:"gone,omitempty"`
}

type letterPartModel struct {
	Letter json.RawMessage `json:"letter"`
	Part   int             `json:"part"`
	Parts  int             `json:"parts"`
	Start  int             `json:"start"`
	End    int             `json:"end"`
	Total  int             `json:"total"`
	Text   string          `json:"text"`
}

type readPartModel struct {
	Session   string            `json:"session"`
	Receipt   string            `json:"receipt"`
	Replay    bool              `json:"replay,omitempty"`
	Letters   []letterPartModel `json:"letters"`
	NextWords []string          `json:"nextWords,omitempty"`
}

// readReserve is kept free in every part for the lines a response may add:
// the replay note and the closing line.
const readReserve = 400

// bridgeRead starts an explicit read under a tool call, or joins the one the
// same words started earlier in this turn: its letters again, the same nothing
// if it found none, the same refusal if it was refused. Only a read the lock,
// the disk or a stop of the mailbox kept from freezing froze nothing, and those
// words try again.
func bridgeRead(ctx *Context, mode inboxMode, site readSite) error {
	scope := ctx.scope
	key := receipt.Key{Epoch: site.epoch, Conversation: scope.ticket.Conversation, Turn: scope.ticket.Turn, Digest: scope.digest}
	record, joined, err := receipt.Begin(site.dir, site.self.Name, key, receipt.Record{Words: scope.words, Transport: scope.ticket.Transport, CalledBoot: scope.ticket.CalledBoot})
	if err != nil {
		return failf("could not journal the read, so nothing was shown: %v", err)
	}
	release, err := lockOperation(ctx, site, record.Token)
	if err != nil {
		return err
	}
	defer release()
	if record, err = receipt.Load(site.dir, site.self.Name, site.epoch, record.Token); err != nil {
		return failf("could not read the receipt of this read: %v", err)
	}
	if record.Read == nil && record.Outcome == nil {
		refusal, err := freezeRead(ctx, mode, site, &record)
		if err != nil {
			return err
		}
		if refusal != nil {
			return keepRefusal(ctx, site, &record, refusal)
		}
		joined = false
	}
	return emitReadLocked(ctx, site, &record, 0, 0, joined)
}

// keepRefusal records a read's refusal as its answer, which the same words
// get again, and prints it.
func keepRefusal(ctx *Context, site readSite, record *receipt.Record, refusal error) error {
	var out, errOut bytes.Buffer
	code := report(&Context{Stdout: &out, Stderr: &errOut, JSON: ctx.JSON}, refusal)
	record.Phase = receipt.Done
	record.Outcome = &receipt.Outcome{Exit: code, Stdout: out.String(), Stderr: errOut.String()}
	if err := receipt.Save(site.dir, site.self.Name, *record); err != nil {
		return failf("could not journal the read: %v", err)
	}
	_, _ = ctx.Stdout.Write(out.Bytes())
	_, _ = ctx.Stderr.Write(errOut.Bytes())
	return &ExitCodeError{Code: code}
}

// freezeRead selects the letters of a read and keeps them in the record. It
// answers a refusal to keep as the read's answer, or an error that froze
// nothing, a stop among them.
func freezeRead(ctx *Context, mode inboxMode, site readSite, record *receipt.Record) (error, error) {
	var refusal, stop error
	wait, cancel := context.WithTimeout(context.Background(), readerLockWait)
	defer cancel()
	err := state.WithMailboxLock(wait, site.dir, site.self.Name, func() error {
		if err := mailAdmitted(site.dir, site.self.Name, site.epoch); err != nil {
			refusal = err
			return nil
		}
		// A stop is not the read's answer: it lasts until a look finds its
		// cause gone, and the same words then read.
		if stop = mailboxStopped(site.dir, site.self.Name); stop != nil {
			return nil
		}
		messages, err := inbox.AvailableUnread(site.dir, site.self.Name, site.epoch)
		if err != nil {
			return fmt.Errorf("could not read the inbox of %s: %w", site.self.Name, err)
		}
		if mode.selected != "" {
			messages = only(messages, mode.selected)
			if len(messages) == 0 {
				refusal = failf("message %q is not available in this session's unread inbox (unknown, not yet announced, reserved or already read); run rewake inbox --peek", mode.selected)
				return nil
			}
		}
		batch := &receipt.ReadBatch{JSON: ctx.JSON}
		for index, view := range viewedMessages(site.dir, messages) {
			letter, err := freezeLetter(ctx.JSON, view, messages[index])
			if err != nil {
				return err
			}
			batch.Letters = append(batch.Letters, letter)
		}
		record.Read, record.Phase = batch, receipt.Done
		return receipt.Save(site.dir, site.self.Name, *record)
	})
	if errors.Is(err, state.ErrMailboxBusy) {
		return nil, failf("the inbox of %s is being read by another command right now; run the same words again in a moment", site.self.Name)
	}
	if stop != nil {
		return nil, stop
	}
	if err != nil {
		return nil, failf("could not freeze the read, so nothing was shown: %v", err)
	}
	return refusal, nil
}

func only(messages []inbox.Message, id string) []inbox.Message {
	for _, message := range messages {
		if message.ID == id {
			return []inbox.Message{message}
		}
	}
	return nil
}

// freezeLetter keeps one letter as it stands, cut into parts.
func freezeLetter(asJSON bool, view messageView, stored inbox.Message) (receipt.Letter, error) {
	raw, err := json.Marshal(stored)
	if err != nil {
		return receipt.Letter{}, err
	}
	sum := sha256.Sum256(raw)
	letter := receipt.Letter{ID: stored.ID, Version: hex.EncodeToString(sum[:8]), Message: raw, Body: letterBody(view)}
	if asJSON {
		head := letterHeadModel{
			ID: view.ID, From: view.From, Kind: inbox.KindOf(view.Message), CreatedAt: view.CreatedAt,
			AddendumTo: view.AddendumTo, Replaces: view.Replaces, GrantDirs: view.GrantDirs, GrantGit: view.GrantGit,
			ThreadChanged: view.ThreadChanged,
		}
		if view.Telemetry != nil {
			head.SenderState = stateLine(view.From, view.Telemetry, view.Availability != nil || view.Departure != nil)
		}
		encoded, _ := json.Marshal(head)
		letter.Head, letter.Body = string(encoded), view.Text
	} else {
		letter.Head = strings.Join(letterHead(view), "\n")
	}
	// Each part is sized so that a response carrying it alone fits one
	// result, with the room the closing lines may take.
	sizing := receipt.Record{Token: strings.Repeat("0", 24), Read: &receipt.ReadBatch{JSON: asJSON}}
	reserve := strings.Repeat("x", readReserve)
	for start := 0; start < len(letter.Body) || start == 0; {
		end := bridge.Cut(letter.Body, start, func(end int) bool {
			trial := letter
			trial.Parts = []receipt.Part{{Start: start, End: end}, {Start: end, End: end}}
			sizing.Read.Letters = []receipt.Letter{trial, trial}
			return bridge.Fits(renderRead(asJSON, "", sizing, []segment{{0, 0}}, false), reserve)
		})
		if end <= start && len(letter.Body) > 0 {
			return receipt.Letter{}, errors.New("a part of the letter would not fit one result")
		}
		letter.Parts = append(letter.Parts, receipt.Part{Start: start, End: end})
		if end >= len(letter.Body) {
			break
		}
		start = end
	}
	return letter, nil
}

// lockOperation holds a record for as long as the call's answer is wanted.
func lockOperation(ctx *Context, site readSite, token string) (func(), error) {
	wait := readerLockWait
	if ctx.scope != nil {
		wait = min(wait, ctx.scope.remaining())
	}
	lockCtx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	release, err := receipt.Lock(lockCtx, site.dir, site.self.Name, site.epoch, token)
	if errors.Is(err, receipt.ErrBusy) {
		return nil, &PendingError{Message: fmt.Sprintf("Rewake: the same operation is running in another call right now; its answer: rewake retry %s", token)}
	}
	if err != nil {
		return nil, failf("could not lock the receipt %s: %v", token, err)
	}
	return release, nil
}

// emitRead prints the part a continuation names, or the answer a read that
// froze nothing gave.
func emitRead(ctx *Context, site readSite, token string, letter, part int) error {
	release, err := lockOperation(ctx, site, token)
	if err != nil {
		return err
	}
	defer release()
	record, err := receipt.Load(site.dir, site.self.Name, site.epoch, token)
	if err != nil {
		return failf("could not read the receipt %s: %v", token, err)
	}
	return emitReadLocked(ctx, site, &record, letter, part, false)
}
