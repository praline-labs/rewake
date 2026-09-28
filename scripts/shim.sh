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
# The alias the entry installs this platform's build under: a version of
# @praline-labs/rewake itself, placed in node_modules by this name.
package="rewake-$platform-$architecture"

# The platform build is found the way Node finds a dependency: the nearest
# node_modules first, starting inside this package and going up. npm places a
# dependency wherever that search reaches it — nested, beside, or hoisted
# several levels up — so a fixed list of places misses some of them, and a
# stale copy beside this package must not win over this package's own.
dir="$(cd "$(dirname "$self")/.." && pwd -P)"
while :; do
  if [ "$(basename "$dir")" != node_modules ]; then
    candidate="$dir/node_modules/@praline-labs/$package/bin/rewake"
    if [ -x "$candidate" ]; then
      exec "$candidate" "$@"
    fi
  fi
  [ "$dir" = / ] && break
  dir="$(dirname "$dir")"
done

echo "rewake: no binary for $platform-$architecture was installed." >&2
case "$platform-$architecture" in
  linux-x64|linux-arm64)
    # The build is an optional dependency, and --omit=optional, or npm's
    # omit setting, leaves it out without a word.
    echo "It comes as the optional dependency @praline-labs/$package; reinstall with optional dependencies included:" >&2
    echo "npm install -g @praline-labs/rewake" >&2 ;;
  *)
    echo "rewake is built for linux-x64 and linux-arm64 only; to build it from source, see https://github.com/praline-labs/rewake" >&2 ;;
esac
exit 1
