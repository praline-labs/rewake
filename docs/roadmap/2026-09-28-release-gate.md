# A release gate for npm

September 28, 2026. Before the first public release, 1.0.0 on npm under
`@praline-labs`, by the owner's decision ([install.md](../install.md#releasing-to-npm)).
`scripts/pack.sh` built the packages and published nothing, and nothing stood between
it and a hand-typed `npm publish`.

## What was found

- Publishing is irreversible and nothing checked it: not the tree, not whether the
  version was taken, not what each package would upload, not that the binary for one
  platform was built for it.
- The usual version check swallows every error of `npm view`, which fails the same way
  for a missing version and for a registry it could not reach, so a network failure
  reads as a free version.
- The packages named no repository: the registry's page would link nowhere, and the
  README's relative links would break there.
- The README said "Not published yet" and named no harness it works with.

## What was done

- `tools/release <version>`, a Go program beside `tools/checksummary` and
  `tools/harnesscache`: it reads npm's JSON answers, which POSIX sh cannot without a
  dependency, and its decisions are tested with a fake npm and git in the five checks.
  Without `--publish` it publishes nothing; a dry run goes on past a failed check to
  show every problem and exits 1. It checks a clean tree whose HEAD `origin/main`
  contains, a semver version free for all three packages — only npm's `E404` frees it,
  anything else stops the release as unsafe to decide — builds with `pack.sh`, checks
  each manifest and the upload against an allowlist both ways, each binary's ELF
  architecture, this machine's `rewake --version` and build, the shim installed with and
  without its platform package, and the shim's own tests.
- `--publish` uploads the two platform packages and then the entry, `--access public`,
  to the public registry named for the scope too, with `--otp` when given; it stops at
  the first failure naming what was and was not published, asks the registry for each
  package and the entry's `latest` afterwards without retrying, and prints the tag
  without creating it.
- The default version in `internal/cli/registry.go` stays what a build from source
  reports, by the owner's decision: the release commit sets it to the release's
  version, and the gate refuses before the build when the line at HEAD differs, naming
  the file and the line to write, so the tagged source cannot build a binary naming
  another version than npm's.
- `pack.sh` gives every package its `repository`, which the gate checks.
- The README installs from npm and from source and names Claude Code and Codex;
  `install.md` describes the release.

## Review

review-codex confirmed three findings in the gate, fixed the same day:

- **P2:** an inherited `npm_config_dry_run=true` makes `npm publish` skip the upload and
  exit 0, and the gate counted the packages as published. `npm publish` now gets
  `--dry-run=false`, and a package not visible after the uploads ends the run with 1,
  no tag, and the `npm view` to repeat.
- **P2:** a transport failure during an upload — the request stored, the answer lost —
  was reported as nothing published and the version free. Only a 4xx or `EOTP` now
  means not published; anything else is asked about once, and a package still absent
  or not askable is reported as unknown, its version not to be reused until checked.
- **P2:** `1.0.0+build.1` was accepted, checked and tagged, and npm would have
  published it as `1.0.0`. Build metadata is refused.

## What stays open

- Nothing: the dry run on the pushed release commit passed and 1.0.0 was published the
  same day ([the record](2026-09-28-release-1.0.0.md)).
