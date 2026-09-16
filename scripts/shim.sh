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
package="rewake-$platform-$architecture"

# The platform package is found the way Node finds a dependency: the nearest
# node_modules first, starting inside this package and going up. npm places a
# dependency wherever that search reaches it — nested, beside, or hoisted
# several levels up — so a fixed list of places misses some of them, and a
# stale copy beside this package must not win over this package's own.
dir="$(cd "$(dirname "$self")/.." && pwd -P)"
while :; do
  if [ "$(basename "$dir")" != node_modules ]; then
    candidate="$dir/node_modules/@iiiokojiadbi/$package/bin/rewake"
    if [ -x "$candidate" ]; then
      exec "$candidate" "$@"
    fi
  fi
  [ "$dir" = / ] && break
  dir="$(dirname "$dir")"
done

echo "rewake: no binary for $platform-$architecture was installed." >&2
echo "Install it with: npm install -g @iiiokojiadbi/$package" >&2
exit 1
