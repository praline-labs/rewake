package sessionstate

import (
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
)

// Freshness goes by the boot clock reading a snapshot carries: a step of the
// wall clock between the publisher and the reader makes a snapshot published
// now look three seconds old, or one second from the future, and it stays
// fresh; a reading two and a half seconds old is stale whatever the wall
// clock says.
func TestFreshnessGoesByTheBootClock(t *testing.T) {
	cases := []struct {
		name string
		wall time.Duration
		boot time.Duration
		want bool
	}{
		{"the wall clock stepped forward", -3 * time.Second, 0, true},
		{"the wall clock stepped back", 1500 * time.Millisecond, 0, true},
		{"an old reading, a current wall time", 0, -2500 * time.Millisecond, false},
	}
	for _, c := range cases {
		dir := privateDir(t)
		published := time.Now().Add(c.wall)
		snapshot := Snapshot{Fresh: true, ContextFresh: true, PublishedAt: &published, PublishedBoot: boottime.Now() + int64(c.boot)}
		if err := Save(dir, "worker", "1.1", snapshot); err != nil {
			t.Fatal(err)
		}
		if got := Load(dir, "worker", "1.1"); got.Fresh != c.want || got.ContextFresh != c.want {
			t.Errorf("%s: fresh %v, context fresh %v, want %v", c.name, got.Fresh, got.ContextFresh, c.want)
		}
	}
}

// A snapshot from a build that wrote no reading is judged by its wall time.
func TestASnapshotWithoutABootReadingGoesByTheWallClock(t *testing.T) {
	now := time.Now()
	for _, c := range []struct {
		at   time.Time
		want bool
	}{{now, true}, {now.Add(-3 * time.Second), false}, {now.Add(2 * time.Second), false}} {
		if got := published(Snapshot{PublishedAt: &c.at}, now, boottime.Now()); got != c.want {
			t.Errorf("published at %s: %v, want %v", c.at.Sub(now), got, c.want)
		}
	}
}
