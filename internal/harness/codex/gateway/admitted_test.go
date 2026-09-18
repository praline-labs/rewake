package gateway

import (
	"fmt"
	"testing"
	"time"
)

func scopedWork() Binding {
	return Binding{Epoch: "epoch", Connection: 7, Generation: 3, Thread: "A", Ready: true}
}

func admit(t *testing.T, a *admittedWork, id, turn string) {
	t.Helper()
	if err := a.prepare(id, scopedWork(), true); err != nil {
		t.Fatal(err)
	}
	a.ack(meta{id: id, turn: turn})
}

func workEvent(a *admittedWork, method, turn, status string, now time.Time) {
	a.event(meta{method: method, thread: "A", turn: turn, status: status}, nil, now)
}

func TestRetainedCompletionBeforeACKAndFailureDiscard(t *testing.T) {
	for _, refused := range []bool{false, true} {
		a := newAdmittedWork()
		if e := a.prepare("id", scopedWork(), true); e != nil {
			t.Fatal(e)
		}
		now := time.Now()
		workEvent(&a, "turn/started", "T", "", now)
		workEvent(&a, "turn/completed", "T", "completed", now)
		if len(a.collect()) != 0 {
			t.Fatal("unacknowledged candidate published")
		}
		a.ack(meta{id: "id", turn: "T", failure: refused})
		out := a.collect()
		if refused {
			if len(out) != 0 || len(a.threads) != 0 {
				t.Fatal("refused work retained/published", out)
			}
		} else if len(out) != 1 || out[0].ID != "A/T" || !out[0].Retained || out[0].Connection != 7 {
			t.Fatal(out)
		}
	}
}

func TestSteerLateACKCannotResurrectReportedTurn(t *testing.T) {
	a := newAdmittedWork()
	admit(t, &a, "first", "T")
	if e := a.prepare("steer", scopedWork(), true); e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	workEvent(&a, "turn/started", "T", "", now)
	workEvent(&a, "turn/completed", "T", "completed", now)
	if len(a.collect()) != 1 {
		t.Fatal("first result missing")
	}
	a.ack(meta{id: "steer", turn: "T"})
	if len(a.collect()) != 0 || len(a.threads) != 0 || len(a.pending) != 0 {
		t.Fatal("late steer ack retained settled work")
	}
}

func TestRetainedStoppedThenFinishedAndMultipleIntervalGap(t *testing.T) {
	a := newAdmittedWork()
	admit(t, &a, "first", "T")
	now := time.Now()
	workEvent(&a, "turn/started", "T", "", now)
	workEvent(&a, "turn/completed", "T", "interrupted", now)
	out := a.collect()
	if len(out) != 1 || out[0].Kind != "stopped" {
		t.Fatal(out)
	}
	if len(a.threads) != 0 || len(a.stopped) != 1 {
		t.Fatal("stopped work held a live admission slot")
	}
	workEvent(&a, "turn/completed", "T", "completed", now)
	out = a.collect()
	if len(out) != 1 || out[0].Kind != "finished" {
		t.Fatal(out)
	}
	admit(t, &a, "second", "U")
	admit(t, &a, "third", "V")
	workEvent(&a, "turn/started", "U", "", now)
	workEvent(&a, "thread/status/changed", "", "idle", now)
	workEvent(&a, "turn/started", "V", "", now)
	workEvent(&a, "turn/completed", "V", "failed", now)
	a.expire(now.Add(time.Second))
	out = a.collect()
	if len(out) != 2 {
		t.Fatal("retained earlier gap lost", out)
	}
	for _, v := range out {
		if v.Kind != "error" {
			t.Fatal(v)
		}
	}
}

