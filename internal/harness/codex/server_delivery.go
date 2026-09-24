package codex

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/harness/codex/gateway"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

type reservedDelivery struct {
	session      *serverSession
	reservation  *gateway.Reservation
	ctx          context.Context
	cancel       context.CancelFunc
	rootsChecked bool
	roots        []string
	rootNote     string
}

func (s *serverSession) Reserve(ctx context.Context, _ inbox.Message) (inbox.Reservation, error) {
	if s.gateway == nil {
		return nil, inbox.ErrThreadUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	reserved, err := s.gateway.Reserve(ctx)
	if err != nil {
		cancel()
		return nil, reserveRefusal(err)
	}
	return &reservedDelivery{session: s, reservation: reserved, ctx: ctx, cancel: cancel}, nil
}

// reserveRefusal tells the inbox a refusal that passes with a running
// compaction, so the message stays pending and goes after it, from one that
// fails the delivery.
func reserveRefusal(err error) error {
	if errors.Is(err, gateway.ErrCompacting) {
		return fmt.Errorf("%w: %v", inbox.ErrNotYet, err)
	}
	return err
}
func (r *reservedDelivery) Close() { r.reservation.Close(); r.cancel() }
func (r *reservedDelivery) Prepare(fn func(string) error) error {
	entered := false
	err := r.reservation.Prepare(func(thread string) error { entered = true; return fn(thread) })
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
	_, err := r.reservation.Deliver(r.ctx, message.ID, mailboxNotice(message), r.roots)
	if err != nil {
		return inbox.Result{State: inbox.Failed, Detail: err.Error() + "; delivery was not retried automatically"}
	}
	return inbox.Result{State: inbox.Delivered, Via: "app-server", Detail: r.rootNote}
}

func (r *reservedDelivery) checkRoots(thread string, message inbox.Message) error {
	grantKind := inbox.Note
	members := message.Batch
	if len(members) == 0 {
		members = []inbox.Message{message}
	}
	for _, member := range members {
		if member.GrantGit {
			kind := inbox.KindOf(member)
			if !r.session.gitWrite || kind != inbox.Task && kind != inbox.Question {
				return fmt.Errorf("explicit Git grant is unsupported for this recipient or message kind")
			}
			grantKind = kind
		}
	}
	r.roots, r.rootNote = r.session.taskGitRoots(r.ctx, r.reservation.ReadThread, thread, grantKind)
	r.rootsChecked = true
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
		notice.Members = append(notice.Members, gateway.MailboxMember{ID: member.ID, From: member.From, FromEpoch: member.FromEpoch, To: member.To, ToEpoch: member.ToEpoch})
	}
	return notice
}
