#!/bin/sh
# Build the npm packages into dist/npm, without publishing anything.
#
# The layout follows what esbuild does: one package per platform holding the
# binary, and one entry package that depends on them optionally and execs
# whichever one npm installed. The entry's bin is a POSIX script rather than a
# Node shim on purpose — it execs the binary and disappears, so the terminal,
# the signals and the exit code belong to rewake itself and not to a process in
# between.
#
# Usage: scripts/pack.sh [version]
set -eu

version="${1:-0.0.1}"
scope="@praline-labs"
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

# One package per platform. The list is short on purpose: a platform nobody
# builds for is a package that installs and then cannot run.
for target in linux-amd64:linux-x64 linux-arm64:linux-arm64; do
  goos_goarch="${target%%:*}"
  npm_name="${target##*:}"
  goos="${goos_goarch%%-*}"
  goarch="${goos_goarch##*-}"

  dir="$out/rewake-$npm_name"
  mkdir -p "$dir/bin"
  # The version reaches the binary too: a package that says 0.9.9 while its
  # rewake --version says something else is a package nobody can place.
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -C "$root" -trimpath \
    -ldflags "-X github.com/praline-labs/rewake/internal/cli.Version=$version -X github.com/praline-labs/rewake/internal/cli.built=$built" \
    -o "$dir/bin/rewake" ./cmd/rewake

  cat > "$dir/package.json" <<JSON
{
  "name": "$scope/rewake-$npm_name",
  "version": "$version",
  "description": "rewake binary for $npm_name",
  "license": "MIT",
  $repository,
  "os": ["$goos"],
  "cpu": ["$( [ "$goarch" = "amd64" ] && echo x64 || echo "$goarch" )"],
  "files": ["bin/rewake", "LICENSE"]
}
JSON
  cp "$root/LICENSE" "$dir/LICENSE"
done

# The entry package: the name people install.
entry="$out/rewake"
mkdir -p "$entry/bin"
cat > "$entry/package.json" <<JSON
{
  "name": "$scope/rewake",
  "version": "$version",
  "description": "Let the coding agents running on your machine message each other",
  "license": "MIT",
  $repository,
  "bin": { "rewake": "bin/rewake" },
  "files": ["bin/rewake", "README.md", "LICENSE"],
  "optionalDependencies": {
    "$scope/rewake-linux-x64": "$version",
    "$scope/rewake-linux-arm64": "$version"
  }
}
JSON
cp "$root/README.md" "$entry/README.md"
cp "$root/LICENSE" "$entry/LICENSE"

cp "$root/scripts/shim.sh" "$entry/bin/rewake"
chmod +x "$entry/bin/rewake"

echo "packages built in $out"
