# Shift

How this project keeps its knowledge. The rules are written on the owner's word; the map below is
written by the shift mod from each document's `description:` and `##` headings.

## Where knowledge goes

The owner's boundary (`knowledge = "split"` in `.shift/settings.toml`):

- A fact about a harness or about rewake itself goes into the project: `docs/` — the research
  documents, `docs/traps.md`, the documents of how rewake works. A lesson about how we work and a
  trap of a tool go to `.shift/knowledge/`, kept by the agents outside git.
- The open work and its order — `docs/roadmap/README.md`; a landed step leaves it, its commit's
  subject is its record, and what it taught goes into the living documents. How the project is built
  and its conventions — `AGENTS.md`, which also says how the documents are kept true.
- The project's own words — `GLOSSARY.md`; the owner's decisions in force — `.shift/OWNER.md`;
  main's tasks in progress and their trackers — `.shift/TASKS.md` and its tasks; the open work
  of the project and its order stay in `docs/roadmap/README.md`.
- Not a harness's memory: only one harness reads it, while any agent opening the project reads
  these files. No second list of the order of work beside the roadmap: it drifts.

<!-- shift:docs — written by the mod from each document's description and ## headings; edit those, not this -->

## Documents

Open a document for numbers, commands and names: the map only points.

### docs/

- `docs/briefs.md` — Briefs — a markdown brief per task that main writes, bound to workers by send --brief, ticked and commented by them; the records under briefs/, the brief commands, the JSON the shell reads, and the worker tools of both harnesses.
  sections: The records · The commands · The tools · Tests · Known limits
- `docs/claude-turns.md` — A Claude Code session's turns through the mod — the outbox and end ids, the hello and its loads, the heartbeat and the live set, readiness and admission, what not holding an end costs.
  sections: The mod's side · The host's side · The live set · Readiness and admission · The start time · What not holding costs
- `docs/claude.md` — The Claude Code adapter as one mod — loading it, when it does not load, the mod↔host protocol, the peer check, tools, telemetry and the cache's life, control.
  sections: What the adapter offers · Loading the mod for one run · When the mod does not load · The protocol · The tools · The turn boundary · Telemetry · The cache ttl · Control · What the mod never does
- `docs/code.md` — Where each part of rewake lives in the tree, package by package; the adapter interface is in rules/adapter-api.md.
- `docs/delivery-adapters.md` — How each harness carries a notice: Claude Code's socket line, inbound gate and late receipts; pi's extension; the fixture; and where each adapter's turn ends come from.
  sections: Claude Code · pi · The fixture
- `docs/delivery-owed.md` — Who owes whom a report — rewake inbox --owed for what a session has read and not reported on, rewake inbox --awaited for what a run sent and still waits on, and when a wait counts as answered or lost.
  sections: Reading again what is owed (`rewake inbox --owed`) · What others owe you (`rewake inbox --awaited`)
- `docs/delivery-sent.md` — Acting on a message already sent — rewake withdraw, rewake edit and rewake send --to — before and after the reader saw it, with the recall note, tombstones, addenda and what is final.
  sections: Who, and by which id · Withdraw (`rewake withdraw <id>`) · Edit (`rewake edit <id> <text>`) · Addendum (`rewake send <name> <text> --to <id>`)
- `docs/delivery-turn-end.md` — The end of a turn: its steps under the sender's lock, its scope and the read clock, pending marks, the interim record, a held end, and what a retry finds.
  sections: The end of a turn · The operation · The read clock · Pending marks · The interim record · What a retry finds · Reports after a thread change · Known limits
- `docs/delivery.md` — How mail reaches a session: sending, the kinds of message, the mailbox lock, the wrapper's servicing, the notice and its preview, the coalescing window, and reading.
  sections: Delivery · The notice · Reading (`rewake inbox`)
- `docs/design.md` — The system as built — scope, processes, the state directory and the session record, rooms, the environment a run gets, the command-line contract, distribution, and the decisions in force.
  sections: Goal · Scope · Design · Launch and signals · Delivery · A session without a wrapper · Interface · Code · Testing · Distribution · Decisions in force
- `docs/flow-endings.md` — The other ends of a message's path — a blocking question, a notify, a session's end — one exchange traced through its files, and where the flow can stall and what the sender sees.
  sections: Act 6. A question, when the sender wants to block · Act 7. A notify, when nothing is owed · Act 8. The session ends · One exchange, as files · Where the flow can stall, and what it says
- `docs/flow.md` — The whole path of one message, from a launch through send, notice and read to the report, with every process, file and status on the way, and how main starts a worker in a space.
  sections: Cast · Act 1. A session starts · Act 2. A task is sent · Act 3. The wrapper announces it · Act 4. The agent reads · Act 5. The turn ends and the report goes back · A space: main starts a worker
- `docs/install.md` — Installing rewake with npm or from source, updating it, a second build beside an installed one, testing how a release installs, upgrading from 1.x, and releasing to npm.
  sections: Installing with npm · Building from source · Testing how a release installs · Upgrading from 1.x · Releasing to npm
