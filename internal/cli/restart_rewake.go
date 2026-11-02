package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
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

// rewake restart rewake switches the running room to the installed build
// (docs/rules/state.md#switching-a-running-room). It runs as the room's build
// — a change form is handed to the room's copy — and checks in the foreground
// what needs no claim: something to switch, and contracts the room admits.
// Then it hands the switch to a leader of its own process (switch_lead.go),
// bound to main's run, and either returns — asked from a session's turn,
// which keeps that session busy while it waits — or waits for the outcome on
// a pipe, asked by the person.

// switchBounds are each attempt's waits.
var switchBounds = switching.Default

// versionWait bounds the staged copy's answer to --version --json.
const versionWait = 10 * time.Second

// switchView is the answer: the builds, and who was adopted or lost.
// Deferred says the switch runs once every session is idle, its outcome a
// note to main.
type switchView struct {
	Switched bool     `json:"switched"`
	Deferred bool     `json:"deferred,omitempty"`
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
	base, err := state.Base()
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
		return printValue(ctx, view, func() []string { return []string{"Rewake: " + switchOutcome{View: view}.line()} })
	}
	// The leader admits its staged copy again: the install may change.
	if why := admits(ctx.table.catalog, installed); why != "" {
		return failf("rewake was not switched: %s; restart the room — end main and start it again — to load it", why)
	}
	spec := leadSpec{Base: base, Room: dir, From: own, Asked: time.Now().UnixMilli(), Installed: installedDir(), For: deferFor, Pause: deferPause, Bounds: switchBounds}
	spec.Main, spec.MainEpoch = roomMain(dir)
	spec.Asker = os.Getenv(state.SessionEnv)
	if spec.Asker != "" {
		spec.AskerEpoch = os.Getenv(state.EpochEnv)
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return failf("rewake was not switched: %v", err)
	}
	request, err := state.RequestSwitch(dir, raw)
	if err != nil {
		return failf("rewake was not switched: %v", err)
	}
	defer func() { _ = request.Close() }()
	var read, write *os.File
	if spec.Asker == "" {
		if read, write, err = os.Pipe(); err != nil {
			return failf("rewake was not switched: %v", err)
		}
		defer func() { _ = read.Close() }()
		spec.Pipe = true
	}
	err = startLeader(spec, request, write)
	if write != nil {
		_ = write.Close()
	}
	if err != nil {
		_ = state.EndSwitchRequest(dir, request)
		return failf("rewake was not switched: its leader could not start: %v", err)
	}
	if read == nil {
		view.Deferred = true
		return printValue(ctx, view, func() []string {
			return []string{"Rewake: the switch to build " + build.Short(target) + " runs once every session is idle, within " + deferFor.String() + "; its outcome comes to main as a note."}
		})
	}
	// The leader takes the room's lease exclusive at its commit: a command
	// waiting for it holds none, and writes nothing more.
	ctx.lease.Release()
	return waitOutcome(ctx, read)
}

// waitOutcome prints what the leader answers on the pipe.
func waitOutcome(ctx *Context, read *os.File) error {
	var out switchOutcome
	data, err := bufio.NewReader(read).ReadBytes('\n')
	if err != nil || json.Unmarshal(data, &out) != nil {
		return failf("the switch's leader ended without saying its outcome; rewake restart shows what the room runs")
	}
	if out.Fail != "" {
		return failf("%s", out.Fail)
	}
	return printValue(ctx, out.View, func() []string { return []string{"Rewake: " + out.line()} })
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
