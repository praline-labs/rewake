package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter/pi"
	"github.com/praline-labs/rewake/internal/infra/parts"
)

// manifestFile is where a platform package carries the manifest of its
// parts, beside the rewake that reads it
// (docs/install.md#the-parts-and-what-an-update-restarts).
const manifestFile = "bin/" + parts.FileName

// goArch is Go's name of an npm cpu.
var goArch = map[string]string{"x64": "amd64", "arm64": "arm64"}

// writeManifests writes each platform package's manifest after pack.sh: the
// digests of what it built, and the contracts this machine's build of
// rewake and of the shell say, which every platform's build shares.
func (g *gate) writeManifests(ctx context.Context) {
	here, ok := platformFor(g.arch)
	if !ok {
		g.fail("no build serves this machine, so no build can say its contracts for the manifests")
		return
	}
	bin := filepath.Join(g.dist(here), "bin")
	version := g.run(ctx, runLimit, g.root, filepath.Join(bin, "rewake"), "--version", "--json")
	if !version.ok() {
		g.fail("the manifests: rewake --version --json failed: %s", version.why())
		return
	}
	contracts := g.run(ctx, runLimit, g.root, filepath.Join(bin, "rewake-shell"), "--contracts")
	if !contracts.ok() {
		g.fail("the manifests: rewake-shell --contracts failed: %s", contracts.why())
		return
	}
	shell, err := parts.SourcesDigest(filepath.Join(g.root, "shell"))
	if err != nil {
		g.fail("the manifests: the digest of the shell: %v", err)
		return
	}
	face, err := parts.SourcesDigest(filepath.Join(g.root, "face"))
	if err != nil {
		g.fail("the manifests: the digest of the face: %v", err)
		return
	}
	for _, p := range packages {
		if p.platform == "" {
			continue
		}
		made := parts.Made{
			Version:   []byte(version.stdout),
			Shell:     parts.Part{Digest: shell},
			Contracts: []byte(contracts.stdout),
			Face:      parts.Part{Digest: face, Within: parts.ShellPart},
			Harnesses: []parts.Part{parts.HarnessPart("pi", pi.Package, pi.PinnedVersion)},
		}
		if err := g.writeManifest(ctx, p, made); err != nil {
			g.fail("%s: the manifest: %v", p.ref(g.version), err)
		} else {
			g.pass("%s: %s lists its parts", p.ref(g.version), manifestFile)
		}
	}
}

func (g *gate) writeManifest(ctx context.Context, p pkg, made parts.Made) error {
	digest := g.goDigest
	if digest == nil {
		digest = parts.GoDigest
	}
	var err error
	if made.Rewake.Digest, err = digest(ctx, g.root, "linux", goArch[p.cpu]); err != nil {
		return err
	}
	dir := g.dist(p)
	if made.Rewake.File, err = parts.FileDigest(filepath.Join(dir, "bin", "rewake")); err != nil {
		return err
	}
	if made.Shell.File, err = parts.FileDigest(filepath.Join(dir, "bin", "rewake-shell")); err != nil {
		return err
	}
	m, err := parts.Make(made)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, manifestFile), parts.Encode(m), 0o644); err != nil {
		return fmt.Errorf("cannot write it: %w", err)
	}
	return nil
}
