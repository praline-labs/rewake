package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/host/switching"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// rewake restart rewake leads the switch of the running room to the installed
// build (docs/rules/state.md#switching-a-running-room): it claims the
// attempt, stages the installed copy, admits it by its contracts, and leads
// the room's long-lived processes through it (host/switching). It runs as the
// room's build: a change form is handed to the room's copy.

// switchBounds are the leader's waits.
var switchBounds = switching.Default

// versionWait bounds the staged copy's answer to --version --json.
const versionWait = 10 * time.Second

// switchView is the answer: the builds, and who was adopted or lost.
type switchView struct {
	Switched bool     `json:"switched"`
	From     string   `json:"from"`
	To       string   `json:"to"`
	Adopted  []string `json:"adopted"`
	Lost     []string `json:"lost"`
}

func restartRewake(ctx *Context) error {
	if ctx.lease == nil {
		return failf("rewake was not switched: the command holds no lease of the room")
	}
	dir, err := state.Dir()
	if err != nil {
		return failf("rewake was not switched: %v", err)
	}
	installed := filepath.Join(installedDir(), "rewake")
	target, err := build.FileID(installed)
	if err != nil {
		return failf("rewake was not switched: the installed rewake cannot be read: %v", err)
	}
	own, err := build.ID()
	if err != nil {
		return failf("rewake was not switched: %v", err)
	}
	view := switchView{From: own, To: target, Adopted: []string{}, Lost: []string{}}
	if target == own {
		return printValue(ctx, view, func() []string {
			return []string{"Rewake: nothing to switch; the room runs the installed rewake, build " + build.Short(own) + "."}
		})
	}
	b := switchBounds
	// One name for every attempt: one is claimed at a time, and the socket a
	// committed attempt could not remove, its lease revoked, is the next's to
	// clear.
	socket := state.SocketPath(dir, "switch", "")
	sw, err := build.Claim(ctx.lease, target, socket, time.Now().Add(b.Settle+b.Ask+b.Drop))
	if err != nil {
		return failf("rewake was not switched: %v", err)
	}
	if err := sw.Stage(installed); err != nil {
		_ = sw.End()
		return failf("rewake was not switched: the installed rewake could not be staged: %v", err)
	}
	if why := admits(ctx.table.catalog, sw.Mark.Staged); why != "" {
		_ = sw.End()
		return failf("rewake was not switched: %s; restart the room — end main and start it again — to load it", why)
	}
	out := switching.Lead(sw, b)
	if !out.Switched {
		return failf("rewake was not switched; the room stays on build %s: %s", build.Short(own), out.Why)
	}
	view.Switched, view.Adopted, view.Lost = true, nonNil(out.Adopted), nonNil(out.Lost)
	if len(out.Lost) > 0 {
		return failf("the room switched to build %s, and %s", build.Short(target), out.Why)
	}
	return printValue(ctx, view, func() []string {
		return []string{"Rewake: the room switched from build " + build.Short(own) + " to " + build.Short(target) + "; adopted: " + listOrNone(out.Adopted) + "."}
	})
}

// admits says why the staged copy may not take the running room over, or "":
// every contract it speaks and requires must be this build's, since what
// runs in the room — a loaded mod, an extension, a handed-over run — keeps
// the one it started with.
func admits(catalog adapter.Catalog, staged string) string {
	theirs, err := contractsOfFile(staged)
	if err != nil {
		return err.Error()
	}
	own := contractsOf(catalog)
	if why := differ("speaks", own.Speaks, theirs.Speaks); why != "" {
		return why
	}
	return differ("requires", own.Requires, theirs.Requires)
}

// contractsOfFile asks a build for its contracts, as the tools that write a
// manifest do.
func contractsOfFile(path string) (Contracts, error) {
	ctx, cancel := context.WithTimeout(context.Background(), versionWait)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version", "--json").Output()
	if err != nil {
		return Contracts{}, errors.New("the installed rewake did not say its contracts: " + err.Error())
	}
	var version struct{ Contracts *Contracts }
	if err := json.Unmarshal(out, &version); err != nil || version.Contracts == nil {
		return Contracts{}, errors.New("the installed rewake names no contracts")
	}
	return *version.Contracts, nil
}

// differ names the first contract, in order, whose versions are not equal.
func differ(verb string, own, theirs map[string]string) string {
	names := make([]string, 0, len(own)+len(theirs))
	for name := range own {
		names = append(names, name)
	}
	for name := range theirs {
		if _, ok := own[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if own[name] != theirs[name] {
			return "the installed rewake " + verb + " " + name + " " + orNone(theirs[name]) + ", and the room's " + verb + " " + orNone(own[name])
		}
	}
	return ""
}

func listOrNone(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

func nonNil(names []string) []string {
	if names == nil {
		return []string{}
	}
	return names
}
