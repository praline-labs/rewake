package pi

import (
	"context"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

func (r *run) Thread() (string, error) {
	l, thread := r.connection()
	if l == nil {
		return "", mail.ErrThreadUnavailable
	}
	return thread, nil
}

func (r *run) Deliver(ctx context.Context, message mail.Message) mail.Result {
	l, thread := r.connection()
	if l == nil {
		return mail.Result{State: mail.Pending, Detail: "the extension has not connected"}
	}
	if message.DeliveryThread != "" && message.DeliveryThread != thread {
		return mail.Result{State: mail.Failed, Detail: "the delivery belongs to another conversation"}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	answer, err := l.ask(ctx, frame{Op: "wake", Thread: thread, Notice: mail.Notice(message)})
	if err != nil {
		return mail.Result{State: mail.Failed, Detail: "the extension did not acknowledge the notice; delivery was not retried automatically"}
	}
	if answer.Pending && !answer.OK {
		return mail.Result{State: mail.Pending, Detail: answer.Error}
	}
	if !answer.OK {
		return mail.Result{State: mail.Failed, Detail: "the extension refused the notice: " + answer.Error}
	}
	via := "pi"
	if answer.Steered {
		via += ", steered into the running turn"
	}
	return mail.Result{State: mail.Delivered, Via: via}
}
