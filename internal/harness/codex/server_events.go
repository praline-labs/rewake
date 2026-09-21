package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// The protocol reader only enqueues. Disk writes and mailbox locks belong to the
// publisher; a callback is not a report receipt until emit returns successfully.
func (s *serverSession) queueCompletion(result harness.Completion) {
	if s.capture != nil && result.Boundary == nil {
		result.Boundary = s.capture()
	}
	if result.Boundary != nil {
		boundary := *result.Boundary
		result.Boundary = &boundary
	}
	s.mu.Lock()
	s.outcomes = append(s.outcomes, result)
	s.reportVersion++
	bytes := 0
	for _, outcome := range s.outcomes {
		bytes += len(outcome.Text)
	}
	overflow := len(s.outcomes) > 256 || bytes > 16<<20
	s.mu.Unlock()
	if overflow {
		s.reportOverflow.Do(func() { s.cancel(); go s.gateway.Close() })
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *serverSession) persistOutcomes() error {
	s.mu.Lock()
	if s.reportVersion == s.reportSaved {
		s.mu.Unlock()
		return nil
	}
	version := s.reportVersion
	pending := append([]harness.Completion(nil), s.outcomes...)
	s.mu.Unlock()
	raw, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	path := s.path + ".outcomes.json"
	if err = state.WriteAtomic(path, raw); err != nil {
		return err
	}
	s.mu.Lock()
	s.reportSaved = version
	s.mu.Unlock()
	return nil
}

// One deadline covers the complete shutdown drain, including an already-running
// publish. A late successful callback cannot dequeue its still-journaled head.
//
// The five seconds below are the largest stage of an ordinary shutdown, and the
// workflow suite's termination budget (test/workflow) is the sum of those
// stages: a change here has to be reflected there, or the suite starts killing
// sessions before this drain can finish.
func (s *serverSession) report(ctx context.Context) {
	defer close(s.stopped)
	publishCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	budgetDone := make(chan struct{})
	defer close(budgetDone)
	go func() {
		select {
		case <-ctx.Done():
		case <-budgetDone:
			return
		}
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
			cancel()
		case <-budgetDone:
		}
	}()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if publishCtx.Err() != nil {
			if err := s.persistOutcomes(); err != nil {
				s.note("completion journal could not be saved: " + err.Error())
			}
			s.note("completion publication remains pending in " + s.path + ".outcomes.json")
			return
		}
		if err := s.persistOutcomes(); err != nil {
			if ctx.Err() != nil {
				s.note("completion publication could not be persisted: " + err.Error())
				return
			}
		} else {
			s.mu.Lock()
			if s.reportSaved != s.reportVersion {
				s.mu.Unlock()
				continue
			}
			empty := len(s.outcomes) == 0
			var result harness.Completion
			if !empty {
				result = s.outcomes[0]
			}
			s.mu.Unlock()
			if empty {
				if ctx.Err() != nil {
					return
				}
			} else if s.publishBound(publishCtx, result) == nil {
				s.mu.Lock()
				s.outcomes = s.outcomes[1:]
				s.reportVersion++
				s.mu.Unlock()
				continue
			}
		}
		select {
		case <-publishCtx.Done():
		case <-ctx.Done():
			if ctx.Err() != nil {
				select {
				case <-publishCtx.Done():
				case <-ticker.C:
				}
			}
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

func (s *serverSession) publishBound(ctx context.Context, result harness.Completion) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.emit == nil {
		return nil
	}
	if result.Boundary != nil {
		boundary := *result.Boundary
		result.Boundary = &boundary
	}
	done := make(chan error, 1)
	go func() { done <- s.emit(ctx, result) }()
	select {
	case err := <-done:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// A same-epoch restart can retry captured callbacks; it never borrows a new run's waits.
func (s *serverSession) restoreOutcomes() error {
	path := s.path + ".outcomes.json"
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 32<<20 {
		return fmt.Errorf("invalid completion journal %s", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &s.outcomes); err != nil {
		return err
	}
	s.reportVersion = 1
	return nil
}
