package codex

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/harness/codex/gateway"
	"github.com/praline-labs/rewake/internal/inbox"
)

type reservedDelivery struct {
	session      *serverSession
	reservation  *gateway.Reservation
	ctx          context.Context
	cancel       context.CancelFunc
	rootsChecked bool
	// read is the thread read Reserve took for a notice carrying a grant,
	// which planning the roots uses rather than reading again.
	read   *threadRead
	change rootsChange
}

func (s *serverSession) Reserve(ctx context.Context, message inbox.Message) (inbox.Reservation, error) {
	if s.gateway == nil {
		return nil, inbox.ErrThreadUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	reserved, err := s.gateway.Reserve(ctx)
	if err != nil {
		cancel()
		return nil, reserveRefusal(err)
	}
	delivery := &reservedDelivery{session: s, reservation: reserved, ctx: ctx, cancel: cancel}
	if s.confirming(reserved.Thread()) {
		// The conversation reserved, not the one selected before: Reserve
		// may wait for a resume to finish, or the person may switch. The
		// mains may take longer to answer than a reservation lives, so it is
		// let go while they are asked.
		delivery.Close()
		return nil, fmt.Errorf("%w: %s", inbox.ErrNotYet, resumeWait)
	}
	if s.appliesGrant(message) {
		// Asked before the message becomes readable: a task read while it
		// waits would go without its grant.
		read := readThreadRoots(ctx, reserved.ReadThread)
		if read.err == nil && read.status == "active" {
			delivery.Close()
			return nil, fmt.Errorf("%w: %s", inbox.ErrNotYet, idleWait)
		}
		if read.err != nil && grantsDirs(message) {
			// A read that failed may pass: the task waits, within its time
			// to live, rather than go without its grant or fail for good.
			// Git metadata alone goes without, with a note, as it did.
			delivery.Close()
			return nil, fmt.Errorf("%w: the thread's roots could not be read yet: %v", inbox.ErrNotYet, read.err)
		}
		delivery.read = &read
	}
	return delivery, nil
}

// reserveRefusal tells the inbox a refusal that passes with a running
// compaction, or with the person's word on the conversation, so the message
// stays pending, from one that fails the delivery.
func reserveRefusal(err error) error {
	if errors.Is(err, gateway.ErrCompacting) || errors.Is(err, gateway.ErrUnintended) {
		return fmt.Errorf("%w: %v", inbox.ErrNotYet, err)
	}
	return err
}
func (r *reservedDelivery) Close() { r.reservation.Close(); r.cancel() }
func (r *reservedDelivery) Prepare(fn func(string) error) error {
	entered := false
	err := r.reservation.Prepare(func(thread string) error { entered = true; return fn(thread) })
	if err != nil && !entered && errors.Is(err, gateway.ErrLapsed) {
		// Nothing was made readable or sent: the notice waits for another
		// reservation rather than failing for good.
		return fmt.Errorf("%w: %v", inbox.ErrNotYet, err)
	}
	if err != nil && !entered {
		return fmt.Errorf("%w: %v", inbox.ErrThreadUnavailable, err)
	}
	return err
}

func (r *reservedDelivery) Deliver(_ context.Context, message inbox.Message) inbox.Result {
	return r.DeliverChecked(r.ctx, message, func() bool { return true })
}

func (r *reservedDelivery) DeliverChecked(_ context.Context, message inbox.Message, valid func() bool) inbox.Result {
	var thread string
	if err := r.Prepare(func(value string) error { thread = value; return nil }); err != nil {
		if errors.Is(err, inbox.ErrNotYet) {
			return inbox.Result{State: inbox.Pending, Detail: err.Error()}
		}
		return inbox.Result{State: inbox.Failed, Detail: err.Error()}
	}
	if !r.rootsChecked {
		if err := r.checkRoots(thread, message); err != nil {
			return inbox.Result{State: inbox.Failed, Detail: err.Error()}
		}
	}
	if !valid() {
		return inbox.Result{State: inbox.Pending, Detail: "announcement membership changed before send"}
	}
	_, err := r.reservation.Deliver(r.ctx, message.ID, mailboxNotice(message), r.change.roots)
	if errors.Is(err, gateway.ErrCompacting) {
		// The server refused the notice for a compaction running: nothing was
		// taken, and the message goes once the compaction has ended.
		return inbox.Result{State: inbox.Pending, Detail: reserveRefusal(err).Error()}
	}
	if errors.Is(err, gateway.ErrNotSent) {
		// Nothing reached the server, and the grants planned for the notice
		// were neither applied nor journaled: it goes again as it is.
		return inbox.Result{State: inbox.Pending, Detail: err.Error()}
	}
	if err != nil {
		return inbox.Result{State: inbox.Failed, Detail: err.Error() + "; delivery was not retried automatically"}
	}
	notes := r.change.notes
	if note := r.session.saveJournal(r.change.journal); note != "" {
		notes = append(notes, note)
	}
	r.session.follow(thread, r.change.followed)
	return inbox.Result{State: inbox.Delivered, Via: "app-server", Detail: strings.Join(notes, "; "), GrantApplied: r.change.applied}
}

func (r *reservedDelivery) checkRoots(thread string, message inbox.Message) error {
	git := false
	var granted inbox.Message
	members := message.Batch
	if len(members) == 0 {
		members = []inbox.Message{message}
	}
	for _, member := range members {
		kind := inbox.KindOf(member)
		work := kind == inbox.Task || kind == inbox.Question
		if member.GrantGit {
			if !r.session.gitWrite || !work {
				return fmt.Errorf("explicit Git grant is unsupported for this recipient or message kind")
			}
			git = true
		}
		if len(member.GrantDirs) > 0 {
			if !work || len(members) > 1 {
				return fmt.Errorf("a directory grant goes only with a task or a question, announced on its own")
			}
			granted = member
		}
	}
	if (git || granted.ID != "") && r.session.legacyLandlock != "" {
		return errors.New(legacyLandlockRefusal(r.session.legacyLandlock))
	}
	read := func() threadRead {
		if r.read != nil {
			return *r.read
		}
		return readThreadRoots(r.ctx, r.reservation.ReadThread)
	}
	change, err := r.session.planRoots(thread, read, git, granted)
	if err != nil {
		return err
	}
	r.change, r.rootsChecked = change, true
	return nil
}

func (s *serverSession) Deliver(ctx context.Context, message inbox.Message) inbox.Result {
	reserved, err := s.Reserve(ctx, message)
	if errors.Is(err, inbox.ErrNotYet) {
		return inbox.Result{State: inbox.Pending, Detail: err.Error()}
	}
	if err != nil {
		return inbox.Result{State: inbox.Failed, Detail: err.Error()}
	}
	defer reserved.Close()
	if message.DeliveryThread != "" {
		if err := reserved.Prepare(func(thread string) error {
			if thread != message.DeliveryThread {
				return inbox.ErrThreadUnavailable
			}
			return nil
		}); err != nil {
			return inbox.Result{State: inbox.Failed, Detail: err.Error()}
		}
	}
	return reserved.Deliver(ctx, message)
}

func mailboxNotice(message inbox.Message) gateway.MailboxNotice {
	members := message.Batch
	if len(members) == 0 {
		members = []inbox.Message{message}
	}
	notice := gateway.MailboxNotice{Notice: noticePrefix(message) + " " + harness.Notice(message)}
	for _, member := range members {
		entry := gateway.MailboxMember{ID: member.ID, From: member.From, FromEpoch: member.FromEpoch, To: member.To, ToEpoch: member.ToEpoch}
		if member.Recall != nil {
			entry.Recalls = member.Recall.ID
		}
		entry.Replaces = member.Replaces
		notice.Members = append(notice.Members, entry)
	}
	return notice
}

// appliesGrant says whether a notice carries a grant this session applies,
// and so waits for it to be idle. One it refuses is refused when its roots
// are planned, without a read.
func (s *serverSession) appliesGrant(message inbox.Message) bool {
	members := message.Batch
	if len(members) == 0 {
		members = []inbox.Message{message}
	}
	for _, member := range members {
		kind := inbox.KindOf(member)
		if kind != inbox.Task && kind != inbox.Question {
			continue
		}
		if member.GrantGit && s.gitWrite || len(member.GrantDirs) > 0 {
			return true
		}
	}
	return false
}

// grantsDirs says whether a notice carries a directory grant.
func grantsDirs(message inbox.Message) bool {
	return len(message.GrantDirs) > 0 || slices.ContainsFunc(message.Batch, func(member inbox.Message) bool { return len(member.GrantDirs) > 0 })
}
