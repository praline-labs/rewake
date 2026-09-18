package codex

import (
	"context"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// PublisherForTest exercises the actual queue/journal, without a native process.
func PublisherForTest(path string, handler harness.CompletionHandler) (func(harness.Completion), func()) {
	s := newServer(path, nil, nil, "")
	s.emit = handler.Publish
	s.capture = handler.Capture
	s.note = func(string) {}
	ctx, cancel := context.WithCancel(context.Background())
	go s.report(ctx)
	return s.queueCompletion, func() { cancel(); <-s.stopped }
}
