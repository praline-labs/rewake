---
description: The steps of adding a harness, a role, a message kind or a session tool, and what each must touch; open before adding one.
---

# Adding a harness, a role, a message kind or a session tool

## A harness

Meant to be one move:

1. A new package `internal/adapter/<name>` with a type implementing `adapter.Harness`: `ID`,
   `Title`, `Summary`, `Examples`, `Notes`, `SingleUseFlags`, `Version`, `EnvNotInherited`,
   `Offers`, `Serve` and `Launch`. `SingleUseFlags` names the flags that harness takes at most once,
   each with all its spellings and whether it carries a value. Returning nothing is not a
   neutral answer — it says every flag may be repeated, and a launch alias plus a typed flag
   then reach a harness that may refuse to parse them. A test over the catalogue requires the
   list to be non-empty. `Offers` says what the adapter offers beyond Launch, and the plan
   `Serve` returns carries what one run serves — its service, wake, turn boundary, telemetry —
   as the interfaces of `internal/adapter` ([rules/adapter-api.md](rules/adapter-api.md)).
   `Launch` composes it after the steps that prepare a start; an adoption calls `Serve` alone,
   so it checks, runs and writes nothing, and a catalogue test holds it so. An
   offer with nothing in the plan behind it is a capability the CLI admits and the run cannot
   serve. The offer answers `Conversation` either way — the flag that names a new conversation
   with rewake's id, or why there is none — and offers `Spaces` only where the run proves them
   through its live set; the catalogue tests hold both ([rules/adapter-api.md](rules/adapter-api.md)).
2. One entry in `New` of `internal/adapter/catalog/catalog.go`.
3. Its private subtree's rule in the `depguard` settings of `.golangci.yml`: copy one
   adapter's rule (`adapter-claude`) for the new package and exclude the subtree from
   `adapter-new`.
4. If it ships an in-agent half, an entry in `agents` of `internal/adapter/purity_test.go` and a
   pin of its wire as the other adapters' `wire_words_test.go` do: the half runs natively and
   carries, and the wrapper decides ([rules/adapter-api.md](rules/adapter-api.md#what-an-agents-half-keeps)).

Everything else follows: the launch command, the guide entry, its own `--help` page, the step
in FLOW, the `harnesses` field of the machine form. A harness cannot be half-registered —
either it is in the catalogue and fully described, or it does not exist.

## A role or a message kind

**A role** is a value in `internal/core/role/role.go` plus its line in the list; its `Play`
text feeds both the briefing and the guide, and a copy of another role's fails a test.
**A message kind** is the constant in `internal/core/mail/inbox.go`, a `messageKind` in
`internal/cli/send_<kind>.go` and its line in `sendKinds`; a kind the other places do not know
is treated as a task (`mail.Owed`), so before adding one meant to behave differently, grep for
`mail.Task`, `mail.Note` and their neighbours and decide each place.

## A session tool

A tool rewake offers a session in every harness — the brief and worktree tools — is a front end
of one `rewake …` command, defined once in Go, `internal/core/sessiontools`: its name, label,
description, input schema, the check of its input into the command's arguments or a refusal, its
bound and the wording of its raw end. The agents hold no copy: they register the table the
wrapper sends and run what it plans ([worktree-tools.md](worktree-tools.md#the-worktree_status-tool)).
The steps:

1. Its `Tool` in `table.go`, its arguments' check in `args.go`, and the roles that see it in
   `Offered`, with their tests in `table_test.go` and `call_test.go`.
2. Nothing in the adapters: the table reaches a running session with the wrapper's restart. An
   adapter's own tests hold its bridge, not each tool.

A tool only one harness can serve — `worktree_enter`, built on Claude Code's own `EnterWorktree` —
is offered only to that harness's run, and its native step stays in that adapter: the mod's move
and its `tool/native` answer.
