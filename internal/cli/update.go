package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/parts"
	"github.com/praline-labs/rewake/internal/infra/state"
	"github.com/praline-labs/rewake/internal/infra/update"
)

// updateCommand shows a newer release of rewake on npm and installs one,
// restarting nothing (docs/install.md#updating-from-inside-rewake).
func updateCommand() *Command {
	return &Command{
		Name: "update", Args: "[<version>]", MaxPositionals: 1,
		Summary:  "With no version, ask npm for a newer release of rewake and show its changes by part and what installing it restarts. With a version, install exactly that release where this rewake was installed; nothing restarts.",
		Options:  []Option{jsonOption},
		Examples: []string{"rewake update", "rewake update 3.0.0", "rewake update --json"},
		Next:     []string{"rewake restart", "rewake restart shell"},
		Notes: []string{
			"Only a rewake installed with npm updates so; one built from source — tools/land, a build by hand — updates by being built again, and is refused. Main or the person may run it, in the room's directory; a worker is refused.",
			"With no version it asks the registry afresh, the same npm configuration the install reads: every npm call takes --global and the install's own --prefix, so a project's .npmrc is never read. It prints the running, installed and latest releases, the changelog's entries between the running release and the latest, and the plan from what the room runs to the latest release's manifest, one line per part as rewake restart prints it. Main's host asks too, every six hours, and the shell shows a newer release in its hints row.",
			"With a version, the exact release: no range, no latest, not a platform build's version; a prerelease, such as 3.1.0-rc.1, only so, since with no version only a release is offered. It must be on the registry. npm installs it with --global under the install's prefix, its output on stderr; then the plan of the newly installed parts is printed, as rewake restart prints it. The room keeps running its own copy of its build until its restart, so installing restarts nothing; an older release is allowed and is a rollback.",
			"Exit 0 shown, or installed; 1 refused or failed (not installed from npm, a worker, the registry not reached, no such release, npm failed); 2 a wrong call — a version that is not a release's.",
		},
		Access: changeForm, Handler: handleUpdate,
	}
}

// updateView is the answer: the releases, the changes between what runs and
// the latest, and the plan — to the latest with no version, of what was just
// installed with one.
type updateView struct {
	Running   string           `json:"running"`
	Installed string           `json:"installed,omitempty"`
	Latest    string           `json:"latest,omitempty"`
	Changes   []update.Release `json:"changes"`
	Steps     []parts.Step     `json:"steps"`
	// Problem is why there is no plan.
	Problem string `json:"problem,omitempty"`
}

// The seams of a test: npm's runner and the directory the running build was
// installed in.
var (
	npmRunner    update.Runner
	installedDir = parts.InstalledDir
)

func handleUpdate(ctx *Context, call Call) error {
	version := ""
	if len(call.Positionals) == 1 {
		version = call.Positionals[0]
		if release, ok := update.PlatformBuild(version); ok {
			return &UsageError{Command: call.Command, Message: fmt.Sprintf("%s is a platform build of the release %s; name the release: rewake update %s.", version, release, release)}
		}
		if _, err := update.ParseVersion(version); err != nil {
			return &UsageError{Command: call.Command, Message: err.Error() + ": name the exact release, as rewake update shows it."}
		}
	}
	if err := updateCaller(ctx); err != nil {
		return err
	}
	if !update.FromNPM() {
		return failf("this rewake was not installed from npm, so it updates as it was built: by building it again (docs/install.md#building-from-source); tools/land does it for a checkout's install")
	}
	origin := installedDir()
	prefix, err := update.Prefix(origin)
	if err != nil {
		return failf("this rewake does not lie in an npm prefix, so npm cannot update it: %v", err)
	}
	npm := update.NPM{Prefix: prefix, Run: npmRunner}
	if version != "" {
		return installRelease(ctx, npm, origin, version)
	}
	return showUpdate(ctx, npm, origin)
}

// updateCaller lets main and the person through: a run of the room whose
// role is main, or a process that is no session's.
func updateCaller(ctx *Context) error {
	if os.Getenv(state.SessionEnv) == "" {
		return nil
	}
	dir, err := state.Dir()
	if err != nil {
		return failf("only main or the person updates rewake, and this session's room is not known: %v", err)
	}
	self, _, err := ownRunIn(ctx.access, dir)
	if err != nil {
		return failf("only main or the person updates rewake, and this shell is not main's: %v", err)
	}
	if self.Role != role.Main.ID {
		return failf("only main or the person updates rewake; %s is a %s session. Ask main to do it.", self.Name, self.Role)
	}
	return nil
}

