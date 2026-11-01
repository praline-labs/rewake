package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter/pi"
	"github.com/praline-labs/rewake/internal/infra/parts"
	"github.com/praline-labs/rewake/internal/infra/update"
)

// manifestFile is where a platform package carries the manifest of its
// parts, beside the rewake that reads it
// (docs/install.md#the-parts-and-what-an-update-restarts).
const manifestFile = "bin/" + parts.FileName

// entryManifest is where the entry package carries a platform build's
// manifest, so a rewake asking what a release restarts fetches the small
// entry rather than the build (docs/install.md#updating-from-inside-rewake).
func entryManifest(platform string) string {
	return path.Join(update.ManifestsDir, platform+".json")
}

// takeDigests takes rewake's digest for each platform before the build, and
// hands each to scripts/pack.sh in its environment, which builds it into the
// binary: a room then judges by the sources, not by bytes that every
// release's version changes, whether this release asks it to restart. The
// manifests take the same digests.
func (g *gate) takeDigests(ctx context.Context) {
	digest := g.goDigest
	if digest == nil {
		digest = parts.GoDigest
	}
	g.digests = map[string]string{}
	for _, p := range packages {
		if p.platform == "" {
			continue
		}
		d, err := digest(ctx, g.root, "linux", goArch[p.cpu])
		if err != nil {
			g.fail("%s: the digest of rewake: %v", p.ref(g.version), err)
			continue
		}
		g.digests[p.cpu] = d
		// The gate's runner passes its own environment on to pack.sh.
		_ = os.Setenv(digestEnv(p.platform), d)
	}
}

// digestEnv is the variable scripts/pack.sh reads a platform's digest from.
func digestEnv(platform string) string {
	return "REWAKE_DIGEST_" + strings.ToUpper(strings.ReplaceAll(platform, "-", "_"))
}

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
		if err := g.writeManifest(p, made); err != nil {
			g.fail("%s: the manifest: %v", p.ref(g.version), err)
		} else {
			g.pass("%s: %s lists its parts, and the entry's %s too", p.ref(g.version), manifestFile, entryManifest(p.platform))
		}
	}
}

func (g *gate) writeManifest(p pkg, made parts.Made) error {
	made.Rewake.Digest = g.digests[p.cpu]
	if made.Rewake.Digest == "" {
		return fmt.Errorf("rewake's digest was not taken")
	}
	var err error
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
	data := parts.Encode(m)
	if err := os.WriteFile(filepath.Join(dir, manifestFile), data, 0o644); err != nil {
		return fmt.Errorf("cannot write it: %w", err)
	}
	entry := filepath.Join(g.root, "dist", "npm", "rewake", filepath.FromSlash(entryManifest(p.platform)))
	if err := os.WriteFile(entry, data, 0o644); err != nil {
		return fmt.Errorf("cannot write it into the entry: %w", err)
	}
	return nil
}