- `docs/launch-defaults.md` — Where a launch's model, effort and arguments come from: worker aliases, environment variables, the settings files and their precedence.
  sections: Launch defaults from the environment · The worker catalogue
- `docs/launch.md` — What a launch does on each harness — the wrapper, the arguments and environment it adds, the workers' profile and service folder, pi's bundled copy, starting the shell, and signals.
  sections: Launching a harness · Signals, and what the wrapper does not do
- `docs/legacy.md` — The legacy mark on code kept only for an older harness or for records an older rewake wrote — its form, how to find the marks, when one is due and how it is cleared; none are open now.
  sections: The mark · Finding them · When a mark is due · Clearing a mark · Oldest supported versions · Current marks
- `docs/mailbox-records.md` — The records a mailbox keeps on disk under inbox/<name>/, by path, with what each is read as — a contract between the processes that share a mailbox.
  sections: The list
- `docs/permission-requests.md` — A worker's permission requests and questions carried to main as a notify and answered with rewake answer: what is carried, the path through the mod, what main sees.
  sections: What is carried · The path · What main sees · A dialog nobody is asked about · The command · Known limits
- `docs/prior-art.md` — Other tools that pass messages between agent sessions, and what rewake takes from each or avoids.
  sections: Prior art
- `docs/project.md` — What rewake is and why: its essence, its philosophy and what the tool does; changed only by the owner's decision.
  sections: The essence · The philosophy · What the tool does · What to read next
- `docs/README.md` — The reading order, the conventions every document keeps, and where each kind of document goes.
  sections: The reading order · Where each kind of document goes · Names in records and addresses
- `docs/remote-control.md` — Main steering a running worker: rewake compact and interrupt, their checks and outcomes, the control directory, how each harness serves it, and the compaction letter.
  sections: The commands · The control directory · The served side · The letter · Known limits · Owner decisions
- `docs/research-claude-mod.md` — Dated records of what probe mods and rewake's mod were seen to do inside Claude Code, by version.
  sections: How the first probe ran · P4: the peer of the mod's requests · P11: a process the mod starts · P12b: a subagent of main as the worktree's keeper · P15a: the space's terminal in the keeper's own tool row · P15b: a real pseudo-terminal in the keeper's row · A worker acts on the notice's preview · S4-6: a worker's turns through the mod · The mod on Claude Code 2.1.302 · The workers' pane (October 18, 2026, 2.1.302) · Compactions counted · Effort · The end of the process, as the mod hears it · Entering a checkout mid-session (October 20, 2026, 2.1.302) · Permission requests and questions through the mod · A worker's dialogs in auto mode · The prompt cache (October 23, 2026, 2.1.302) · The model's name and thinking · The mod on Claude Code 2.1.310 (October 25, 2026)
- `docs/research-herdr.md` — herdr, the terminal manager main may run in: how it finds an agent, its reporting command and its rules, and the live check of rewake's reports.
  sections: How herdr finds an agent · The reporting command · Live check (October 27, 2026, herdr 0.9.5)
- `docs/research-launch.md` — What a harness binary answers at launch: models, efforts, repeated flags, the settings order, the workers' configuration folder.
  sections: Models, efforts and their catalogues · Which flags may be repeated · When a check through the help text can be trusted · Passing a model and an effort for one launch · How each harness is published on npm · Whether Claude Code has a client-server split to sit between · A prompt draft at launch · A worktree at launch · Hook options and settings order that rewake's launch layer relies on · What the worker profile relies on · A configuration directory of the workers · A read outside the working directories · Claude Code's cross-session inbound gate
- `docs/research-pi.md` — pi's adapter: how it launches, wakes and hears turns, its wire contract, the bundled copy, and the probes behind each fact.
  sections: Launch and extension lifetime · Wake and the end of a turn · What the mock probes observed · Telemetry and remote control · The adapter's wire contract · Live acceptance on a real model (October 18, 2026) · The bundled pi
- `docs/research-worktree.md` — Claude Code's own worktree: continuing a conversation, trust, `.worktreeinclude`, and `git worktree add` beside another.
  sections: Continuing a conversation, and trust · Names with a slash · `.worktreeinclude` · What else Claude Code's worktree does · git worktree add beside another
- `docs/research.md` — Claude Code facts a running session shows: the inbound socket and its gate, how a notice is drawn, the status line's input, interrupts.
  sections: Claude Code · Other harnesses (for later) · Other Claude Code records
- `docs/roles.md` — The roles a session runs under, what each is told at launch and owes, and how a session's name is chosen.
  sections: Roles · What a session is told · Names
- `docs/session-state.md` — What main sees of its workers: the state header, the session table, activity labels, availability, compaction and departure notices, and where telemetry comes from.
  sections: The state header · The session table · Activity · Availability notices · Compaction notices · Departure notices · Claude Code source · pi source · Persistence and freshness
- `docs/shell.md` — rewake's shell — its layout and panes, who owns each pseudo-terminal, closing, the raw stream a worker's pane is fed, the space host's emulator, keys, colours, the bar, the strip, the briefs view, and how it ships.
  sections: Who owns which pty · How a pane is fed: the raw stream · The space host's emulator · What the widget lacks and what the shell does about it · Solid: registering the widget, room for a modal · The prefix and the room · The bar · The strip · The briefs · How rewake starts the shell, and how it ships
