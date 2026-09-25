package cli

import (
	"runtime/debug"
	"sync"
	"time"
)

// Build says which build of rewake is running. The version alone is the same
// for every build between two releases, so it cannot tell one binary from
// another; the revision the binary was built from, and when it was built, can.
// Go records the revision when it builds inside a git checkout; a binary built
// from an archive, or run with go run, carries none, and the revision is then
// unknown rather than guessed.
type Build struct {
	Version string `json:"version"`
	// Known is false when the binary carries no revision; Revision and
	// Modified are empty then.
	Known    bool   `json:"known"`
	Revision string `json:"revision,omitempty"`
	// Built is when the binary was built, which only the build command can
	// say: it passes the time in, as scripts/pack.sh and the documented
	// local build do. Nothing in the binary records it otherwise.
	Built *time.Time `json:"built,omitempty"`
	// Committed is the revision's commit time, which Go records itself; the
	// line falls back to it for a build that passed no time in.
	Committed *time.Time `json:"committed,omitempty"`
	// Modified is set when the working tree differed from the revision, so
	// the binary is not that revision alone.
	Modified bool `json:"modified,omitempty"`
}

// built is the build time the build command passes in, in RFC 3339:
// -ldflags "-X github.com/iiiokojiadbi/rewake/internal/cli.built=$(date -u +%Y-%m-%dT%H:%M:%SZ)".
var built string

// thisBuild is the running binary's build, read once. A test replaces it, so
// nothing it prints depends on how the test binary was built.
var thisBuild = sync.OnceValue(func() Build {
	info, ok := debug.ReadBuildInfo()
	return buildFrom(info, ok, built)
})

// buildFrom reads the VCS stamp Go puts into a binary built in a checkout,
// and the build time passed in, if any. A time that does not parse is none.
func buildFrom(info *debug.BuildInfo, ok bool, builtAt string) Build {
	build := Build{Version: Version}
	if at, err := time.Parse(time.RFC3339, builtAt); err == nil {
		at = at.UTC()
		build.Built = &at
	}
	if !ok || info == nil {
		return build
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			build.Revision = setting.Value
		case "vcs.time":
			if at, err := time.Parse(time.RFC3339, setting.Value); err == nil {
				at = at.UTC()
				build.Committed = &at
			}
		case "vcs.modified":
			build.Modified = setting.Value == "true"
		}
	}
	if build.Revision == "" {
		// Settings without a revision describe no checkout: go run, or a
		// build from an archive.
		return Build{Version: Version, Built: build.Built}
	}
	build.Known = true
	return build
}

// Line is the build in one line, the first line of the guide and the whole
// of --version: "rewake 0.0.1 · build 36d6b90 · built 2026-09-25 11:58 UTC".
// The time is the build's when the build passed it in, else the revision's
// commit time, labeled as such.
func (b Build) Line() string {
	line := "rewake " + b.Version
	if b.Known {
		line += " · build " + shortRevision(b.Revision)
	} else {
		line += " · build unknown"
	}
	switch {
	case b.Built != nil:
		line += " · built " + b.Built.Format("2006-01-02 15:04") + " UTC"
	case b.Committed != nil:
		line += " · committed " + b.Committed.Format("2006-01-02 15:04") + " UTC"
	}
	if b.Modified {
		line += " · modified"
	}
	return line
}

// shortRevision is the revision as git abbreviates it by default.
func shortRevision(revision string) string {
	if len(revision) > 7 {
		return revision[:7]
	}
	return revision
}
