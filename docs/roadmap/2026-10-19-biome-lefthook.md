# Biome for the TypeScript, Lefthook on every commit

October 19, 2026, by the owner's decision. `biome.json` at the root formats and lints the shell,
the mod and pi's extension with the owner's style and the recommended rules, leaving out the
generated `protocol.gen.ts`; one commit applied it to the existing sources, after the one lint
error was fixed by hand. `lefthook.yml` runs on every commit: Biome over staged TypeScript,
`golangci-lint fmt` over staged Go, then build, vet and lint of the module. Tried in a private
clone and its linked worktree: a TypeScript and a Go commit formatted, a lint finding stopping
the commit. Open: `lefthook install` in the shared clone on landing; 28 warnings and 34 infos
of Biome left as they are.
