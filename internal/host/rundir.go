package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The run directory (docs/rules/launch.md): what an
// adapter writes for one run goes with the run. The run removes its own on
// every path out; a run killed outright removes nothing, so a later launch in
// the room removes what it left once that run is proven dead.

// makeRunDir makes this run's directory, private. A launch that cannot make
// it does not start: an adapter told to keep its run's files there would
// otherwise keep them nowhere or somewhere that outlives the run.
func makeRunDir(dir, name, epoch string) (string, error) {
	path := state.RunPath(dir, name, epoch)
	if err := state.EnsureSubdir(path); err != nil {
		return "", fmt.Errorf("could not make the run's directory: %w", err)
	}
	return path, nil
}

// observeRun judges a run's wrapper. Replaceable in tests, since no process
// can be made to answer "cannot be told" on demand.
var observeRun = registry.ObserveRun

// reclaimRuns removes the directories that runs proven dead left: their run
// directories and their control directories, each named by the run. Only a
// wrapper proven ended counts — gone, another process at its pid, a zombie,
// another boot — by the judgment that prunes a dead record; a run that is
// running, or one that cannot be told, keeps what it has, and so does an
// entry that names no run. Each removal is that run's alone: another run of
// the same name has another epoch and so another directory.
func reclaimRuns(dir string) {
	for _, parent := range []string{state.RunsPath(dir), state.ControlsPath(dir)} {
		entries, err := os.ReadDir(parent)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			// A file or a symlink is no directory a run made.
			if !entry.IsDir() {
				continue
			}
			epoch, ok := runOf(entry.Name())
			if !ok || observeRun(epoch) != proc.IdentityEnded {
				continue
			}
			_ = state.RemoveAll(filepath.Join(parent, entry.Name()))
		}
	}
}

// runOf reads the run out of an entry named <name>.<pid>.<start>.<boot>. A
// name may hold dots, so the epoch is read from the end; an entry of any
// other shape, an epoch without its boot among them, names no run that can
// be judged.
func runOf(entry string) (string, bool) {
	parts := strings.Split(entry, ".")
	if len(parts) < 4 {
		return "", false
	}
	name := strings.Join(parts[:len(parts)-3], ".")
	epoch := strings.Join(parts[len(parts)-3:], ".")
	if !state.ValidName(name) {
		return "", false
	}
	if _, _, boot, ok := registry.ParseRun(epoch); !ok || boot == "" {
		return "", false
	}
	return epoch, true
}
