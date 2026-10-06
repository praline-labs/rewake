package inbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A completed journal is kept while its run may retry it, and one that names
// no run — no epoch, bytes that do not parse, a record closed to reading —
// is kept by every sweep however old it is: nothing tells whose retry it
// would answer. One that names an ended run goes, which makes the rest mean
// something.
func TestNothingThatNamesNoRunIsSweptByAge(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	journals := map[string]string{
		"no epoch":     `{"op":"end"}`,
		"unparseable":  "{",
		"unreadable":   `{"epoch":"ended-run","op":"end"}`,
		"an ended run": `{"epoch":"ended-run","op":"end"}`,
	}
	old := time.Now().Add(-2 * keepFinished)
	paths := map[string]string{}
	for what, content := range journals {
		path := filepath.Join(JournalPath(dir, "api"), NewID()+doneSuffix)
		paths[what] = path
		writeRaw(t, path, content)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	closeToReading(t, paths["unreadable"])
	(&Server{Dir: dir, Name: "api", Epoch: run}).sweepFinished()
	for what, path := range paths {
		_, err := os.Stat(path)
		if gone := os.IsNotExist(err); gone != (what == "an ended run") {
			t.Errorf("a completed journal with %s: gone %v", what, gone)
		}
	}
}
