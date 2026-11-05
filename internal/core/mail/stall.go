package mail

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A worker once ended a turn pending on another session's run and went idle
// with nothing of its own to wake it; the task stood until the owner asked
// after it (November 3, 2026; docs/turn-outcomes.md#a-stalled-pending). So a
// run whose last word is an interim end, with no turn since and a sender
// still waiting, is a stall: main is shown it, and after a while the run's own
// wrapper wakes it once with a letter (host/delivery/stall.go). Its mail
// evidence is on disk — the interim record, the run's ends and timed starts,
// its waits — so another image finds it too; current activity is sampled anew.

// Stall is a run idle under a pending mark.
type Stall struct {
	// Op is the interim end's operation: a later interim end is another
	// stall, and is reminded of again.
	Op string
	// Text is the pending line.
	Text string
	// Ended is when the interim turn ended, Marked when its mark was made,
	// both on the boot clock; Marked is Ended when the mark is not found.
	Ended, Marked int64
	// Senders are the sessions still waiting on a task, whose runs live.
	Senders []string
}

// IdleSince is when the interim turn ended, on the wall clock at now.
func (s Stall) IdleSince(now time.Time) time.Time { return wallAt(s.Ended, now) }

// wallAt is the wall time of a boot-clock reading, taken at now.
func wallAt(at int64, now time.Time) time.Time {
	return now.Add(-time.Duration(boottime.Now() - at))
}

// Stalled answers the run's stall, or false when it is none: its last word is
// not an interim end of this run, or that end's time is not known, or a turn
// has been heard of since — a later end on record, a timed start after it —,
// or nobody whose run lives still waits on a task. Whether the session is idle
// right now is its telemetry's to say, and the caller's to ask. It only reads.
func (b Box) Stalled() (Stall, bool, error) {
	record, err := readInterim(b.dir, b.name)
	if err != nil || record == nil || record.Epoch != b.epoch || record.Settled || record.Ended == 0 {
		return Stall{}, false, err
	}
	if latest, err := latestEndOf(b.dir, b.name, b.epoch); err != nil || latest > record.Ended {
		return Stall{}, false, err
	}
	if started, err := latestTurnStart(b.dir, b.name, b.epoch); err != nil || started > record.Ended {
		return Stall{}, false, err
	}
	waiters, err := readWaiters(b.dir, b.name, b.epoch)
	if err != nil {
		return Stall{}, false, err
	}
	senders, err := liveSenders(b.dir, waiters)
	if err != nil || len(senders) == 0 {
		return Stall{}, false, err
	}
	stall := Stall{Op: record.Op, Text: record.Text, Ended: record.Ended, Marked: record.Ended, Senders: senders}
	// The mark that made the end interim is the latest made by its end.
	if mark, ok, err := markWithin(b.dir, b.name, b.epoch, 0, record.Ended); err == nil && ok {
		stall.Marked = mark.At
	}
	return stall, true, nil
}

// StallJudgment combines the mail evidence with current session activity.
// A non-idle or stale verdict carries no stall or idle time.
type StallJudgment struct {
	Verdict   session.Verdict
	Stall     *Stall
	IdleSince *time.Time
}

// JudgeStall only reads. On an evidence error it retains the ordinary activity
// verdict, so an informing view can degrade without treating an error as absence
// when deciding a write. The caller loads and validates the snapshot's epoch.
func (b Box) JudgeStall(snapshot session.Snapshot, now time.Time) (StallJudgment, error) {
	judgment := StallJudgment{Verdict: snapshot.Judge()}
	if judgment.Verdict.Activity != session.ActivityIdle || judgment.Verdict.Stale {
		return judgment, nil
	}
	stall, ok, err := b.Stalled()
	if err != nil || !ok {
		return judgment, err
	}
	since := stall.IdleSince(now)
	judgment.Stall, judgment.IdleSince = &stall, &since
	judgment.Verdict = snapshot.JudgePending(&since)
	return judgment, nil
}

// RemindStallIfDue samples live activity and rejudges the current interim under
// the mailbox lock before publishing. No earlier stall can decide the letter.
// The returned op was published (or already sent); empty means none was due.
// Activity may still change after the live sample: the lock orders mail only.
func (l Locked) RemindStallIfDue(bound time.Duration, liveSnapshot func() session.Snapshot, now time.Time) (string, error) {
	if bound <= 0 || liveSnapshot == nil {
		return "", nil
	}
	judgment, err := l.box.JudgeStall(liveSnapshot(), now)
	if err != nil || judgment.Stall == nil {
		return "", err
	}
	stall := *judgment.Stall
	if time.Duration(boottime.Now()-stall.Ended) < bound {
		return "", nil
	}
	if err := l.box.remindStall(stall, now); err != nil {
		return "", err
	}
	return stall.Op, nil
}

// stallRecord is the reminder of one stall, beside the interim record: the
// letter's id is drawn before it is written, so a retry or a wrapper of the
// next image writes the same letter, and Sent once it is in the mailbox.
type stallRecord struct {
	Epoch string `json:"epoch"`
	Op    string `json:"op"`
	ID    string `json:"id"`
	Sent  bool   `json:"sent,omitempty"`
}

func stallPath(dir, name string) string { return filepath.Join(pendingDir(dir, name), "stalled.json") }

// remindStall puts the letter reminding the run of its stall into its own
// mailbox, once for each interim end: a stall reminded of already writes
// nothing. The letter is a notify from the run to itself, so it owes nothing
// and settles nothing; the wrapper announces it as any other. The caller holds
// the mailbox lock.
func (b Box) remindStall(stall Stall, now time.Time) error {
	path := stallPath(b.dir, b.name)
	var record stallRecord
	raw, err := readRecord(path)
	switch {
	case err == nil:
		if json.Unmarshal(raw, &record) != nil || record.Epoch != b.epoch || record.Op != stall.Op || !safeID(record.ID) {
			record = stallRecord{}
		}
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("the stall record %s cannot be read: %w", path, err)
	}
	if record.Sent {
		return nil
	}
	if record.ID == "" {
		record = stallRecord{Epoch: b.epoch, Op: stall.Op, ID: NewID()}
		if err := b.writeStallRecord(record); err != nil {
			return err
		}
	}
	letter := Message{ID: record.ID, From: b.name, FromEpoch: b.epoch, To: b.name, ToEpoch: b.epoch, Kind: Note, CreatedAt: now, Text: stallText(stall, now)}
	if err := PutOnce(b.dir, letter); err != nil {
		return err
	}
	record.Sent = true
	return b.writeStallRecord(record)
}

func (b Box) writeStallRecord(record stallRecord) error {
	if err := state.EnsureSubdir(pendingDir(b.dir, b.name)); err != nil {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return state.WriteAtomic(stallPath(b.dir, b.name), raw)
}

// stallText is the reminder: the pending line and when it was marked, who
// still waits, how long the session has been idle, and both ways on, since an
// unmarked end of the turn it wakes is the report.
func stallText(stall Stall, now time.Time) string {
	line, _, _ := strings.Cut(stall.Text, "\n")
	return fmt.Sprintf("Rewake: your pending %q since %s still holds open the task of %s, and you have been idle %s with nothing waking you. "+
		"Go on, or end the turn with the report; if the work still waits on something, run rewake pending \"<what it waits for>\" again before you end the turn.",
		line, wallAt(stall.Marked, now).Local().Format("15:04"), strings.Join(stall.Senders, ", "), session.IdleFor(now.Sub(stall.IdleSince(now))))
}
