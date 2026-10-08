package channel

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// elsewhere are the failure points another test holds, by its name: the
// keeper's writes, its held closes and its end, which the record alone
// does not show.
var elsewhere = map[string]string{
	"the inbox that would carry a notice cannot be written":                        "TestNoticesWaitForTheOneInFlight",
	"a shell-advising notice to the worker fixed, its write failed, then a denial": "TestAdviceFixedBeforeTheBlockIsDropped",
	"main restarts during a suppressed failure":                                    "TestANewMainGetsTheCurrentCategory",
	"the harness exits, or the wrapper accepts SIGTERM or SIGHUP":                  "TestNothingIsToldAfterTheHarnessExits",
	"a server closes, and the harness exits within a heartbeat":                    "TestACloseYoungerThanAHeartbeatWaits",
	"a close, then later failures, folded after them":                              "TestACloseFoldedAfterLaterFailuresLandsWhereItHappened",
}

// Every row of the failure table is held by a test: a scenario of the
// same name, or the test named for it.
func TestEveryFailurePointOfTheTableIsHeld(t *testing.T) {
	table, err := os.ReadFile(filepath.Join("..", "..", "docs", "mail-bridge-channel-failures.md"))
	if err != nil {
		t.Fatal(err)
	}
	named := map[string]bool{}
	for _, sc := range scenarios() {
		named[sc.name] = true
	}
	tests := testSources(t)
	rows := 0
	for _, line := range strings.Split(string(table), "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 3 || !strings.HasPrefix(line, "| ") || strings.TrimSpace(cells[1]) == "Point" {
			continue
		}
		point := strings.TrimSpace(cells[1])
		rows++
		switch test, ok := elsewhere[point]; {
		case named[point]:
		case ok && !regexp.MustCompile(`func `+test+`\(`).MatchString(tests):
			t.Errorf("%q names %s, which no test of the module defines", point, test)
		case !ok:
			t.Errorf("no scenario holds the failure point %q", point)
		}
	}
	if rows < 25 {
		t.Fatalf("read %d rows of the failure table", rows)
	}
}

// testSources are the test files of the packages the table's rows are
// held in.
func testSources(t *testing.T) string {
	t.Helper()
	var all strings.Builder
	for _, dir := range []string{".", filepath.Join("..", "wrap")} {
		files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			all.Write(raw)
		}
	}
	return all.String()
}
