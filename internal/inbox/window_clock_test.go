package inbox

import (
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/boottime"
)

// The wall clock of a machine can be stepped by seconds while a window runs —
// a time sync does it, every half a minute on some virtual machines. A letter
// whose wall time says it was written 1.8 s before it was, as a step between
// the first letter and the second makes it, keeps the cap it has by the boot
// clock, and the three letters still go out together.
func TestAWallClockStepInsideTheWindowKeepsTheCap(t *testing.T) {
	f := serveWindow(t, Window{Quiet: 2 * time.Second, Cap: 3500 * time.Millisecond})
	for index, text := range []string{"first left", "second left", "first available"} {
		if index > 0 {
			time.Sleep(1100 * time.Millisecond)
		}
		letter := message(text)
		letter.Kind, letter.ToEpoch = Note, "5.5"
		if index == 0 {
			letter.CreatedAt = letter.CreatedAt.Add(-1800 * time.Millisecond)
		}
		if err := Put(f.dir, letter); err != nil {
			t.Fatal(err)
		}
	}
	f.seen(1, 5*time.Second)
	time.Sleep(600 * time.Millisecond)
	notices, _ := f.seen(1, 0)
	if len(notices) != 1 || len(members(notices[0])) != 3 {
		t.Fatalf("got %d notice(s), want one carrying three notes: %+v", len(notices), notices)
	}
}

// writtenAt goes by the boot clock when the letter carries a reading it can
// use, and by the wall clock otherwise.
func TestWrittenAtFallsBackToTheWallClock(t *testing.T) {
	now := time.Now()
	boot := boottime.Now()
	wall := now.Add(-time.Second)
	cases := []struct {
		name   string
		letter Message
		want   time.Time
	}{
		{"a boot reading", Message{CreatedAt: now.Add(-time.Hour), CreatedBoot: boot - int64(2*time.Second)}, now.Add(-2 * time.Second)},
		{"no boot reading", Message{CreatedAt: wall}, wall},
		{"a reading ahead of boot, from before a reboot", Message{CreatedAt: wall, CreatedBoot: boot + int64(time.Hour)}, wall},
		{"a wall time ahead of now", Message{CreatedAt: now.Add(time.Second)}, now},
		{"neither", Message{}, now},
	}
	for _, c := range cases {
		if got := writtenAt(c.letter, now, boot); !got.Equal(c.want) {
			t.Errorf("%s: written at %s, want %s", c.name, got, c.want)
		}
	}
}

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
	if stored, _ := readCopy(dir, "api", fresh.ID); stored.CreatedBoot < before {
		t.Errorf("the fresh letter carries %d, want a reading from %d on", stored.CreatedBoot, before)
	}
	if stored, _ := readCopy(dir, "api", given.ID); stored.CreatedBoot != 42 {
		t.Errorf("the given reading became %d", stored.CreatedBoot)
	}
}
