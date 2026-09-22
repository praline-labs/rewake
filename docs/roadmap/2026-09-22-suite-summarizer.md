# A summary of a suite run instead of its transcript — done, September 22, 2026

A workflow suite run prints hundreds of lines, and an agent reads all of them to find
two things: how many scenarios ran, and which observation failed. `tools/checksummary`
removes that cost. It runs the engine, reads `go test -json`, and prints a few lines —
the scenarios, the cases by outcome, the absent capabilities with the column they are
absent from, and the path to a `summary.json` holding everything else. A failing case
brings its name, its harness column, the observation that failed by name, the detail,
and one path to that case's evidence.

It reads records, not prose. Every case now prints one line of JSON when its verdict is
published, and the run prints one naming the scenarios that ran; the summarizer reads
those two tags and ignores every other line. That is what makes the budget in
[check-runner.md](../check-runner.md) hold rather than merely be stated: a print added
to a scenario tomorrow cannot grow the summary, because nothing but a tagged record is
read. A tagged line it cannot parse fails the run rather than being skipped — a summary
of the part of a run that happened to be readable says nothing about the rest.

**The shape of those records lives in one package both ends import**,
`test/workflow/record`: the two tags and three structures, in plain strings, with the
suite's own outcome type converted once where a case publishes. The first version
declared them twice — the suite in its test files, the summarizer in its own — and
nothing connected the two but a comment claiming the end-to-end run would catch a
rename. It would not have: that run asserted four fields of eight, so renaming the
harness, the reason, the evidence or a duration stayed green while a column quietly
emptied. With one package a rename is a compile error, and the end-to-end run is left
proving what a compiler cannot: that a real suite run round-trips through this program
— records printed in the volume and order a real run produces them, and a green
summary inside its budget. The package sits outside `cmd/`, nothing shipped imports it,
and `go list ./...` finds it, so the five checks cover it.

There is deliberately no `--json` on standard output. The machine-readable form is the
file under `--into`, and a second copy on the console would be the transcript this
program exists to replace.

Two facts the build turned up, both about a run that has already gone wrong. The run
record was published only when every test passed, so a red run had no scenario count at
all — the first question about a red run is whether the case that failed was the only
thing that ran, and it could not be answered. It is published on both paths now. And an
engine that fails with no case reporting it — a build error, a panic, a package timeout
— must not come out green for want of anything red to point at, so the summarizer keeps
the engine's own exit code and says when it is the only thing that failed. Running the
engine rather than reading a pipe is what makes that number exact.

Where it lives and what it leaves alone. `tools/` rather than `cmd/`, because `cmd/` is
what the project ships and [design.md](../design.md) names `cmd/rewake` as the entry
point; a second binary there would say this one is installed for somebody. It does not
run the five checks from `AGENTS.md`: each of those is already one command with one line
of output and prints its findings in its own words, and wrapping them would add a second
place where the list can be forgotten. The suite is where the hundreds of lines are.

The summary names which column a red result came from and stops there. Codex is the
regression gate and Claude Code the search column, and
[check-runner-scenarios.md](../check-runner-scenarios.md) forbids a runner from
promoting the second on its own; saying which column is the whole of what it may do
with that difference.
