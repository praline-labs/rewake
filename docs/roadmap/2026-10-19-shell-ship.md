# The shell, step 3: shipping

October 19, 2026. **Done.** `shell/build.ts` compiles the shell with Bun and the Solid plugin into
one executable per target, reading no `bunfig.toml`, `.env`, `tsconfig.json` or `package.json` of
the directory it runs in. `scripts/pack.sh` installs every Linux native library of the lockfile
and puts `bin/rewake-shell` beside the binary in both platform packages: 99 MB, 49.5 MB packed.
The release gate checks its upload, its architecture and, after the install, that the installed
shell draws main's output; live, an npm-installed package ran `rewake --shell --main claude`.
A build from the checkout compiles the shell beside itself or keeps `REWAKE_SHELL`
([install.md](../install.md#trying-20-beside-103)). **Open.** Step 2's clipboard check.
