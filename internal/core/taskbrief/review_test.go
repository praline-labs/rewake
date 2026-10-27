package taskbrief

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

var reviewer = Author{Name: "review-one", Epoch: "400.4.boot"}

// A run given the brief to review reads and comments, never ticks, and its
// comments say so; given it again as work, the same run ticks. The owner's
// decision of October 27, 2026 (docs/briefs.md).
func TestAReviewBindingCommentsAndIsRefusedATick(t *testing.T) {
	dir := room(t)
	home := Home{Room: dir}
	brief := made(t, dir, "- [ ] one\n")
	if err := Bind(home, brief.ID, reviewer, Review); err != nil {
		t.Fatal(err)
	}
	var review *ReviewOnlyError
	if _, err := Tick(home, brief.ID, reviewer, 1, 1, true); !errors.As(err, &review) {
		t.Fatalf("a reviewer's tick: %v", err)
	}
	if read := loaded(t, dir, brief.ID); read.Done() != 0 || len(read.Log) != 1 {
		t.Fatalf("the refused tick wrote: %+v", read.Log)
	}
	if entry, _, err := Comment(home, brief.ID, reviewer, "item 1 is not done"); err != nil || entry.Capacity != Review {
		t.Fatalf("a reviewer's comment: %+v, %v", entry, err)
	}
	if entry, _, err := Comment(home, brief.ID, worker, "on it"); err != nil || entry.Capacity != Work {
		t.Fatalf("a worker's comment: %+v, %v", entry, err)
	}

	if err := Bind(home, brief.ID, reviewer, Work); err != nil {
		t.Fatal(err)
	}
	read := loaded(t, dir, brief.ID)
	if binding, _ := read.BindingOf(reviewer); len(read.Bound) != 2 || binding.Kind != Work {
		t.Fatalf("given again as work, the bindings are %+v", read.Bound)
	}
	if _, err := Tick(home, brief.ID, reviewer, 1, 1, true); err != nil {
		t.Fatalf("the run's tick once given as work: %v", err)
	}
	if comments := loaded(t, dir, brief.ID).Comments(); comments[0].Capacity != Review {
		t.Fatalf("an earlier comment's capacity changed with the binding: %+v", comments[0])
	}
}

// A brief written before kinds — a binding without one, a comment without a
// capacity — reads as work, and its comment as no capacity.
func TestAnOldBindingReadsAsWork(t *testing.T) {
	dir := room(t)
	home := Home{Room: dir}
	brief := made(t, dir, "- [ ] one\n")
	path := filepath.Join(state.BriefsPath(dir), FormatID(brief.ID))
	record := loaded(t, dir, brief.ID).Record
	record.Bound[0].Kind = ""
	err := state.WithBriefLock(dir, func() error {
		if err := writeRecord(path, record); err != nil {
			return err
		}
		return publish(path, Entry{N: 2, At: time.Now().UTC(), From: worker.Name, Epoch: worker.Epoch, Kind: KindComment, Edition: 1, Text: "old"})
	})
	if err != nil {
		t.Fatal(err)
	}
	read := loaded(t, dir, brief.ID)
	if binding, bound := read.BindingOf(worker); !bound || binding.Kind != "" || binding.Kind.Of() != Work {
		t.Fatalf("the old binding: %+v", read.Bound)
	}
	if comments := read.Comments(); len(comments) != 1 || comments[0].Capacity != "" {
		t.Fatalf("the old comment: %+v", comments)
	}
	if _, err := Tick(home, brief.ID, worker, 1, 1, true); err != nil {
		t.Fatalf("an old binding's tick: %v", err)
	}
}
