package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter/pi"
	"github.com/praline-labs/rewake/internal/infra/parts"
)

// contractLimit bounds a part's answer of its own contracts.
const contractLimit = buildLimit

// put puts the landed tree in place of the room's and writes its
// manifest (docs/install.md#the-parts-and-what-an-update-restarts): rewake is
// built only when its digest changed, since every build's bytes differ by
// its time and would ask for a room restart; the shell and the face run from
// the checkout and need no install. It answers the line of what this
// landing restarts, or why the install failed.
func (l *lander) put(ctx context.Context) (string, string) {
	old, oldErr := parts.Read(l.install)
	// A manifest left from before would describe the old build once the new
	// one is in place.
	if err := os.Remove(filepath.Join(l.install, parts.FileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err.Error()
	}
	digest, err := l.goDigest(ctx, l.root)
	if err != nil {
		l.say("FAIL  digest of rewake: %v; built anyway", err)
	}
	was, _ := old.Find(parts.Rewake)
	binary := filepath.Join(l.install, "rewake")
	if err == nil && oldErr == nil && was.Digest == digest {
		l.say("skip  build    rewake's sources are the installed build's")
	} else if why := l.installBuild(ctx); why != "" {
		return "", why
	}
	if err != nil {
		return "restart: room (no manifest: the digest of rewake failed)", ""
	}
	next, err := l.made(ctx, digest, binary)
	if err != nil {
		l.say("FAIL  manifest: %v", err)
		return "restart: room (no manifest: it could not be made)", ""
	}
	if err := writeManifest(l.install, next); err != nil {
		l.say("FAIL  manifest: %v", err)
		return "restart: room (no manifest: it could not be written)", ""
	}
	if oldErr != nil {
		return "restart: room (no manifest before this landing)", ""
	}
	return planLine(old, next), ""
}

// made is the manifest of what this landing installed.
func (l *lander) made(ctx context.Context, digest, binary string) (parts.Manifest, error) {
	m := parts.Made{Rewake: parts.Part{Digest: digest}}
	var err error
	if m.Rewake.File, err = parts.FileDigest(binary); err != nil {
		return parts.Manifest{}, err
	}
	r := l.run(ctx, command{dir: l.root, name: binary, args: []string{"--version", "--json"}, limit: contractLimit}, l.log)
	if !r.ok() {
		return parts.Manifest{}, fmt.Errorf("%s --version --json: %s", binary, r.why())
	}
	m.Version = []byte(r.stdout)
	shell := filepath.Join(l.root, "shell")
	if m.Shell.Digest, err = parts.SourcesDigest(shell); err != nil {
		return parts.Manifest{}, err
	}
	r = l.run(ctx, command{dir: shell, name: "bun", args: []string{"src/main.tsx", "--contracts"}, limit: contractLimit}, l.log)
	if !r.ok() {
		return parts.Manifest{}, fmt.Errorf("bun src/main.tsx --contracts: %s", r.why())
	}
	m.Contracts = []byte(r.stdout)
	if m.Face.Digest, err = parts.SourcesDigest(filepath.Join(l.root, "face")); err != nil {
		return parts.Manifest{}, err
	}
	if version := installedPi(l.install); version != "" {
		m.Harnesses = append(m.Harnesses, parts.HarnessPart("pi", pi.Package, version))
	}
	return parts.Make(m)
}

// installedPi is the version of the pi npm installed beside the build, or ""
// when there is none.
func installedPi(install string) string {
	data, err := os.ReadFile(filepath.Join(install, "node_modules", pi.Package, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct{ Version string }
	if json.Unmarshal(data, &pkg) != nil {
		return ""
	}
	return pkg.Version
}

// writeManifest puts the manifest beside the build through a rename, so a
// reader never meets half of one.
func writeManifest(dir string, m parts.Manifest) error {
	next := filepath.Join(dir, parts.FileName+".next")
	if err := os.WriteFile(next, parts.Encode(m), 0o644); err != nil {
		return err
	}
	return os.Rename(next, filepath.Join(dir, parts.FileName))
}

// planLine is what a room that ran the old install needs to run the new
// one: the room's restart when any step asks for it, which loads every
// other part too; else the shell's; else nothing.
func planLine(old, next parts.Manifest) string {
	steps := parts.Plan(old.Parts, next)
	for _, class := range []string{parts.Room, parts.Shell} {
		var names []string
		for _, s := range steps {
			switch {
			case s.Class != class:
			case s.Why == "changed":
				names = append(names, s.Part+" changed")
			default:
				names = append(names, s.Part+": "+s.Why)
			}
		}
		if len(names) == 0 {
			continue
		}
		if class == parts.Shell {
			return fmt.Sprintf("restart: shell (rewake restart shell; %s)", strings.Join(names, "; "))
		}
		return fmt.Sprintf("restart: room (%s)", strings.Join(names, "; "))
	}
	if slices.ContainsFunc(steps, func(s parts.Step) bool { return s.Class == parts.Live }) {
		return "restart: nothing (the face reloads from its files)"
	}
	return "restart: nothing"
}
