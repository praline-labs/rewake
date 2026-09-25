package buildtime

import (
	"testing"
	"time"
)

func TestABuildSetsADuration(t *testing.T) {
	if got := Duration("builtWait", "", 3*time.Second); got != 3*time.Second {
		t.Fatalf("a build that sets nothing serves %s, want the standard 3s", got)
	}
	if got := Duration("builtWait", "1500ms", 3*time.Second); got != 1500*time.Millisecond {
		t.Fatalf("a build's duration is %s, want 1.5s", got)
	}
	for _, bad := range []string{"soon", "0s", "-1s"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("a build with builtWait=%q served a duration", bad)
				}
			}()
			Duration("builtWait", bad, 3*time.Second)
		}()
	}
}
