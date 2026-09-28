# The Codex grant path by conversation

September 28, 2026. review-codex's reconnaissance of the Codex grant path found three
defects that had not been fixed; main set them as one package the same day.

## What was found

- **A settled grant was taken back from the conversation selected, not its own.** The
  adapter listed every settled entry of the run's journal, whatever conversation it was
  granted in, and took its path out of the roots of the conversation the notice went
  into. After the person switched the terminal from A to B, a grant made in A and
  reported on was recorded dropped when B did not have the directory, or took out B's
  own root of the same path; A kept the root, and nothing would take it back there, since
  its entry was already ended.
- **A refused hint after a resume took out a confirmed one's root.** Each hint took its
  paths out before its own answer was applied: of two copies naming one directory — a
  task closed since and a later one still open — the open one was confirmed and added
  back, and when the closed one came second it was refused and took the directory out
  again. The open grant stayed journaled as restored with no root, depending on the
  order the copies were read in.
- **Slow confirmations failed the notice for good.** The mains were asked one after
  another while the notice held its reservation, two seconds each at most, and the
  reservation lasts three: two slow answers outlived it, and the notice failed as
  "not retried automatically", though nothing had been sent and the restore promised to
  ask again at the next notice.

## What was done

- A settled grant is taken back only at a notice into the conversation its entry names,
  and a root counts as rewake's only there: in another conversation the same directory is
  the person's, covers a grant inside it and is neither journaled nor taken back
  ([grants.md](../grants.md#taking-a-grant-back)).
- The resume first collects every answer, then takes out every root a settled hint
  names, confirmed or refused, and only then adds back the confirmed grants; a refused
  hint's note names only what stayed out
  ([grants-resume.md](../grants-resume.md#codex)).
- The mains are asked all at once in the background, and the notice holds no
  reservation meanwhile: it stays pending, with a detail saying why, until they answered or
  ran out of time, and the answers are used for thirty seconds at most. The gateway says
  when a delivery was refused before anything was sent (`gateway.ErrNotSent`), and such a
  notice stays pending instead of failing.
- The send note on a Codex grant's end and `docs/flow.md` say the conversation.
- Tests: a settled grant untouched by notices into another conversation and taken back in
  its own; a live grant in another conversation that does not make this one's root rewake's; a refused
  and a confirmed hint naming one directory, in both orders; two mains answering slowly;
  a notice whose reservation lapsed before sending. Each was run against the code before
  the fix and failed there — the hint pair in the order that showed the defect — and the
  thread scope of a root's owners and the pending not-sent notice were each taken out on
  their own and failed.

## Review

review-codex accepted the change after four fixes; the first three, each confirmed in the code:

- **The shared root was kept only within one notice.** A grant restored at the first
  notice was left out of the hints at the next one, as journaled, and a hint for the
  same directory refused there — its main had not answered before and had ended since —
  took the root from under it. A root a live entry of the conversation holds is now left
  in place.
- **The answers were looked up for the conversation selected before the reservation.**
  With none selected yet the check was skipped, and Reserve then waited for the resume
  and took a conversation whose grants had no answers: the notice went out without them.
  The same happened on a switch between the two calls. The check is now made against the
  reservation's own conversation, and the reservation is let go while the mains are
  asked.
- **A reservation that lapsed before its notice was prepared failed it for good**, though
  nothing was readable or sent. The gateway says so (`gateway.ErrLapsed`), and preparing
  through the inbox or in the delivery leaves the notice pending.

A fourth finding came at the acceptance of those three, confirmed by a trace: a deadline
passing after Deliver found the reservation valid, but before the request was written,
still failed the notice for good. A request the gateway never wrote now answers
`gateway.ErrNotSent` as well, and the notice stays pending.

Tests over two notices, over a conversation selected only after the notice began to
wait, over a reservation lapsed before preparing, and over a deadline passed between
that check and the write; each fix was taken out on its own and its test failed.

## What stays open

- A main's wrapper answers one confirmation at a time, so of several hints from one
  slow main only those answered within the two seconds come back at the first notice;
  the rest follow at the next ones.
- A grant in a conversation the terminal never returns to is not taken back: its entry
  stays live until a notice goes into it, or the run ends.
- Not run against the real terminal; the cases use the fixture's threads.
