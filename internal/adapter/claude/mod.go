package claude

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The mod rides with the binary and is written into each run's directory, so
// a run loads the mod of the build that started it and nothing in the
// person's settings changes (docs/v2/design-claude.md#loading-the-mod-for-one-run).
// The files are named one by one: a pattern skips names that begin with a
// dot, and the manifest's directory is one, while the tests and the schema
// are no part of what a run loads.
//
//go:embed mod/.claude-plugin/plugin.json mod/hooks/hooks.json mod/hooks/rewake.ts mod/hooks/outbox.ts mod/protocol.gen.ts
var modFiles embed.FS

// pluginDirFlag loads a plugin from a directory for one launch.
const pluginDirFlag = "--plugin-dir"

// modSocketEnv names the run's mod socket in the harness's environment, where
// the mod reads it.
const modSocketEnv = "REWAKE_MOD_SOCKET"

// modSuffix ends every mod socket, the hashed form's included.
const modSuffix = ".mod.sock"

// harnessWait bounds how long a request of the mod waits for the host to name
// the harness's process. The mod's first request comes at session.start, well
// after the process is running; the wait only covers the host's own step.
const harnessWait = 5 * time.Second

// modDir is where a run's mod is written.
func modDir(runDir string) string { return filepath.Join(runDir, "mod") }

// modSocketPath is the run's mod socket: beside the run's inbox socket, in the
// room's directory of sockets, kept within the limit a unix socket's path
// allows, as the inbox socket is.
func modSocketPath(request adapter.LaunchRequest) string {
	if own := strings.TrimSuffix(request.Socket, ".sock") + modSuffix; len(own) <= maxSocketPath {
		return own
	}
	sum := sha256.Sum256([]byte(request.Name + "\x00" + request.Epoch))
	return filepath.Join(filepath.Dir(request.Socket), fmt.Sprintf("rewake-%x%s", sum[:8], modSuffix))
}

// writeMod writes the embedded mod into dir, private to this user.
func writeMod(dir string) error {
	return fs.WalkDir(modFiles, "mod", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := modFiles.ReadFile(name)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(name, "mod/")))
		if err := state.EnsureSubdir(filepath.Dir(target)); err != nil {
			return err
		}
		return state.WriteFile(target, data, 0o600)
	})
}

// modRun is the service of a run that loads the mod: the run's mod endpoint
// beside the inbox lane, the mod written before either starts. Its turns
// reach the core through the endpoint (docs/v2/stage4-turns.md).
type modRun struct {
	*lane
	dir    string
	socket string
	server *modhost.Server
	// hostsSpaces is the run's role opening spaces: an admitted hello then
	// proves SpacesCapability.
	hostsSpaces bool

	// reporting orders the live set's reports, the start's and each
	// hello's, so the set reported last is the run's.
	reporting sync.Mutex
	live      func([]adapter.Capability)
	proven    bool
	// withheld is why the mod will not be heard, once Start found it.
	withheld []adapter.Withheld
}

// ReportLive: the inbox lane is live once it starts, as a run that reports
// nothing would have it; the turn boundary joins it with the first admitted
// hello, and spaces with it on a run that hosts them
// (docs/v2/design-api.md#spaces). Withdrawing them when the mod is lost is
// the heartbeat's (docs/v2/stage4-turns.md#the-live-set), not built yet: an
// admitted hello keeps them for the run's life.
func (r *modRun) ReportLive(live func([]adapter.Capability)) { r.live = live }

func (r *modRun) report(proven bool) {
	r.reporting.Lock()
	defer r.reporting.Unlock()
	if r.live == nil {
		return
	}
	r.proven = r.proven || proven
	set := []adapter.Capability{adapter.WakeCapability}
	if r.proven {
		set = append(set, adapter.TurnBoundaryCapability)
		if r.hostsSpaces {
			set = append(set, adapter.SpacesCapability)
		}
	}
	r.live(set)
}

// admitted hears a hello the endpoint bound: from then the mod is loaded, the
// run's turns reach the core, and a run that hosts spaces has them.
func (r *modRun) admitted(modhost.HelloRequest) { r.report(true) }

// ReportTurns hands the endpoint where the run's turns go.
func (r *modRun) ReportTurns(handler adapter.TurnHandler) { r.server.ReportTurns(handler) }

// Withheld is why the mod of this run will not be heard, if Start found a
// reason: a role that owes reports is then refused before the harness starts.
func (r *modRun) Withheld() []adapter.Withheld { return r.withheld }

func (r *modRun) Start(ctx context.Context, note func(string)) error {
	if err := writeMod(r.dir); err != nil {
		r.withhold("its files could not be written into the run's directory: " + err.Error())
	} else if err := r.server.Start(); err != nil {
		// The mod would load and find nobody: it reports nothing.
		r.withhold("its endpoint is not listening: " + err.Error())
	}
	sweepReplies(filepath.Dir(r.socket))
	if err := r.lane.Start(ctx, note); err != nil {
		return err
	}
	r.report(false)
	return nil
}

func (r *modRun) withhold(cause string) {
	r.withheld = append(r.withheld, withheldMod(cause))
}

// withheldMod is the turn boundary withheld because the mod will not load or
// be heard: everything the mod proves goes with it.
func withheldMod(cause string) adapter.Withheld {
	return adapter.Withheld{Capability: adapter.TurnBoundaryCapability, Cause: "the rewake mod will not be heard in this run: " + cause}
}

// modless is the service of a run that loads no mod: its inbox lane, and why
// there is no mod.
type modless struct {
	*lane
	cause string
}

func (m modless) Withheld() []adapter.Withheld { return []adapter.Withheld{withheldMod(m.cause)} }

func (r *modRun) Close() {
	r.lane.Close()
	r.server.Close()
}

func (r *modRun) HarnessStarted(harness adapter.RunProcess) { r.server.HarnessStarted(harness) }

func (r *modRun) CountWorkers(workers adapter.Workers) { r.server.CountWorkers(workers) }

// withEnv is env with name set to value, whatever it held before.
func withEnv(env []string, name, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, name+"=") {
			out = append(out, kv)
		}
	}
	return append(out, name+"="+value)
}