- `docs/space.md` — A space — main starting a worker through a detached space host, the worker's launch, the space record and its states, the listing, closing, kept checkouts and what a restore would read.
  sections: Opening · The worker's launch · The stream to the shell · The record · The listing · Closing · Restore
- `docs/testing-cases.md` — What each of the 24 workflow scenarios in test/workflow claims — the sessions it starts, what the fixture plays and what it must see — and what none of them proves.
  sections: Delivery · Launch, reports and what a main waits on · A worktree · Actions on a sent message · Conversations · A stop, and steering a session · The live set · Spaces
- `docs/testing.md` — How rewake is tested: the tiers, the fault build, the fixture column, running and reading the workflow suite, the checks before a publication, and the traps it has paid for.
  sections: The tiers · Running it · Checking a new harness version before updating · Reading a result · Extending the suite · Traps this suite has already paid for
- `docs/traps.md` — What misbehaves in rewake and the harnesses it lives with, by symptom — roles and reports, tests next to a live session, arguments, delivery — each entry dated, with its cause and what to do.
  sections: Roles and reports · Tests next to a live session · Arguments and continuing a conversation · Delivery and its proofs
- `docs/turn-outcomes.md` — What each turn end reports — finished, error, stopped or pending — who it goes to, and the hold that keeps an interim end from closing a task.
  sections: Failed turns · Interim turn ends (`rewake pending`) · The confirmation · Stops
- `docs/user-guide.md` — The person's guide — the few things only the person does: install, logins, starting main, the worker templates, a project's .rewake/, the shell's keys, stopping and restarting, where things live, upgrading.
  sections: Install · Log in · Start main · Worker templates · A project's `.rewake/` · The workers' own settings · The shell · Stop and restart · Where things live · Upgrading
- `docs/worker-profile.md` — The Claude Code workers' profile: the service folder, login, onboarding and trust, rewake's defaults in settings.json and their upgrades, the run's own settings, memory.
  sections: The service folder · The login · Onboarding and trust · Skills, plugins and MCP servers · settings.json: the workers' user settings · Retired defaults · The run's --settings · Main's checkout is readable · The worker's memory · A template that enters a checkout · The person edits the file · An upgrade adds only the newer defaults · A file rewake cannot read
- `docs/worker-types.md` — Worker types: the kinds of work a worker is launched for, their briefing sections, where a type is defined, and the memory by type.
  sections: Where types come from · Turning built-in types off · Raising a worker for a type · Memory · The project's `.rewake/` · In `rewake list`
- `docs/worktree-tools.md` — A worker entering a checkout mid-session, the worktree status tool and its close forecast, and main's list and land tools.
  sections: A template's `worktree = true` · `rewake worktree status` · The `worktree_status` tool · Main's `worktree_list` and `worktree_land` · A worker's entry: `enter_worktree = true` and `worktree_enter`
- `docs/worktree.md` — Checkouts rewake makes for a launch: creation, records, land and finish, removal and what keeps one.
  sections: The launch · Launches at once · Files git ignores: `.worktreeinclude` · Landing and finishing · Listing and removing

### docs/roadmap/

- `docs/roadmap/README.md` — The open work and its order — the owner's looks and logins, and the acceptance of additive fields — and the ideas not scheduled.
  sections: Open, in order · Not scheduled

### docs/rules/

- `docs/rules/adapter-api.md` — The adapter API — capabilities offered and live, what the core does without each, the interfaces in internal/adapter, the catalogue, minimum and pinned versions, and adding a harness.
  sections: Why capabilities · The capabilities · Two levels: offered and live · The catalogue · Minimum versions
- `docs/rules/effects.md` — E1–E8, what may decide a mail operation's effect and the order of a turn end's steps, the rule a pending mark keeps and how a letter is retired; open before changing how mail is written, read or published.
  sections: The rules · A pending mark · Retiring a letter
- `docs/rules/launch.md` — L1–L3: what a launch adds to a harness, what it never changes, and that nothing it writes outlives the run.
  sections: The rules
- `docs/rules/layout.md` — The import rule between layers, and where a harness may be named; open before adding a package or an import.
  sections: The import rule · Harness names outside the adapters
- `docs/rules/README.md` — The index of the numbered rules — effects, turns, launch, space — and the unnumbered ones of layout, the adapter API and state; how a rule is written.
  sections: How a rule is written
- `docs/rules/space.md` — SP1–SP10 — what a space guarantees about its worker, its record and its closing, and the tests that hold each rule.
  sections: The rules
- `docs/rules/state.md` — The state root and its levels, the build's identity, the room's lease and its lock order, the bootstrap, views and changes through the write seam, and the state of 1.x left alone.
  sections: The state root · The build's identity · The room's lease · Views and changes · The state of 1.x
- `docs/rules/turns.md` — O1–O2, an interim wake that must not close a task early and how failed and interrupted turns report; open before changing what a turn end reports.
  sections: The rules

<!-- /shift:docs -->
