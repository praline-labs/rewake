package mail

import (
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
)

func TestLaterDeliveryThreadsAreCompared(t *testing.T) {
	dir := stateDir(t)
	if err := mailtest.WriteDeliveryThread(dir, "api", "first", "current"); err != nil {
		t.Fatal(err)
	}
	if err := mailtest.WriteDeliveryThread(dir, "api", "second", "previous"); err != nil {
		t.Fatal(err)
	}
	if !ReportThreadChanged(dir, "api", []string{"first", "second"}, "current") {
		t.Fatal("later mismatched delivery omitted from shared report")
	}
}
