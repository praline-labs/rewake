package cli

import (
	"sort"
	"strconv"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mirror"
	"github.com/praline-labs/rewake/internal/host"
	"github.com/praline-labs/rewake/internal/host/mainhost"
	"github.com/praline-labs/rewake/internal/host/spacehost"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/parts"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Contracts are what this build speaks to the parts beside it and requires
// of them (docs/install.md#the-parts-and-what-an-update-restarts); --version
// --json carries them, so the tools that write a manifest ask the built
// binary rather than read its sources.
type Contracts struct {
	Speaks   map[string]string `json:"speaks"`
	Requires map[string]string `json:"requires"`
}

// The contracts a running room holds across a switch of its build
// (docs/rules/state.md#switching-a-running-room): the wrapper's handover and
// the face's model it carries, the space host's, main's host's, and the
// switch's own mark, messages and parked image.
const (
	handoverContract         = "handover"
	mirrorHandoverContract   = "handover.mirror"
	spaceHandoverContract    = "handover.space"
	mainHostHandoverContract = "handover.mainhost"
	switchContract           = "switch"
)

// contractsOf is this build's: the host's channel and the mirror's lines,
// what a switch of the room carries over — the handover, the switch, and
// each harness's own (adapter.Contracts) — and the version of each harness
// rewake ships beside itself.
func contractsOf(catalog adapter.Catalog) Contracts {
	c := Contracts{
		Speaks: map[string]string{
			parts.HostContract:       strconv.Itoa(mainhost.Version),
			parts.MirrorContract:     strconv.Itoa(mirror.Version),
			handoverContract:         strconv.Itoa(host.HandoverVersion),
			mirrorHandoverContract:   strconv.Itoa(mirror.HandedVersion),
			spaceHandoverContract:    strconv.Itoa(spacehost.HandoverVersion),
			mainHostHandoverContract: strconv.Itoa(mainhost.HandoverVersion),
			switchContract:           strconv.Itoa(state.SwitchVersion),
		},
		Requires: map[string]string{},
	}
	for _, harness := range catalog.All() {
		if _, ok := harness.(adapter.Shipped); ok {
			c.Requires[harness.ID()] = harness.Version().Pinned
		}
		if held, ok := harness.(adapter.Contracts); ok {
			for name, version := range held.Contracts() {
				c.Speaks[name] = version
			}
		}
	}
	return c
}

// digest is the digest of the sources this build was made from
// (parts.GoDigest), which the tool that builds it passes in:
// -ldflags "-X github.com/praline-labs/rewake/internal/cli.digest=<digest>",
// as tools/release through scripts/pack.sh and tools/land do. Empty in a
// build by hand, whose rewake the plan then judges by its file.
var digest string

// ownPart is this build as a part. A command of a room runs as the room's
// own copy, handed off to it when another build was called, so this is the
// rewake the room runs.
func ownPart(catalog adapter.Catalog) parts.Part {
	id, _ := build.ID()
	c := contractsOf(catalog)
	return parts.Part{Name: parts.Rewake, Class: parts.Switch, Digest: digest, File: id, Speaks: c.Speaks, Requires: c.Requires}
}

// partNames are the parts rewake restart knows: every build's, each harness
// it ships, and whatever the installed manifest lists.
func partNames(catalog adapter.Catalog, installed parts.Manifest) []string {
	seen := map[string]bool{parts.Rewake: true, parts.ShellPart: true, parts.Face: true}
	for name := range contractsOf(catalog).Requires {
		seen[name] = true
	}
	for _, p := range installed.Parts {
		seen[p.Name] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// updateOf is the room's plan: the parts running — this build, and the shell
// main's host draws with, if one answers — against the manifest of the
// install in dir.
// problem says why there is no plan; running is what it compared.
type updateOf struct {
	installed parts.Manifest
	running   []parts.Part
	steps     []parts.Step
	problem   string
}

func roomUpdate(catalog adapter.Catalog, dir string) updateOf {
	u := updateOf{running: []parts.Part{ownPart(catalog)}}
	if dir, err := state.Dir(); err == nil {
		if shells, err := mainhost.Running(mainhost.HostSocket(dir)); err == nil {
			u.running = append(u.running, shells...)
		}
	}
	installed, err := parts.Read(dir)
	if err != nil {
		u.problem = err.Error()
		return u
	}
	u.installed = installed
	u.steps = parts.Plan(u.running, installed)
	return u
}