func showUpdate(ctx *Context, npm update.NPM, origin string) error {
	root, err := state.Root()
	if err != nil {
		return &UsageError{Message: err.Error()}
	}
	record, err := update.Check(context.Background(), root, npm, time.Now())
	if err != nil {
		return failf("the registry could not be asked for rewake's latest release: %v", err)
	}
	installed, _ := update.Installed(origin)
	view := updateView{Running: Version, Installed: installed, Latest: record.Latest, Changes: []update.Release{}, Steps: []parts.Step{}}
	room := roomUpdate(ctx.table.catalog, origin)
	newer := update.Offered(record.Latest, installed)
	switch {
	case newer && record.Manifest != nil:
		view.Changes = update.Between(record.Changelog, Version, record.Latest)
		view.Steps = parts.Plan(room.running, *record.Manifest)
	case newer:
		view.Problem = "the latest release's manifest was not fetched"
	case room.problem != "":
		view.Problem = room.problem
	default:
		view.Steps = append(view.Steps, room.steps...)
	}
	return printValue(ctx, view, func() []string {
		head := fmt.Sprintf("Rewake: running %s · installed %s · latest %s", view.Running, orNone(installed), view.Latest)
		if !newer {
			lines := []string{head + ": up to date."}
			if len(view.Steps) > 0 {
				lines = append(lines, "The room runs older parts than installed:")
				for _, step := range view.Steps {
					lines = append(lines, step.Line())
				}
			}
			return lines
		}
		lines := []string{head + "."}
		for _, r := range view.Changes {
			lines = append(lines, r.Lines()...)
		}
		switch {
		case view.Problem != "":
			lines = append(lines, "What installing "+view.Latest+" restarts is not known: "+view.Problem+".")
		case len(view.Steps) == 0:
			lines = append(lines, "Installing "+view.Latest+" restarts nothing.")
		default:
			lines = append(lines, "Installing "+view.Latest+" restarts:")
			for _, step := range view.Steps {
				lines = append(lines, step.Line())
			}
		}
		return append(lines, "Install it, restarting nothing yet: rewake update "+view.Latest)
	})
}

func installRelease(ctx *Context, npm update.NPM, origin, version string) error {
	run := context.Background()
	if err := npm.Exists(run, version); err != nil {
		if errors.Is(err, update.ErrNoRelease) {
			return failf("%v: rewake update shows the latest", err)
		}
		return failf("the registry could not be asked for %s %s: %v", update.Package, version, err)
	}
	if err := npm.Install(run, version, ctx.Stderr); err != nil {
		return failf("npm did not install %s@%s: %v", update.Package, version, err)
	}
	installed, err := update.Installed(origin)
	if err != nil || installed != version {
		return failf("npm installed %s@%s, but the install this rewake came from, %s, holds %s: npm put it under another prefix", update.Package, version, origin, orNone(installed))
	}
	// The plan reads the install this rewake came from, taken before npm
	// ran: npm moved the running file aside to replace it, and the
	// executable's path now names that removed directory (seen live,
	// November 1, 2026).
	room := roomUpdate(ctx.table.catalog, origin)
	view := updateView{Running: Version, Installed: installed, Changes: []update.Release{}, Steps: []parts.Step{}, Problem: room.problem}
	view.Steps = append(view.Steps, room.steps...)
	return printValue(ctx, view, func() []string {
		lines := []string{"Rewake: installed " + version + "; nothing restarted, the room runs " + Version + "."}
		switch {
		case view.Problem != "":
			lines = append(lines, "no plan: "+view.Problem+".")
		case len(view.Steps) == 0:
			lines = append(lines, "nothing to restart; the room runs what is installed.")
		}
		for _, step := range view.Steps {
			lines = append(lines, step.Line())
		}
		return lines
	})
}

// availableOf is a newer release than the one installed and the one
// running, as the record of the last check has it, and the most installing
// it asks of the room; nil for a build not from npm or with nothing newer.
// It reads the record only: no view asks the registry.
func availableOf(running []parts.Part) *Available {
	if !update.FromNPM() {
		return nil
	}
	root, err := state.Root()
	if err != nil {
		return nil
	}
	record := update.ReadRecord(root)
	installed, err := update.Installed(installedDir())
	if err != nil || record.Manifest == nil || !update.Offered(record.Latest, installed) || !update.Offered(record.Latest, Version) {
		return nil
	}
	return &Available{Version: record.Latest, Restarts: restartsOf(parts.Plan(running, *record.Manifest))}
}

// restartsOf is the most a plan asks of the room: a room step loads every
// part, so it comes before the shell's.
func restartsOf(steps []parts.Step) Restarts {
	most := RestartsNothing
	for _, step := range steps {
		switch step.Class {
		case parts.Room:
			return RestartsRoom
		case parts.Shell:
			most = RestartsShell
		}
	}
	return most
}
