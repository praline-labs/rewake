package mail

import (
	"testing"

	"github.com/praline-labs/rewake/internal/infra/boottime"
)

// Put stamps the boot clock on a letter that has no reading, and keeps one it
// was given.
func TestPutStampsTheBootClock(t *testing.T) {
	dir := stateDir(t)
	before := boottime.Now()
	fresh, given := message("fresh"), message("given")
	given.CreatedBoot = 42
	for _, letter := range []Message{fresh, given} {
		if err := Put(dir, letter); err != nil {
			t.Fatal(err)
		}
	}
	if stored, _, _ := readCopy(dir, "api", fresh.ID); stored.CreatedBoot < before {
		t.Errorf("the fresh letter carries %d, want a reading from %d on", stored.CreatedBoot, before)
	}
	if stored, _, _ := readCopy(dir, "api", given.ID); stored.CreatedBoot != 42 {
		t.Errorf("the given reading became %d", stored.CreatedBoot)
	}
}
