package workflow

import (
	"fmt"
	"strings"
	"time"
)

// The suite's binary runs a few of the product's waits shorter than a
// release, set at build through package buildtime: each is waited out in
// dozens of cases, and at the real length it adds minutes to a run while
// proving nothing the shorter one does not. The code is a release's; only
// these numbers differ, and docs/testing.md lists them.
//
// Each stays long enough for what the cases need of it under a loaded
// machine: the cases run side by side (pool_test.go), and a wait cut to the
// bone on an idle machine is a flaky case on a busy one.
const (
	// suiteQuiet and suiteCap are the coalescing window's (internal/host/delivery,
	// three and four seconds). The scenario that observes the window,
	// batch-arrival, spaces its heads-ups 600 ms apart and needs them inside
	// this quiet and cap. At 1.5 and 2 seconds, one crosswise run in four lost
	// its third heads-up to the cap: with six cases running, the sends that
	// write the heads-ups started over a second late.
	suiteQuiet = 2 * time.Second
	suiteCap   = 3 * time.Second
	// suitePickup is how long rewake compact and rewake interrupt wait for
	// the target to take a request (internal/cli, five seconds). The plugin
	// polls four times a second; the cases where
	// nobody takes it wait out all of it.
	suitePickup = 1500 * time.Millisecond
	// suiteLetterWait is how long a compaction letter waits for its other half
	// (internal/host, three seconds). Both halves come within moments of each
	// other; the cases where one never comes wait out all of it.
	suiteLetterWait = 1500 * time.Millisecond
	// suiteNoticeScan is how often main's wrapper looks for other sessions'
	// comings, goings and compactions (internal/host, one second). The cases
	// that prove no notice was sent wait out a scan and the coalescing cap.
	suiteNoticeScan = 250 * time.Millisecond
)

// suiteTags are the build tags of the binary under test:
// rewakefixture puts the fixture harness in the catalog, for its column.
const suiteTags = "rewakefixture"

// suiteFlags shorten the waits in the binary under test.
var suiteFlags = []string{"-ldflags", strings.Join([]string{
	buildValue("host/delivery", "builtQuiet", suiteQuiet),
	buildValue("host/delivery", "builtCap", suiteCap),
	buildValue("cli", "builtPickup", suitePickup),
	buildValue("host", "builtLetterWait", suiteLetterWait),
	buildValue("host", "builtNoticeScan", suiteNoticeScan),
}, " "), "-tags", suiteTags}

func buildValue(pkg, name string, value time.Duration) string {
	return fmt.Sprintf("-X github.com/praline-labs/rewake/internal/%s.%s=%s", pkg, name, value)
}
