package codex

import (
	"context"
	"fmt"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/harness/codex/gateway"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

type reservedDelivery struct {
	session     *serverSession
	reservation *gateway.Reservation
	ctx         context.Context
	cancel      context.CancelFunc
}

func (s *serverSession) Reserve(ctx context.Context, _ inbox.Message) (inbox.Reservation, error) {
	if s.gateway == nil {
		return nil, inbox.ErrThreadUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	reserved, err := s.gateway.Reserve(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	return &reservedDelivery{session: s, reservation: reserved, ctx: ctx, cancel: cancel}, nil
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
	var thread string
	if err := r.Prepare(func(value string) error { thread = value; return nil }); err != nil {
		return inbox.Result{State: inbox.Failed, Detail: err.Error()}
	}
	roots, note := r.session.taskGitRoots(r.ctx, r.reservation.ReadThread, thread, inbox.KindOf(message))
	_, err := r.reservation.Deliver(r.ctx, message.ID, noticePrefix(message)+" "+harness.Notice(message), roots)
	if err != nil {
		return inbox.Result{State: inbox.Failed, Detail: err.Error() + "; delivery was not retried automatically"}
	}
	return inbox.Result{State: inbox.Delivered, Via: "app-server", Detail: note}
}

func (s *serverSession) Deliver(ctx context.Context, message inbox.Message) inbox.Result {
	reserved, err := s.Reserve(ctx, message)
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
