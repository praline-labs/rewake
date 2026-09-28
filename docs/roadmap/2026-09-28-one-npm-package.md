# One npm package for rewake

September 28, 2026, for release 1.0.2. The owner decided the same day that one tool is
one npm project: 1.0.0 and 1.0.1 went out as three, `@praline-labs/rewake` with the
separate platform packages `@praline-labs/rewake-linux-x64` and
`@praline-labs/rewake-linux-arm64` (the esbuild pattern the September 16 design named).
The model is the owner's other project, whose platform builds are versions of its one
package.

## What was done

- `scripts/pack.sh` builds three uploads, all named `@praline-labs/rewake`: the entry
  `<version>`, with the shim, the README and the builds in `optionalDependencies` through
  npm aliases, `"@praline-labs/rewake-linux-x64": "npm:@praline-labs/rewake@<version>-linux-x64"`
  and the same for arm64; and each build as the version `<version>-linux-x64` or
  `<version>-linux-arm64`, with its `os` and `cpu`, the binary and `LICENSE`. The aliases
  are scoped: no package from outside the organization can sit where the shim looks.
- `scripts/shim.sh` looks for the alias in `node_modules` as before, nested first and
  then upwards; without it, it names the reinstall of the entry with its optional
  dependencies, since a build has no package of its own to install, and on a platform
  nobody builds for it points to the source.
- `tools/release` asks the registry for each of the three versions exactly, under the
  same rule that only npm's `E404` frees one; checks each upload's name and version, a
  build's `os`, `cpu` and missing `bin`, and the entry's `optionalDependencies` as
  exactly the aliases to this release's builds; refuses a build's own version, such as
  `1.0.2-linux-x64`, as a release; publishes the builds first under `--tag linux-x64` and
  `--tag linux-arm64`, the entry last under `--tag latest` (`next` for a prerelease); and
  afterwards wants each version visible and each tag on its upload before it prints the
  git tag.
- The hand-laid `node_modules` of the old smoke check could not show npm resolving an
  alias. The gate now serves the release from a registry in its own process, on the
  loopback interface, and has npm 11 install it from there with fresh configuration and
  cache: this machine's build alone, whose command runs; the arm64 build alone under
  `--cpu=arm64`; and no build under `--omit=optional`, whose command fails naming the
  reinstall.
- The docs: [design.md](../design.md#distribution) and
  [install.md](../install.md#releasing-to-npm) carry the decision and the procedure; the
  local test install is the gate's dry run, since an install from tarballs on disk cannot
  resolve an `npm:` alias; the uninstall is `npm uninstall -g @praline-labs/rewake`.

## How the aliasing was verified

- Against the public registry, with npm 11.17.0: the owner's other project, published in
  this layout, installed into an empty prefix got its entry and only the x64 build under
  its alias, and with `--cpu=arm64` only the arm64 one; `npm ls` showed the other builds
  as unmet optional dependencies.
- Against the gate's own registry, with the built 1.0.2 uploads: the same, and the
  installed command ran the release's binary.
- A finding on the way: npm 11.17 installs a global package's optional dependencies even
  under `--omit=optional`; into a project it omits them. The gate's check without a build
  installs into a project for that reason.

## What stays open

- The old packages `@praline-labs/rewake-linux-x64` and `-linux-arm64` were removed from
  the registry by the owner on September 28, 2026, so 1.0.0 and 1.0.1 now install the
  shim without a binary. They are to be deprecated once 1.0.2 is out, by main with the
  owner.
- 1.0.2 is published only on the owner's word, through the gate.
