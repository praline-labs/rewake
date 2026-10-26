package mail

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
)

func TestAWaitingReportKeepsItsDeliveryThread(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	task := message("work")
	task.FromEpoch = "2.2"
	task.ToEpoch = run
	unread(t, dir, task)
	if err := mailtest.WriteDeliveryThread(dir, "api", task.ID, "old"); err != nil {
		t.Fatal(err)
	}
	if err := MarkRead(dir, "api", run, task, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ThreadPath(dir, "api"), task.ID)
	old := time.Now().Add(-2 * keepFinished)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	sweepAsLiveRun(t, dir, "api", run)
	if !ReportThreadChanged(dir, "api", []string{task.ID}, "new") {
		t.Fatal("sweep discarded an unsettled wait's thread")
	}
	for _, waiter := range Waiters(dir, "api", run) {
		if err := ClearAwaiting(dir, "api", run, waiter); err != nil {
			t.Fatal(err)
		}
	}
	sweepAsLiveRun(t, dir, "api", run)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("settled thread record was not swept: %v", err)
	}
}
