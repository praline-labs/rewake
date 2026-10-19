# pi built into rewake

October 19, 2026, by the owner's decision that pi is part of rewake. **Done.** The npm entry
depends on pi at the exact `PinnedVersion` (1.3.0); a launch without `--command` finds it in a
`node_modules` above the binary or through `REWAKE_PI`, runs it with `node` (22.19 or later) and
refuses any other version before the claim; `--command` keeps the 1.2.0 minimum with a note.
Every pi rewake starts keeps its state in `~/.config/rewake/pi/agent` (0700), and drops `-ne` and
`-na`, so the person's, packages' and the project's extensions load beside rewake's. The release
gate's registry forwards pi to the public one and checks the installed copy. Live in a private
HOME ([research-pi.md](../research-pi.md#the-bundled-pi)). **Open.** The owner's `/login` and a
model's answer; a project's trust prompt holds a run until answered; no lock of pi's own tree.
