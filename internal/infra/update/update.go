// Package update is updating rewake from inside it
// (docs/install.md#updating-from-inside-rewake): whether this build came
// from npm, the versions of the releases, the changelog shipped in each, the
// record of the last check for a newer one under the state root, and the
// calls of npm that check, fetch and install. What a release restarts is
// never read from its changelog: it is parts.Plan against the release's own
// manifest.
package update

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// Version is a release's version: semver's major.minor.patch, with a
// prerelease where it has one (3.1.0-rc.1) and no build metadata, which npm
// strips. A platform build's version, such as 3.0.0-linux-x64, reads as a
// prerelease too; PlatformBuild tells it apart.
type Version struct {
	Major, Minor, Patch int
	// Pre is the prerelease's identifiers, dot-joined; "" for a release.
	Pre string
}

// semver is semver.org 2.0.0's grammar without build metadata, as
// tools/release takes a version.
var semver = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?$`)

// ParseVersion reads a release's version.
func ParseVersion(s string) (Version, error) {
	m := semver.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("%q is not a release's version, such as 3.0.0 or 3.1.0-rc.1", s)
	}
	var n [3]int
	for i := range n {
		v, err := strconv.Atoi(m[i+1])
		if err != nil {
			return Version{}, fmt.Errorf("%q is not a release's version: %w", s, err)
		}
		n[i] = v
	}
	return Version{n[0], n[1], n[2], m[4]}, nil
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Stable says v is no prerelease.
func (v Version) Stable() bool { return v.Pre == "" }

// Compare is -1, 0 or 1 as v is older than, the same as or newer than w, by
// semver's precedence: a prerelease comes before its release.
func (v Version) Compare(w Version) int {
	if c := cmp.Compare(v.Major, w.Major); c != 0 {
		return c
	}
	if c := cmp.Compare(v.Minor, w.Minor); c != 0 {
		return c
	}
	if c := cmp.Compare(v.Patch, w.Patch); c != 0 {
		return c
	}
	switch {
	case v.Pre == w.Pre:
		return 0
	case v.Pre == "":
		return 1
	case w.Pre == "":
		return -1
	}
	a, b := strings.Split(v.Pre, "."), strings.Split(w.Pre, ".")
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := comparePre(a[i], b[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a), len(b))
}

// comparePre orders two prerelease identifiers: numbers by value, before
// any alphanumeric one, and those by their ASCII text.
func comparePre(a, b string) int {
	na, errA := strconv.Atoi(a)
	nb, errB := strconv.Atoi(b)
	switch {
	case errA == nil && errB == nil:
		return cmp.Compare(na, nb)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	}
	return strings.Compare(a, b)
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

// Offered says an update offers latest over a build of over: a release,
// never a prerelease, newer than it. A prerelease is installed only when it
// is named, so a stable build is never moved onto one.
func Offered(latest, over string) bool {
	v, err := ParseVersion(latest)
	return err == nil && v.Stable() && Newer(latest, over)
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
// 3.0.0-linux-x64 or 3.1.0-rc.1-linux-x64, and which release.
func PlatformBuild(version string) (string, bool) {
	for _, system := range []string{"-linux-", "-darwin-"} {
		at := strings.LastIndex(version, system)
		if at <= 0 {
			continue
		}
		if _, err := ParseVersion(version[:at]); err == nil {
			return version[:at], true
		}
	}
	return "", false
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