func TestUnrelatedTurnAndManualMaintenanceNeverBecomeAdmittedResults(t *testing.T) {
	a := newAdmittedWork()
	admit(t, &a, "work", "T")
	now := time.Now()
	workEvent(&a, "turn/started", "T", "", now)
	a.manualStart("A", newObserver())
	workEvent(&a, "turn/completed", "T", "completed", now)
	if len(a.collect()) != 1 {
		t.Fatal("maintenance hid previously admitted task")
	}
	workEvent(&a, "turn/started", "M", "", now)
	workEvent(&a, "turn/completed", "M", "completed", now)
	if len(a.collect()) != 0 {
		t.Fatal("manual turn published")
	}
	if !a.excluded["A/M"] {
		t.Fatal("manual identity not retained for duplicate filtering")
	}
	if e := a.prepare("pending", scopedWork(), true); e != nil {
		t.Fatal(e)
	}
	workEvent(&a, "turn/started", "unrelated", "", now)
	workEvent(&a, "turn/completed", "unrelated", "completed", now)
	a.ack(meta{id: "pending", turn: "new"})
	if len(a.collect()) != 0 {
		t.Fatal("unrelated terminal event attached to ack")
	}
	workEvent(&a, "turn/completed", "new", "completed", now)
	if out := a.collect(); len(out) != 1 || out[0].ID != "A/new" {
		t.Fatal("missing start hid matching terminal event", out)
	}
}

func TestAdmittedCapacityRefusesBeforeDiscardingLiveWork(t *testing.T) {
	a := newAdmittedWork()
	for i := 0; i < 64; i++ {
		admit(t, &a, fmt.Sprint(i), fmt.Sprint(i))
	}
	if e := a.prepare("overflow", scopedWork(), true); e == nil {
		t.Fatal("unbounded admissions")
	}
	if len(a.threads["A"].turns) != 64 {
		t.Fatal("capacity refusal discarded live scopes")
	}
}

func TestStoppedContinuationACKKeepsLiveTextAndReleasesNoOtherScope(t *testing.T) {
	a := newAdmittedWork()
	admit(t, &a, "first", "T")
	if e := a.prepare("other", scopedWork(), true); e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	workEvent(&a, "turn/started", "T", "", now)
	workEvent(&a, "turn/completed", "T", "interrupted", now)
	_ = a.collect()
	if e := a.prepare("continuation", scopedWork(), true); e != nil {
		t.Fatal(e)
	}
	a.event(meta{method: "item/completed", thread: "A", turn: "T"}, []byte(`{"params":{"item":{"type":"agentMessage","text":"continued final"}}}`), now)
	a.ack(meta{id: "continuation", turn: "T"})
	workEvent(&a, "turn/completed", "T", "completed", now)
	out := a.collect()
	if len(out) != 1 || out[0].Text != "continued final" {
		t.Fatal(out)
	}
	if _, ok := a.pending["other"]; !ok {
		t.Fatal("continuation consumed unrelated pending request")
	}
}

func TestRepeatedStoppedContinuationDoesNotEvictItsCurrentSettledScope(t *testing.T) {
	a := newAdmittedWork()
	now := time.Now()
	for i := 0; i <= 128; i++ {
		admit(t, &a, fmt.Sprint(i), "T")
		workEvent(&a, "turn/started", "T", "", now)
		workEvent(&a, "turn/completed", "T", "interrupted", now)
		_ = a.collect()
	}
	if len(a.threads) != 0 || len(a.stopped) != 1 || len(a.stoppedOrder) != 1 {
		t.Fatal("settled scope order retained dead references")
	}
	workEvent(&a, "turn/completed", "T", "completed", now)
	if out := a.collect(); len(out) != 1 || out[0].Kind != "finished" {
		t.Fatal(out)
	}
}

func TestMaintenanceCannotBecomeSettledWorkThroughAnInterruptedACK(t *testing.T) {
	a := newAdmittedWork()
	a.manualStart("A", newObserver())
	if err := a.prepare("injected", scopedWork(), true); err == nil {
		t.Fatal("injection admitted during known maintenance")
	}
	if err := a.prepare("native", scopedWork(), false); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	workEvent(&a, "turn/started", "M", "", now)
	a.ack(meta{id: "native", turn: "M"})
	workEvent(&a, "turn/completed", "M", "interrupted", now)
	if len(a.collect()) != 0 || len(a.stopped) != 0 {
		t.Fatal("manual interrupt became settled work")
	}
	workEvent(&a, "turn/completed", "M", "completed", now)
	if len(a.collect()) != 0 {
		t.Fatal("manual completion leaked through settled cache")
	}
}
