#!/bin/sh
# Build the npm packages into dist/npm, without publishing anything.
#
# One package name carries everything. Each platform build is a version of that
# same package, <version>-linux-x64 and <version>-linux-arm64, published under a
# dist-tag of its own so it never takes latest; the entry, <version> under
# latest, names them in optionalDependencies through npm aliases, and npm
# installs only the one whose os and cpu match this machine. A registry name per
# platform would be a separate project to own for every platform ever built; a
# version suffix costs nothing (owner decision, September 28, 2026).
#
# The entry's bin is a POSIX script rather than a Node shim on purpose — it
# execs the binary and disappears, so the terminal, the signals and the exit
# code belong to rewake itself and not to a process in between.
#
# Usage: scripts/pack.sh [version]
set -eu

version="${1:-0.0.1}"
name="@praline-labs/rewake"
root="$(cd "$(dirname "$0")/.." && pwd)"
# Where the source lives. The registry's page links to it, and resolves the
# README's relative links against it; without it they lead nowhere.
repository='"repository": { "type": "git", "url": "git+https://github.com/praline-labs/rewake.git" }'
out="$root/dist/npm"
# The build time, which the binary cannot know otherwise: rewake --version
# shows it, so two builds of one revision can be told apart.
built="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

rm -rf "$out"
mkdir -p "$out"

# One build per platform. The list is short on purpose: a platform nobody
# builds for is a version that installs and then cannot run.
for target in linux-amd64:linux-x64 linux-arm64:linux-arm64; do
  goos_goarch="${target%%:*}"
  platform="${target##*:}"
  goos="${goos_goarch%%-*}"
  goarch="${goos_goarch##*-}"

  # Staged under the alias the entry installs it as: both builds publish under
  # one name, and two directories keep them apart until then.
  dir="$out/rewake-$platform"
  mkdir -p "$dir/bin"
  # The version reaches the binary too: a package that says 0.9.9 while its
  # rewake --version says something else is a package nobody can place. The
  # binary names the release, not the platform build it arrived in.
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -C "$root" -trimpath \
    -ldflags "-X github.com/praline-labs/rewake/internal/cli.Version=$version -X github.com/praline-labs/rewake/internal/cli.built=$built" \
    -o "$dir/bin/rewake" ./cmd/rewake

  cat > "$dir/package.json" <<JSON
{
  "name": "$name",
  "version": "$version-$platform",
  "description": "rewake binary for $platform, installed by the rewake command as an optional dependency",
  "license": "MIT",
  $repository,
  "os": ["$goos"],
  "cpu": ["${platform##*-}"],
  "files": ["bin/rewake", "LICENSE"]
}
JSON
  cp "$root/LICENSE" "$dir/LICENSE"
done

# The entry: the version people install. Each alias is scoped, so no package
# outside the organization can ever sit where the shim looks for its binary.
entry="$out/rewake"
mkdir -p "$entry/bin"
cat > "$entry/package.json" <<JSON
{
  "name": "$name",
  "version": "$version",
  "description": "Let the coding agents running on your machine message each other",
  "license": "MIT",
  $repository,
  "bin": { "rewake": "bin/rewake" },
  "files": ["bin/rewake", "README.md", "LICENSE"],
  "optionalDependencies": {
    "@praline-labs/rewake-linux-x64": "npm:$name@$version-linux-x64",
    "@praline-labs/rewake-linux-arm64": "npm:$name@$version-linux-arm64"
  }
}
JSON
cp "$root/README.md" "$entry/README.md"
cp "$root/LICENSE" "$entry/LICENSE"

cp "$root/scripts/shim.sh" "$entry/bin/rewake"
chmod +x "$entry/bin/rewake"

echo "packages built in $out"
