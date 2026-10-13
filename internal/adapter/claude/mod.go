package claude

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
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
//go:embed mod/.claude-plugin/plugin.json mod/hooks/hooks.json mod/hooks/rewake.ts mod/protocol.gen.ts
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
// beside the inbox lane, the mod written before either starts.
type modRun struct {
	*lane
	dir    string
	socket string
	server *modhost.Server
}

func (r *modRun) Start(ctx context.Context, note func(string)) error {
	if err := writeMod(r.dir); err != nil {
		return fmt.Errorf("could not write the rewake mod for this run: %w", err)
	}
	sweepReplies(filepath.Dir(r.socket))
	if err := r.server.Start(); err != nil && note != nil {
		// The mod loads and finds nobody: it reports nothing, and the run
		// goes on as one whose mod never reported in.
		note("the rewake mod will not be heard: " + err.Error())
	}
	return r.lane.Start(ctx, note)
}

func (r *modRun) Close() {
	r.lane.Close()
	r.server.Close()
}

func (r *modRun) HarnessStarted(harness adapter.RunProcess) { r.server.HarnessStarted(harness) }

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
