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
scope="@iiiokojiadbi"
root="$(cd "$(dirname "$0")/.." && pwd)"
out="$root/dist/npm"

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
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -C "$root" -trimpath -o "$dir/bin/rewake" ./cmd/rewake

  cat > "$dir/package.json" <<JSON
{
  "name": "$scope/rewake-$npm_name",
  "version": "$version",
  "description": "rewake binary for $npm_name",
  "license": "MIT",
  "os": ["$goos"],
  "cpu": ["$( [ "$goarch" = "amd64" ] && echo x64 || echo "$goarch" )"],
  "files": ["bin/rewake"]
}
JSON
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
  "bin": { "rewake": "bin/rewake" },
  "files": ["bin/rewake", "README.md"],
  "optionalDependencies": {
    "$scope/rewake-linux-x64": "$version",
    "$scope/rewake-linux-arm64": "$version"
  }
}
JSON
cp "$root/README.md" "$entry/README.md"

cat > "$entry/bin/rewake" <<'SHIM'
#!/bin/sh
# Find the binary npm installed for this platform and become it.
set -eu

case "$(uname -s)" in
  Linux) platform=linux ;;
  Darwin) platform=darwin ;;
  *) echo "rewake: unsupported system $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) architecture=x64 ;;
  aarch64|arm64) architecture=arm64 ;;
  *) echo "rewake: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

# npm installs the command as a symlink in its bin directory, so the script has
# to follow the link to find the package it lives in. Without this it looks for
# the binary next to the symlink and reports it missing.
self="$0"
while [ -L "$self" ]; do
  target="$(readlink "$self")"
  case "$target" in
    /*) self="$target" ;;
    *) self="$(dirname "$self")/$target" ;;
  esac
done
here="$(cd "$(dirname "$self")" && pwd -P)"
package="rewake-$platform-$architecture"
for candidate in \
  "$here/../../$package/bin/rewake" \
  "$here/../node_modules/@iiiokojiadbi/$package/bin/rewake" \
  "$here/../../../@iiiokojiadbi/$package/bin/rewake"
do
  if [ -x "$candidate" ]; then
    exec "$candidate" "$@"
  fi
done

echo "rewake: no binary for $platform-$architecture was installed." >&2
echo "Install it with: npm install -g @iiiokojiadbi/$package" >&2
exit 1
SHIM
chmod +x "$entry/bin/rewake"

echo "packages built in $out"
