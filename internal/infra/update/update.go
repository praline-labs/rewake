// Package update is updating rewake from inside it
// (docs/install.md#updating-from-inside-rewake): whether this build came
// from npm, the versions of the releases, the changelog shipped in each, the
// record of the last check for a newer one under the state root, and the
// calls of npm that check, fetch and install. What a release restarts is
// never read from its changelog: it is parts.Plan against the release's own
// manifest.
package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Channel is where this build was installed from, which the tool that
// builds it passes in: scripts/pack.sh sets
// -X github.com/praline-labs/rewake/internal/infra/update.Channel=npm. Only
// such a build checks the registry, shows a newer release or installs one;
// a build by tools/land, by hand or with go run leaves it empty and updates
// by being built again.
var Channel string

// FromNPM says this build came from npm.
func FromNPM() bool { return Channel == "npm" }

// Package is the one name every build of a release is published under.
const Package = "@praline-labs/rewake"

// Version is a release's version, major.minor.patch: the entry of a release
// is never a prerelease here, and a platform build's version, such as
// 2.12.0-linux-x64, is not a release.
type Version struct{ Major, Minor, Patch int }

// ParseVersion reads a release's version.
func ParseVersion(s string) (Version, error) {
	fields := strings.Split(s, ".")
	if len(fields) != 3 {
		return Version{}, fmt.Errorf("%q is not a release's version, major.minor.patch", s)
	}
	var n [3]int
	for i, f := range fields {
		v, err := strconv.Atoi(f)
		if err != nil || v < 0 || f != strconv.Itoa(v) {
			return Version{}, fmt.Errorf("%q is not a release's version, major.minor.patch", s)
		}
		n[i] = v
	}
	return Version{n[0], n[1], n[2]}, nil
}

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

// Compare is -1, 0 or 1 as v is older than, the same as or newer than w.
func (v Version) Compare(w Version) int {
	for _, d := range [][2]int{{v.Major, w.Major}, {v.Minor, w.Minor}, {v.Patch, w.Patch}} {
		switch {
		case d[0] < d[1]:
			return -1
		case d[0] > d[1]:
			return 1
		}
	}
	return 0
}

// Newer says a is a release newer than b; a version that does not read as
// a release is never newer, and every release is newer than one that does
// not read.
func Newer(a, b string) bool {
	va, err := ParseVersion(a)
	if err != nil {
		return false
	}
	vb, err := ParseVersion(b)
	return err != nil || va.Compare(vb) > 0
}

// Platform is this machine's npm platform, the suffix of the build npm
// installs here: linux-x64 or linux-arm64.
func Platform() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	return runtime.GOOS + "-" + arch
}

// PlatformBuild says version names a platform build of a release, such as
// 2.12.0-linux-x64, and which release.
func PlatformBuild(version string) (string, bool) {
	release, suffix, found := strings.Cut(version, "-")
	if !found {
		return "", false
	}
	if _, err := ParseVersion(release); err != nil {
		return "", false
	}
	return release, strings.HasPrefix(suffix, "linux-") || strings.HasPrefix(suffix, "darwin-")
}

// Prefix is the npm prefix an install lies under: the part of its origin,
// the directory the installed rewake is in, above lib/node_modules, where
// npm install -g --prefix puts a package.
func Prefix(origin string) (string, error) {
	const mark = "/lib/node_modules/"
	at := strings.Index(origin+"/", mark)
	if at <= 0 {
		return "", fmt.Errorf("%s is not inside an npm prefix's lib/node_modules", origin)
	}
	return origin[:at], nil
}

// ErrNotInstalled is an origin no npm package of rewake holds.
var ErrNotInstalled = errors.New("not an npm install of rewake")

// Installed is the release installed at origin: the version of the platform
// build whose bin it is, read from its package.json, without its platform.
func Installed(origin string) (string, error) {
	data, err := os.ReadFile(filepath.Join(origin, "..", "package.json"))
	if err != nil {
		return "", fmt.Errorf("%w at %s: %v", ErrNotInstalled, origin, err)
	}
	var pkg struct{ Name, Version string }
	if err := json.Unmarshal(data, &pkg); err != nil || pkg.Name != Package {
		return "", fmt.Errorf("%w at %s", ErrNotInstalled, origin)
	}
	release, ok := PlatformBuild(pkg.Version)
	if !ok {
		return "", fmt.Errorf("%w at %s: its version %s names no platform build", ErrNotInstalled, origin, pkg.Version)
	}
	return release, nil
}
