---
description: Briefs — a markdown brief per task that main writes, bound to workers by send --brief, ticked and commented by them, read and commented by a reviewer given it with --review; the records under briefs/, where a brief lives (the room or a folder of the project), the brief commands, the JSON the shell reads, the skills a brief gives, copied from main's harness with the skills they link, and the worker tools of both harnesses.
---

# Briefs

Decided by the owner, October 26, 2026: a **brief** is a markdown document for one task — a
title, a description, checkbox items, anything markdown holds — kept in the room, many per room.
Main alone writes and edits its text; a worker it was given to ticks items and writes comments of
at most 1000 characters, and a reviewer it was given to only comments; the person reads briefs in the shell, in the worker's pane, and edits nothing there. A brief
outlives compactions of main and workers; a closed one goes to an archive. Main gives one with
`rewake send <worker> "..." --brief <id>`, and the worker gets its text with that task, and only
then. The finish report stays mail and is not copied into the brief.

Decided by the owner, October 27, 2026: a reviewer gets a brief to read **and comment**, never to
tick («делай второе»), with `send --brief <id> --review`; its findings land in the brief where the
writer and the person see them, and its verdict still reaches main as the report. Every comment
shows who wrote it and in what capacity, so the models reading it know its author («показывать
там, кто сделал комментарий, чтобы модели получали инфу о авторе комментария»). The word is not the
briefing, the launch text of `internal/core/brief`; the code lives in `internal/core/taskbrief`
(the records), `internal/cli/brief*.go` and `send_brief.go` (the commands), and the tools in
`internal/adapter/claude/mod/hooks/briefs.ts` and `internal/adapter/pi/briefs.ts`. The shell's
view is in [shell.md](shell.md#the-briefs).

## The records

Under the room, beside `inbox/` and `spaces/`:

```
briefs/.lock                 one flock for every write under briefs/
briefs/<id>/brief.json       {id, created, bound: [{name, epoch, since, kind: work|review, checkout?, commit?}]}, closed once closed
briefs/<id>/log/<n>.json     {n, at, from, epoch, kind: edit|tick|untick|comment, edition, item, itemText | text, capacity}
briefs/<id>/text/<e>.md      the text of edition e, as main wrote it
briefs/<id>/skills/<e>/      the copies of the skills edition e names, and their skills.json (below)
briefs/closed/<id>/          a closed brief, moved whole
briefs/index.json            {briefs: {"<id>": {path, closed?}}}, the briefs kept outside the room
```

- **A brief's id** is a positive integer, the highest in `briefs/` and `briefs/closed/` plus one,
  taken under the lock: short to type in `--brief 3`, and nothing for main to invent. A new brief —
  `brief.json`, its first text and the edit of edition 1 — is written into `briefs/.tmp-<random>/`
  and renamed to `briefs/<id>`, so a brief is whole or absent; a crash leaves a `.tmp-*` directory,
  which is never a record.
- **The log** numbers every event of the brief from one counter: entry `n` is the highest plus one,
  published with `state.PublishExclusive` under the lock, never edited or removed. A tick's
  `itemText` is the item's text cut to 200 code points, a cut one ending in `…`: whole, a long
  item made an entry past the 64 KiB a reader takes. Every write of `brief.json` or an entry is
  refused, nothing written, when the record would pass that bound, so no accepted write leaves a
  brief its readers refuse. Two numbers
  stay apart. The **edition** counts main's edits, 1 for a new brief, and only an edit raises it:
  the edit writes `text/<e>.md` first and its log entry, naming the edition, last, so the entry is
  the commit point — a crash between leaves a text no entry names, and nothing changed. The
  **current text** is the latest edition's. The **revision** is the log number of the latest
  edit, tick or untick — comments leave it —, what main's edit is checked against.
- **The title** is the current text's first `# ` heading, else its first line, cut to 60
  characters.
- **An item** is a task-list line — `- [ ]`, `* [x]`, `1. [ ]`, nested or not, outside a fenced
  block — numbered from 1 in the order of the current text; the scan is a line scanner of rewake's
  own, no markdown dependency in Go. A worker names an item by that number, and the answer echoes
  its text. A tick or untick names the edition it was read in, and is refused, exit 1, when main
  has edited since: the refusal gives the current edition and says to read the brief again, so a
  number read in an older text never ticks another item. A worker's own ticks leave the edition,
  so it ticks one item after another with no re-read. A tick is recorded with its edition and the
  item's number, and applies only within that edition: an item is done when the latest tick or
  untick of the current edition says so, else as main's own `[x]` in the text says. Nothing is
  matched by text.
- **An edit keeps the ticks by folding them into main's text.** Main edits the current text with
  the ticks applied as `[x]` (`brief show <id> --markdown`), and `brief edit` names the revision it
  started from: refused, exit 1, unless that is still the brief's revision, so neither a tick nor
  an edit made since is written over; the refusal names the revision now and says to read it again.
  After the edit the ticks live in main's text, where an item deleted, moved or reworded takes its
  own mark with it. Rejected: ticks keyed by an item's text and its occurrence — two items "Run
  tests", the first ticked and then deleted by main, and the second read done.
- **Who writes what.** Every write is a change form holding the room's build lease, then
  `briefs/.lock` (`state.WithBriefLock`, in the seam's lock
  list of [rules/state.md](rules/state.md#what-is-a-writer)). Main writes `brief.json` (`WriteAtomic`), the texts and their edits,
  and moves a brief to `closed/` (`Rename`); a bound run only adds ticks and comments to `log/`,
  after checking under the same lock that the brief is open, its run is bound and, for a tick,
  that the binding is `work` and its edition is current. A view takes no lock: each file is whole by the atomic write, the set of them
  is no snapshot. The lock is taken after the lease and with no other lock of the room held, and
  nothing is taken inside it.
- **A binding's kind and a comment's capacity.** A binding's `kind` is its right: `work` reads,
  ticks and comments, `review` reads and comments. A comment's `capacity` is the kind of its
  writer's binding when it wrote, kept as written though the binding changes later. Both are
  additive: a binding written before them has no `kind` and is `work`, a comment none and shows
  its writer's name alone. A review binding given a checkout names its path and commit
  ([below](#a-reviewers-checkout)), additive too.
- **The letter.** A task or question sent with `--brief` carries `brief: "<id>"`, an additive
  field of the message record ([mailbox-records.md](mailbox-records.md)); the brief's text is not copied into it.

## Where a brief lives

Decided by the owner, October 27, 2026: briefs are meant to outlive restarts, and the room is the
temporary directory, so a brief may live in a folder the project names, beside its own task
records, without main naming it each time. Three homes, in the order a brief is looked for:

- **The room**, as above: `briefs/<id>/`, `briefs/closed/<id>/`. The default, and where every
  brief made before this stays; it needs no entry anywhere.
- **The project's folder**: `[briefs] dir = "<path>"` in the project's `.rewake/aliases.toml`
  ([user-guide.md](user-guide.md#a-projects-rewake)), relative to main's directory, read from the
  directory in main's session record, so a worker outside the project finds the same folder. A
  new brief goes to `<dir>/brief-<id>/`, made if the folder is missing. The user's file may not set
  it. A project file that does not read refuses `brief new` rather than put the brief in the room.
- **Another folder**, `rewake brief new --dir <path>` (the directory must exist): the brief goes to
  `<path>/brief-<id>/` and wins over the setting; a shift task's folder is the case it is for.

A brief outside the room keeps the same format in its folder (`brief.json`, `log/`, `text/`), and
its `brief.json` records `room`, the room's name: a folder rooms share holds only this room's briefs
to it, and another room's `brief-<id>` is listed nowhere and never written. The room keeps
`briefs/index.json`, `id → folder` for every brief kept outside it, the project's folder's
included, written under the brief lock after the folder is renamed into place (a crash between
leaves a folder no index names, which the project's folder scan still finds). The index is what
makes a moved brief known and keeps its id taken; the scan of the project's folder is what finds its
briefs again when the room was wiped with the temporary directory and the index with it. A brief
made with `--dir` is forgotten by such a wipe, a known limit.

A brief that goes with a task kept in folders, a shift task or any tracker with a folder per task,
takes that task's folder per brief, with `--dir <task folder>`; `[briefs] dir` is only the default
for a brief with no folder named. The folder is the link: the brief need not retell the task, it
points to the task's own files. Close the brief (`rewake brief close <id>`) before the task's
folder is moved or archived; a brief whose folder moved first lists as unreadable "moved", and only
`brief close` still works on it.

- **Ids** stay the room's: past the highest of the room's directories, `closed/`, the index and
  every `brief-<id>` in the project's folder, whatever room made it, so no name collides.
- **Moved.** An indexed brief whose folder is gone or holds no `brief.json` — a task archived with
  its folder — or holds another room's brief now, lists in `unreadable` as `{path, why: "moved"}`, so `--json` readers and the shell
  need no new field. `show`, `tick`, `comment`, `edit` and `send --brief` answer that the brief
  moved, exit 1, and nothing else fails. `brief close` of it marks its index entry `closed` and
  never removes the entry: no id is taken again while the room lives.
- **Close** of a brief in a project's folder records `closed` in its `brief.json` where it is, and
  does not move it to `closed/`: the folder is the project's. It lists as closed all the same.
- **The writes** go through the seam like every other: `state.WithOutside(room, folder, fn)` admits
  writes under one folder for the length of `fn`, while the room's lease is held and under the
  brief lock, and only the briefs' package calls it
  ([rules/state.md](rules/state.md#what-is-a-writer)). The lock stays the room's one
  `briefs/.lock`: allocating an id and writing the index need a single lock, and a lock file in a
  project's folder would not outlive its move.

## The commands

The `brief` group of the command table, with `--json` on each, examples parsed as every other:

| Form | Who | Class | What |
|---|---|---|---|
| `brief new <text>` or `--file <path>`, `[--dir <path>]` | main | change | a new brief; answers its id and title. Text at most 64 KiB, not empty. Where it goes: [above](#where-a-brief-lives); the skills its `Skills:` line names are copied in ([below](#skills)) |
| `brief edit <id> --rev <n> <text>` or `--file <path>` | main | change | replaces the text whole, refused unless `n` is still the brief's revision; the ticks since the last edit end there; the named skills are copied again |
| `brief close <id>` | main | change | moves it to `closed/`; every binding ends, the skills' copies go, and the reviewers' checkouts go unless they hold work |
| `brief list [--closed]` | main | view | open briefs, or closed ones too: id, title, done of items, comments, who is bound and whether that run lives |
| `brief show <id> [--markdown]` | main, a bound run | view | a header with the edition and the revision, the bound runs, the text with ticks applied and each item's number, the comments; `--markdown` prints only the text with ticks applied, no numbers, for main to edit |
| `brief tick <id> <n> --edition <e> [--undo]` | a run bound as `work` | change | ticks or unticks item `n` of edition `e`, refused once main edited since; answers the item's text |
| `brief comment <id> -- <text>` | a bound run | change | markdown, at most 1000 characters (code points), not empty; recorded with the binding's kind as its capacity |
| `send <worker> <text> --brief <id> [--review [--commit <rev>]]` | main | change | a task or `--question` that binds the brief to the recipient's run, as `work` or with `--review` as `review`, and carries the brief; with `--commit` the reviewer gets a checkout of its own at that commit; refused beside `--notify`; `--review` without `--brief`, and `--commit` without `--review`, are refused |

- **Who.** "main" is the verified run of the room's main (`ownRunIn`, its record's role `main`, as
  `compact` checks); "a bound run" is a run whose name and epoch `brief.json` lists. Anyone else is
  refused, exit 1, naming who may; a tick by a run bound as `review` is refused, exit 1, saying it
  was given the brief to review and that ticking is the worker's. Main is bound to no brief and
  does not comment. A shell outside a session reads no brief: the person reads them
  in the shell, which asks as main's run.
- **Binding.** `send --brief` runs every check of `send` first, then binds under the brief lock —
  adding the recipient's run, or keeping it; a run given the brief before with the other kind has
  its binding replaced, kind and `since` — then writes the letter. `--review` is a switch beside
  `--brief`, not a value of it: `--brief` keeps one meaning, the id, and `--review` says how it is
  given, as `--undo` says how `tick` acts. Binding is by run, so a later
  worker of the same name gets no right to an earlier brief. It ends when the brief closes or the
  run ends; an ended run stays listed, marked ended. There is no bind or unbind of its own: a
  binding without a task would give a worker a brief it was never told of.
- **What the worker gets.** `rewake inbox` prints a letter that carries a brief and then, read at
  that moment, the brief: a header `brief 3 · <title> · edition 2 · 2 of 5 done · 4 comments`, a
  line naming `rewake brief tick`, `comment` and `show` and their tools, and the text with each
  item's number (`- [ ] #3 run the tests`); under `--json` the message gains a `brief` object. To a
  reviewer the line says it was given the brief to review, names `comment` and `show` and their
  tools, and that ticking is the worker's; `brief show` says the same to it.
  `inbox --owed` prints it again with the task, the current text, so a compacted worker re-reads
  it. A brief closed meanwhile prints as closed; one that cannot be read prints the letter whole
  and says the brief could not be read and how to read it again (E5, E6). A letter an earlier run
  of the name left unread prints its brief only to a run the brief was given to, as `show` would:
  to any other the letter is shown and the brief is one line, that it was given to an earlier run
  and main can give it again with `--brief` (`"earlierRun": true` under `--json`). A notice never
  carries it.
- **An edit tells nobody.** "Only then" is the owner's: main that wants a worker to see a new
  text sends a letter, with `--brief` again.
- `rewake edit` of a task with a brief keeps the brief in its replacement; `withdraw` leaves the
  binding.

### The JSON the shell reads

A field is added, never renamed or retyped (held by
`internal/cli/brief_test.go`); times are RFC 3339:

```
brief list [--closed] --json  {room, briefs: [summary], unreadable: [{path, why}]}
                              open briefs, then closed ones, each newest first
summary                       {id, title, closed, created, closedAt?, edition, revision,
                               done, total, commentCount,
                               bound: [{name, epoch, since, run: running|ended|unknown,
                                        kind: work|review, checkout?, commit?}],
                               skills?: [name]}
brief show <id> --json        summary and {text, items: [{n, text, done}],
                               comments: [{n, from, at, edition, text, capacity?}],
                               skillFiles?: [{name, description, path}], skillsUnreadable?}
                              n is the comment's place among the brief's comments, 1, 2, 3 …,
                              derived, not the log's number
                              skills, the names the text's line gives, closed or open;
                              skillFiles, the copies a worker reads, none once closed
```

A comment is markdown and its newlines are kept from the tool's argument through the log to
`brief show`, which prints its lines indented under the header line, a blank one blank, `--json`,
the room view and the shell's pane, which renders it as markdown. Decided by the owner, October 27,
2026, a copied comment having read as one line («нормальный md давать в комментарий? а не кашу»):
the workers' briefing and the `brief_comment` tool's description ask for a one-line point first,
then short bullets, one idea per line, no wall of text.
`capacity` is absent on a comment written before capacities. Every place a comment is shown names
its author and capacity — `review-pi · review`, `write-claude · work`, the name alone for an old
comment: `brief show`'s lines (and so `brief_read`), `--json`, the room view's `BriefComment` and
the shell's pane; `brief show` marks a reviewer in its `bound:` line, `api (review)`.
`text` is the current text with the ticks applied and no numbers, what `<markdown>` renders; a
binding has ended when the brief is closed or its `run` is `ended`. Main may show a closed
brief; `--markdown` changes nothing under `--json`. The changes answer: `new` and `edit` a
summary, `tick` `{id, edition, item}`, `comment` `{id, n}`, `close` `{id, already,
checkoutsKept?: {path: why}, checkoutsError?}`.

## A reviewer's checkout

Decided by the owner, October 27, 2026, from the retro: a reviewer three times reviewed a
writer's checkout that had moved on to its next brief, and main made
`git worktree add --detach /tmp/rewake-review-<name> <commit>` by hand. So
`send <reviewer> "..." --brief <id> --review --commit <rev>` gives the reviewer a checkout of
its own, detached at that commit:

- **Which commit: main's word.** `--commit` takes a commit, branch or tag, resolved in the
  repository main's session runs in (its record's working directory), where every writer's
  worktree branch is visible. Rejected: the bound writer's HEAD at send time — main often gives
  the writer its next brief before the review goes out, which is the very case that failed, and
  main has the commit from the writer's report anyway. Without `--commit`, `--review` gives no
  checkout, as before: a review of a document needs none.
- **The checkout** is a `git worktree add --detach` under the worktree root, beside the launch
  checkouts, named `review-<id>-<reviewer>-<commit's first 7>`, with a record like theirs
  ([worktree.md](worktree.md#a-reviewers-checkout)): no branch, no session, and a `review`
  field naming the brief and the reviewer's run. The same commit given again finds the
  checkout it has; a new commit makes another, and the one the binding named before goes when
  nothing in it would be lost.
- **Made before the binding.** The brief is read first, so a missing or closed one makes
  nothing; then the checkout; then the binding records its path and commit; then the letter.
  One module, `internal/host/reviewcheckout`, owns that order (`Give`): a binding that fails
  takes the checkout just made back. A send that fails after the binding leaves it, `rewake
  worktree rm` takes it.
- **The letter.** The checkout is not copied into the letter: the reviewer's inbox prints the
  brief with a line naming the path and commit under the line saying how to act on it, and
  `brief show` (and `brief_read`) print the same line to that reviewer, read from its binding.
  `--owed` prints it again after a compaction.
- **Removed when the review ends**: at `brief close`, and when the reviewer's space closes —
  its run ended — each through the release a worker's own checkout gets at its close
  ([worktree.md](worktree.md#a-reviewers-checkout)). One holding changes stays, named in the
  close's answer (`checkoutsKept`) or the space's end, for `rewake worktree rm`. A release that
  could not look at them (`checkoutsError`, a line of the space's end, a line on stderr of the
  send that replaced one) says so and fails nothing: the close, the end and the send are done.
  The module's `ReleaseBrief` and `ReleaseRun` hold the one rule of whose a checkout is.

## Skills

Decided by the owner, October 28, 2026: a worker gets skills from main, per brief, not from a
skill folder of its own. A brief names its skills; rewake copies them from where main's harness
reads skills into the brief's folder; the worker given the brief reads them by the paths its
letter gives; closing the brief takes the copies away. Nothing is loaded into a running harness
(«если с динамикой не очень, то лучше не делать»). The accepted limit: a skill given so does not
trigger by its description, it is read because the brief names it, and its scripts are read, not
run. Decided by the owner the same day: skills reach a worker only so. rewake makes no skills folder
for the workers, neither in Claude Code's service folder nor in pi's agent directory
([worker-profile.md](worker-profile.md#skills-plugins-and-mcp-servers)); one a person made stays
theirs, never written.

- **The line.** One line of the text, outside a fenced block: `Skills: mods, mp-tdd`. It is
  markdown as written, so the shell renders it with no change, and it survives `brief edit`,
  which starts from `show --markdown`. Refused as a bad text, exit 2: a second such line, a name
  that is not `[a-z0-9][a-z0-9._-]{0,63}`, a name with `:` — a plugin's skill, which a brief does
  not give; the hint says to link its folder into main's user skill folder — a name twice, more
  than 8 (`taskbrief.DeclaredSkills`).
- **The catalogue is main's harness's.** An adapter that reads skills offers
  `adapter.SkillCatalog`, the folders in the order its harness looks, the first
  `<folder>/<name>/SKILL.md` giving the skill. It is asked with main's environment, home and
  directory: only main makes and edits a brief, so the caller's are main's. Claude Code: the user
  folder, `$CLAUDE_CONFIG_DIR/skills` or `~/.claude/skills`, then `.claude/skills` from main's
  directory up to the repository's root, never the home directory
  ([research-launch.md](research-launch.md#where-claude-code-reads-skills)). pi: `.pi/skills`,
  `.agents/skills` up to the repository's root, the agent directory's `skills`, `~/.agents/skills`
  ([research-pi.md](research-pi.md#skills)). The fixture, for the workflow suite:
  `~/.fixture/skills`. Not offered: managed skills, plugins' skills, pi's names from the front
  matter and its nested skills, `--add-dir` folders. A harness without the interface gives none.
- **The never-list.** `[skills] never = ["keychain"]` in the user's
  `~/.config/rewake/aliases.toml` and in the project's `.rewake/aliases.toml`, the two joined
  ([user-guide.md](user-guide.md#a-projects-rewake)). A skill missing or barred refuses
  `brief new` and `edit`, exit 1, naming the folders looked in or the file that bars it; so does
  an alias file that does not read, since what it bars is then unknown. Nothing is written.
- **Linked skills come along** (the owner, October 28, 2026). A skill may refer to a sibling skill —
  `mp-tdd/SKILL.md` to `../mp-codebase-design/SKILL.md` — and the copy keeps the catalogue's layout,
  `skills/<e>/<name>/`, so such a path leads somewhere only if the sibling is copied too. The
  rule (`taskbrief.linksOut`): in every `.md` file of a skill being copied, each run of the
  characters `A-Za-z0-9._~/-` that holds `../` and does not start with `/` — a markdown link's
  target, a path in backticks or bare in the text alike — less one trailing full stop, is joined to
  the folder of the file it is written in; when the result leaves the skill's folder by exactly one
  level, `../<name>` or `../<name>/…`, and `<name>` is a skill's name, `<name>` is looked up in
  main's catalogue, as a named skill is. Examples: `[design](../mp-codebase-design/SKILL.md)` in
  `mp-tdd/SKILL.md` brings `mp-codebase-design`; ``see `../../mp-grilling/SKILL.md` `` in
  `mp-tdd/references/flow.md` resolves from `references/` to `../mp-grilling/SKILL.md` and brings
  `mp-grilling`. Found, it is copied whole and scanned in its turn, each skill once, so a cycle
  ends; a name the catalogue has no skill of is passed over (`../cypress/support/e2e.js` stays a
  path that leads nowhere); a name on `[skills] never` is not copied and the letter says so beside
  the skill that links it. A name alone in prose is not followed — `mods` is a word — nor an
  absolute path or a URL. **Bound: 16 skills given**, the named and the linked together, beside the
  bound per skill; past it `new` and `edit` refuse, exit 1, naming the chain of links that reached
  past it, `mp-a → mp-b → mp-c`. A catalogue that fails for another reason than a missing or a barred
  name — an alias file that does not read — refuses as for a named skill.
- **One snapshot per edition.** The skills of edition `e` are `skills/<e>/<name>/`, beside
  `skills/<e>/skills.json`, `[{name, description, from, broughtBy?, barred?}]`: the description
  from `SKILL.md`'s front matter, cut to 1,024 code points, the folder it was copied from, the
  skill whose link brought a linked one, and the linked names `[skills] never` bars, not given. The current edition — the
  one the log's last edit commits — selects its snapshot, so a text and its skills change
  together. Every edit copies the named skills again, which is how a changed skill reaches the
  brief; a text without the line has none. `brief.json` does not change; a brief made before has
  no `skills/` and gives none.
- **The copy.** Each named skill's folder is copied whole through its link, as an installer
  links it. A file is written new, 0600, flushed, with no executable bit; a link inside the skill
  to a regular file is copied as that file; any other link — to a folder, broken — and anything
  neither file nor folder refuses the brief, named. **Bound: 16 MiB and 4,096 entries per
  skill**, about three times the largest seen (5 MB in 1,695 files); checked when the skill is
  measured and again while it is copied.
- **Preflight, then the order.** Before any effect the line is parsed, every skill found, barred
  or not, walked against the bound and its entries and scanned for its links, and `skills.json` encoded against the 64 KiB
  record bound; a refusal writes nothing. `new` writes `skills/1/` inside its `.tmp-*` build, so
  the brief is whole or absent. `edit` to edition `e`, under the lock: (a) drop a leftover
  `skills/<n>` for `n ≥ e` and any `skills/.tmp-*`, never committed and so read by nobody; (b)
  copy into `skills/.tmp-<random>/` and rename it to `skills/<e>/`; (c) write `text/<e>.md`; (d)
  publish the edit's entry, the commit point; (e) drop `skills/<n>` for `n < e`. A failure before
  (d) leaves edition `e-1` with its snapshot whole, though the skill changed since; one in (e)
  leaves a snapshot nobody selects, dropped at the next edit or the close. A brief kept outside
  the room gets the same, under `state.WithOutside`.
- **The letter.** `inbox`, `--owed` and `brief show` to any bound run, the reviewer's included,
  print after the header: `Rewake: skills main gave with this brief: read each SKILL.md before you
  start, and the files beside it as it says; they are copies, read here, not run.`, then
  `- <name> — <description> — <path of its SKILL.md>` per skill, the current edition's, the named
  first in the line's order, then the linked as they were found, each ending `— linked from
  <skill>`; under a skill that links a barred one, `  - its link to <name> leads nowhere: [skills]
  never bars <name> from briefs, so it is not given`. A path read
  before an edit is gone after it: main sends `--brief` again, as for any edit. A snapshot that
  does not read prints one line saying so and asking main to edit the brief again. Under `--json`
  the letter's `brief` object and `show` carry `skillFiles`, each with `broughtBy` and `barred`
  where they apply, and `skillsUnreadable`; the summary's
  `skills` are the names, taken from the text, so a closed brief and the room view still name
  them.
- **Reading it.** A Claude Code worker's run `--settings` allows `Read(//<room>/briefs/**)`
  beside the rule for main's checkout ([worker-profile.md](worker-profile.md#mains-checkout-is-readable)):
  the room's folder lies outside its working directories, where auto mode's one-time prompt would
  stop it. A project's `[briefs] dir` under main's checkout is covered by that checkout's rule.
  pi asks no permission to read.
- **Closing.** `brief close` removes `skills/` under the lock before it marks or moves the brief;
  a close cut short is finished by its retry. A moved brief has nothing to remove.

## The tools

The same three in Claude Code's mod and rewake's pi extension, offered to every run whose
`REWAKE_ROLE` is not `main` — a choice of what is shown, as for the worktree tools; main uses the
commands. They are thin front ends of the CLI, as `worktree_status` is
([worktree-tools.md](worktree-tools.md)): the command by argv through `REWAKE_EXECUTABLE`, no
shell, the run's `REWAKE_DIR`, `REWAKE_ROOM`, `REWAKE_SESSION`, `REWAKE_EPOCH` set over the
harness's environment, bound 60 seconds; a nonzero exit, a cut answer or a command that did not run
is an error result.

| Tool (Claude Code / pi) | Input | Runs | Answers |
|---|---|---|---|
| `mcp__rewake__brief_read` / `brief_read` | `id` | `brief show <id>` | the line form, its header naming the edition a tick takes: markdown reads as written, not escaped in JSON |
| `mcp__rewake__brief_tick` / `brief_tick` | `id`, `item`, `edition`, `undo?` | `brief tick <id> <item> --edition=<edition> [--undo]` | the item's number, text and state; a reviewer's refusal as an error |
| `mcp__rewake__brief_comment` / `brief_comment` | `id`, `text` | `brief comment <id> -- <text>` | the comment's number among the comments |

The tool refuses before any process starts an `id`, `item` or `edition` that is not a positive
integer and an empty `text`; the `--` keeps a comment that begins with a dash from being read as a
flag. The right is the CLI's check, so the tool and a command the model types from its shell answer
alike.

- **Claude Code**: registered at `session.start` beside the worktree tools, served from a
  `tool.call` hook with `$.process.run` and a `.catch` that turns a failed hook into an error
  result, through the worktree tools' `serveCommand`; the module `hooks/briefs.ts`, named in
  `mod.go`'s `embed` line. Built on the mod API of 2.1.310 (the `mods` skill).
- **pi**: `pi.registerTool` and `pi.exec`, `killed` read first, as in `worktree.ts`; the module
  `briefs.ts`, in `run.go`'s `embed` line and module list. pi names a tool as registered, so its
  three are `brief_read`, `brief_tick` and `brief_comment`, as its `worktree_status` is.
- **No protocol of their own**: the mod↔host schema and pi's wire know no brief. Rejected: `brief/*`
  endpoints on the mod socket with the peer check — a second path beside the CLI, and pi would need
  wire operations of its own for the same thing.

## Tests

By `AGENTS.md`: the item scan, the title and tick resolution (`taskbrief/items_test.go`, pure
logic); the records' format and the rights (`taskbrief/store_test.go`), a review binding and a
record from before kinds (`taskbrief/review_test.go`); a project's folder, the wiped room, a moved brief, a shared folder and the admitted folders (`taskbrief/outside_test.go`, `cli/brief_folder_test.go`, `state/outside_test.go`); the `--json` the shell
reads, the checks before any effect and the rights of the commands (`cli/brief_test.go`); the
binding of `send --brief` and what `inbox` and `--owed` print (`cli/brief_send_test.go`), and
`--review` from the send to the show (`cli/brief_review_test.go`), a reviewer's checkout from
the send to the close (`cli/brief_review_checkout_test.go`, `worktree/review_test.go`); a
brief's skills — the line, the copy and its bound, refusals that write nothing, an edit cut short
before its commit, the close (`taskbrief/skills_test.go`), the link rule, a chain, a cycle, a
barred link, one outside the catalogue and the bound of skills given
(`taskbrief/skills_plan_test.go`), the catalogues' order
(`adapter/skills_test.go`, each adapter's `skills_test.go`), the never-list
(`launch/alias_briefs_test.go`), the commands and their `--json` (`cli/brief_skills_test.go`),
the read rule (`claude/run_settings_test.go`); the
tools' argv and refused input in the mod's `tests/briefs.test.ts` and pi's `briefs.test.ts`; and
one workflow scenario, `brief`: main makes a brief, sends it, the fixture worker reads it with
its task, ticks and comments, a reviewer given it with `--review` is refused a tick and comments,
and main's `show` sees the tick and both comments with their capacities; and one, `brief-skill`:
main makes a brief naming a skill of its fixture, the worker's letter gives the copy's path, the
file there holds the skill, a skill it links comes next marked as brought by it, and the close
takes them
([testing-cases.md](testing-cases.md#launch-reports-and-what-a-main-waits-on)).

## Known limits

- The line scanner and `marked` may disagree on an odd checkbox (inside HTML, a lazy continuation):
  one may not be tickable, or drawn as text.
- The right to a brief is the run's identity from its environment, as a letter's sender is: a guard
  against mistakes, not against a session forging another's variables.
- A send that fails after binding leaves the worker bound to a brief it never got; it may read,
  tick and comment there, as any bound run.
- Briefs in the room live in the state directory: a reboot that clears the temporary directory takes
  them, as it takes mail; a project's `[briefs] dir` keeps them. A `--dir` brief is forgotten with
  the index by such a wipe. Closed ones are not swept.
- A skill given with a brief does not trigger by its description, and its scripts are read, not
  run; a skill whose instructions say to run a script beside it asks the worker to run a copy
  with no executable bit, through its interpreter. A plugin's skill, a managed one and pi's
  nested or front-matter-named skills are not given. A linked skill is looked up by its name in
  main's catalogue, not by where the link leads: a skill of that name in an earlier folder of the
  catalogue is the one copied. A sibling mentioned by its name alone is not brought. A `--dir` folder outside main's checkout is
  not in a Claude Code worker's read rule: reading its copies there asks main for permission.
