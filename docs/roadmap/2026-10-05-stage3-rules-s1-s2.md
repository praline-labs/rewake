# Stage 3 of 2.0: the rules accepted, S1 and S2 built

Stage 3 of 2.0 builds the core of the rewrite in nineteen steps, S1–S19
([stage3.md](../v2/stage3.md)). On October 5, 2026 its rules were written and accepted
after seven review rounds, S1 was built and accepted after four, and S2 was built and
accepted after two. **The stage is not closed**: S3 was accepted on October 6
([its entry](2026-10-06-stage3-s3.md)), and S4–S19 follow in the order of the build
table.

## The rules, in seven rounds

Every round was a Codex-side review of the documents alone, before any code.

1. **Refused, fifteen findings.** The step order was not buildable as written: S2
   removed the event producer the retained test rig drove, S6 and S7 split files from
   their receiver type and moved core code that took upper-layer values, the launch
   helpers' destination made an adapter import the host, deleting all of
   `turn_records.go` took journal retention with it, and the temporary suite gate was
   two declaration exceptions where the case policy rejects every unsupported result.
   The fixture's liveness waited on the operation it refused; the state root confused a
   room with a root; the lease rules called writers reads; clearing 1.x was not fenced
   against a new 1.x launch; the build identity had a weaker fallback. The operator
   decision broadened authority past the report's recipient, both froze and rewrote the
   same evidence, and gave a resend no lifecycle. Moving a document to the archive
   unchanged left its relative links pointing at siblings that stayed behind. The review
   named four classes behind them: a file group is not a dependency boundary, a gap
   label is not an executable gate, recovery must define composition, and a read or an
   absence needs an explicit contract.
2. **Refused, eleven findings.** Most of round 1 closed. Left: the fixture could not
   confirm an interim end through S5's contract, S14's test moves still crossed removed
   dependencies, S3 kept another test that lifted a stop by deleting its cause, S17
   removed directory creation before its replacement, an abandoned upgrade rename read
   as passed checks, a competing clearer bypassed the refusal for live 1.x sessions,
   and the decision's recovery deadlocked on its own child decision, froze only one
   writer of the original evidence, had no proven-absent resend state and let resend
   identities grow without bound.
3. **Refused, six findings.** The order was largely repaired; the recovery contract
   still failed under ordinary journal progress (a decision could not recognize its
   resend once that had saved progress, completed or been swept), promised decision
   attribution by writers that never learn of the decision, reused a recurring cause's
   earlier resolution, took a retry of a held end for its continuation, and admitted a
   launch beside a live old session. The review proposed the form correction 9 took.
4. **Split into two parts.** Recovery after an operator decision had drawn findings in
   every round while the rest converged, so the stage was accepted in two parts with a
   verdict each: part A, S1–S17 and S19; part B, S18, the operator decision. Part A was
   accepted with two local corrections to S5's held-end identity, applied; part B was
   refused on three findings. Two probes run through a test overlay confirmed the
   carried behaviour part B had to account for: a report recorded moot beside a letter
   that had landed, and a parent's wait left untouched behind an unknown child.
   Committed as `639acd4`, part B marked under review.
5. **A publication race found.** A probe showed an ordinary retry landing one report
   twice: the retry passes the recipient's liveness check, the run ends, the next run's
   sweep retires the letter and its mark, and the retry, finding no proof, writes again.
   The carried code checks liveness before the recipient's lock and reads the evidence
   outside it. Main placed the fix in part A as the first commit of S3. The same round
   accepted S3's reading of a stop: an open occurrence is resolved only by evidence, and
   a moot disposition never closes it, and refused part B again: proof retention and
   the evidence domain still let the race through. Committed as `cae5c56`.
6. **Refused, four findings.** The publication contract,
   [stage3-publication.md](../v2/stage3-publication.md), holds the recipient's lock from
   the liveness check through the evidence reads to the write, and retires proof only
   there, by the live run; the review found its ordering sound, and confirmed a further
   gap the writer reported, a sweep of an ended run's server retiring a live
   replacement's proof. Its acceptance schedule did not kill its named mutation, one
   test turned a saved published outcome into moot, and an edit displaced S4 in the
   build table. Part B still excluded one execution the contract permits.
7. **Accepted in full.** Part A's publication contract, with two test wordings applied
   as the review gave them, and part B. Committed as `3106ed7`.

## S1: the tests first, in four rounds

S1 put in place the checks that hold the shape of 2.0 from the first step: the layout
test over every build variant with exception tables that shrink as packages move, the
rules of 2.0 in [rules/](../rules/README.md), each naming tests a check of the five is
shown to run, the archive rule, and one Markdown reader that refuses what it cannot
place. The review's question each round was what the checkers let through.

1. **Refused, seven findings.** An exception excused new violations in the same file;
   discovery missed packages built only under a tag and registries in test files; the
   import loader dropped edges before the rule judged them; a rule examined the quoted
   token rather than the string's value; a named function was not shown to be a test
   the checks run; deleting a rule's whole `Tests:` block dropped the rule from checking;
   the `-X` check was not tied to the package the suite compiles.
2. **Refused, four findings.** Deduplication hid a new registry beside an excused one;
   the `-race` run was modelled as an ordinary build; a permitted flag could still
   suppress every named test; a table example outside the build order counted as a step.
3. **Refused, three findings.** Fenced examples still counted as steps and rules; the
   command's own `GOFLAGS` prefix could bypass the positive-count check; two mutants
   reported equivalent changed an observable rejection.
4. **Accepted** under main's stated threat model, accidental drift rather than a change
   built to fool the checker, with two documentation clarifications applied.

Each round's counterexamples were probes added through a test overlay, run by main; each
fix turned them into rejection tests through the full discovery path, and the surviving
mutants were classified one by one. An exception now records how many findings it
excuses and which; fences are recognized by character and length; commands are read
only from the first code block under Checks, in the direct `go test` form. Committed as
`0100db4`.

## S2: the 1.x migration leaves

S2 removed what read records an earlier rewake wrote: the conversion of 1.x turn
records into journals, the earlier-build machinery, `TurnsPath` and the cutover package.
Run identity and liveness stay; journal retention stays, as
`internal/inbox/journal_retention.go`. The tests that used conversion records were
rebuilt on a two-session lab of current journals, nine mutants run one at a time were
all killed, and the workflow suite gave the same outcome per case before and after.
[protocol-cutover.md](../archive-1.x/protocol-cutover.md) moved to the archive unchanged.

The Codex-side review found no blocking defect in the code and refused on the
documentation: E8 and the recovery sequence still described the conversion's mailbox-wide
collection of closed obligations and its `superseded` decision, and two documents
described S3's evidence-only stop resolution as today's behaviour. Both were corrected:
the barrier completes journals one by one, which never overlap, so no report is decided
superseded; and today a reading stop lifts once its cause is gone, the gap S3 closes. The
follow-up review accepted the corrections, Codex side included, and S2 landed in the
commit that adds this entry.

## What stays open

- S4–S19 in the order of the build table, S18 on part B's accepted rules; S3 is in
  [its own entry](2026-10-06-stage3-s3.md).
- The gaps each rule in [rules/](../rules/README.md) names, each with the step that
  closes it; from S19 any gap fails the rules test.
