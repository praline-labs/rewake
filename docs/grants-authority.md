# Who can grant a directory

How a directory grant ([grants.md](grants.md)) is made one a worker cannot forge: main's
wrapper registers and confirms every grant, and nothing on disk counts as proof. The
same scheme confirms `--grant-git` ([git-grants.md](git-grants.md)).

## The scheme

Everything a message carries is written by whoever wrote the file, and the state
directory — registry, mailboxes, journals — is writable by a Codex worker in
`workspace-write`, since it lies in `/tmp`. So nothing on disk says who sent a grant.
What does is main's wrapper, a process outside every sandbox, asked over an abstract
unix socket named from the room and main's run (`@rewake/grant/<hash>`, no file to
replace or leave behind). The wrapper binds it before it writes its record, so from the
moment the address can be worked out it is taken; a `rewake send` registers only with a
listener that runs above it, so a listener there first would not take a grant and have
send report it as registered.

1. **At send.** `rewake send` with a grant registers it with its own session's wrapper
   before the letter is written. `rewake edit` asks the wrapper, from the same callers,
   to carry over to the replacement what it holds for the task replaced; the letter's
   own grant fields are no source, since the worker can rewrite its unread mail. Nothing
   held carries nothing, and a letter that names a grant anyway is refused. The wrapper takes it only from a
   process below itself: the caller's uid and pid from `SO_PEERCRED`, then its chain of
   parents up to the wrapper, each no younger than its child, so a pid taken again on the
   way ends the walk; and it must share the wrapper's mount, user and PID namespaces. A
   worker's process runs below its own wrapper and not main's, unless main started it;
   one that left its tree through `setsid` and a double fork runs below neither. The
   wrapper holds the grant in memory — message, recipient, the recipient's run,
   directories, broad ones, Git — while its task is open: until the task is read and
   reported on or taken back, and for the task's time to live and five minutes more when
   its letter cannot be found; at most 256 at once, and past that it refuses a new one
   rather than forget an old one. A resumed run of the recipient asks it again
   ([grants-resume.md](grants-resume.md)).
2. **At delivery.** The recipient's wrapper asks the address the letter's run leads
   to, and takes the answer only from the process that run names: the
   listener's pid from `SO_PEERCRED` must be that run's pid, alive with that start time
   before and after the answer, and in the same mount, user and PID namespaces as the
   asking wrapper. The grant confirmed must be the one the letter carries, for this
   recipient and this run. A letter main never registered, one naming a run it is not,
   one answered from another process fail, and main is told. This question names no
   conversation: a task that waits — for an idle reader, for a turn to end — may go into
   another one than the session had when it was first checked. Once the letter is
   pinned, under the mailbox lock as it becomes readable, the recipient's wrapper asks
   again naming the conversation it was pinned to. Main's wrapper keeps that name beside
   the grant, taken only from the wrapper of the run the grant went to, as the one
   conversation a resume may take the grant into ([grants-resume.md](grants-resume.md)).
   A delivery refused after the pin — the conversation compacting — is pinned again when
   it is tried again, maybe into another conversation, and that wrapper's latest name
   replaces the earlier one, as its own record of the letter does (September 28, 2026;
   until then main kept the first, and a resume of the conversation the task went into
   was refused). A main that does not answer that second
   question leaves the grant with no conversation: it holds for the run, and a resume
   does not restore it.
3. **When main cannot answer.** While main's wrapper is alive and does not answer, the
   task stays pending and is asked again until it expires. Once that wrapper has ended,
   nobody can confirm the grant, now or later: the task fails, naming the main that sent
   it, and has to be sent again from the current one.

What this holds against rests on the peer checks, not on the address being hidden. A
Codex sandbox with its network on shares the host's network namespace and sees every
`@rewake` name ([research-codex.md](research-codex.md#the-sandboxs-namespaces)); only
with the network off is it cut off from them. So the wrapper checks who is at the other
end both ways — the pid, the start time and the mount, user and PID namespaces — and the
namespace check at registration is what keeps a sandbox main started from granting.

## What it holds against, and what not

- **A worker inside the Codex sandbox.** Codex runs a command in a mount, user and PID
  namespace of its own, in `workspace-write` and `read-only` alike, and `setsid`, a
  double fork or `nohup` do not change them
  ([research-codex.md](research-codex.md#the-sandboxs-namespaces)). Its process is below
  no wrapper, so it registers nothing; a listener it starts in a run it names itself is
  in other namespaces, so nothing it confirms counts. This is what the scheme is for.
- **A sandbox main started itself.** A Codex that main runs from its own shell — for a
  live probe, as this repository's instructions do — runs below main's wrapper, and so
  does every command of its sandbox; with network allowed, one could reach main's
  address, register a grant for any recipient and put the letter in a mailbox itself.
  Its commands are in namespaces of their own, so the wrapper refuses the registration
  as it refuses a confirmation from them.
- **A worker's code main runs.** `go test` on a worker's branch, run by main, runs below
  main's wrapper in its namespaces and could register a grant. It is no new hole: code
  main runs outside a sandbox can do whatever main can, a grant included.
- **Codex's legacy Landlock backend** runs sandboxed commands in the app-server's own
  namespaces. It is off by default and never chosen as a fallback, but a session whose
  launch arguments or `config.toml` mention `use_legacy_landlock` takes no grant. The
  refusal is the recipient's: other sessions of the room take grants as before.
- **A worker with no sandbox** — Claude Code, whose approved shell commands run as the
  user — has no boundary at all. It can start a listener in the wrappers' own namespaces,
  write a registry record naming that listener's run as a main, and send a letter
  pointing at it; nothing in rewake tells that listener from a wrapper. It is not a new
  hole: such a worker writes wherever the user can anyway, and a grant on Claude Code
  removes prompts rather than draws a line.
- **A Codex main** cannot grant: its own sandbox refuses `connect()` on a unix socket
  ([research-permissions.md](research-permissions.md)), so its `rewake send` never
  reaches its wrapper. `--grant-dir` and `--grant-git` from a Codex main are refused with
  exit 1; before this change `--grant-git` from one was accepted
  ([git-grants.md](git-grants.md)).
- **A Claude Code main** registers through its shell, which shares the wrapper's network
  namespace today. A Claude Code sandbox with network isolation turned on would cut the
  shell off from the abstract socket as well, and the send would fail with exit 1 naming
  the wrapper it could not reach; not observed, since the sandbox is off here.
