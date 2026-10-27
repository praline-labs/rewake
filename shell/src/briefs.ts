// The briefs the shell shows in the worker's place (docs/shell.md, "The
// briefs"): the JSON of rewake brief list, read as main's run once a second
// while the view shows, and of brief show while a brief is open, and the pure
// parts of drawing them.
import { separator } from "./hints.ts";
import { mainAs, readRewake } from "./room.ts";

export type Bound = { name: string; epoch?: string; since?: string; run: string };

export type Summary = {
  id: number;
  title: string;
  closed: boolean;
  edition: number;
  done: number;
  total: number;
  commentCount: number;
  bound: Bound[];
};

export type Comment = { n: number; from: string; at: string; text: string };

export type Detail = Summary & { text: string; comments: Comment[] };

// ended says whether a binding has ended: its run, or the brief itself.
export const ended = (brief: Summary, b: Bound) => brief.closed || b.run === "ended";

// ordered is the shell's order of the briefs, oldest first so a new one
// lands at the bottom (the owner, October 27, 2026): the open ones by id,
// then the closed ones by id. The CLI's own order stays newest first.
export const ordered = (briefs: Summary[]) =>
  [...briefs].sort((a, b) => Number(a.closed) - Number(b.closed) || a.id - b.id);

// cut is a text at most width cells long, a cut one ending in `…`.
export function cut(text: string, width: number): string {
  if (text.length <= width) return text;
  return width <= 0 ? "" : `${text.slice(0, width - 1)}…`;
}

// headOf is a list entry's first line, the one drawn as a button: `3 · Auth
// refactor`.
export const headOf = (brief: Summary) => `${brief.id} · ${brief.title}`;

// infoOf is its second line, drawn faint under the title: `2/5 · 4 comments ·
// write-claude, review-claude (ended)`; a closed brief says so, and its
// bindings, all ended with it, are not marked one by one.
export function infoOf(brief: Summary): string {
  const parts = [
    `${brief.done}/${brief.total}`,
    `${brief.commentCount} comment${brief.commentCount === 1 ? "" : "s"}`,
  ];
  if (brief.closed) parts.push("closed");
  const names = brief.bound.map((b) =>
    ended(brief, b) && !brief.closed ? `${b.name} (ended)` : b.name,
  );
  if (names.length > 0) parts.push(names.join(", "));
  return parts.join(separator);
}

// A navigation item over an opened brief: a brief by its id, or, without
// one, the way back to the list.
export type NavItem = { id?: number; text: string };

export const backText = " ‹ briefs ";

// navOf is the row in place of an opened brief's header: back to the list,
// then the open briefs in the list's order and the shown one where it is
// closed, each ` 3 Title `, one cell apart, in width cells. The titles are
// cut to one length, the longest that lets the row fit, whichever brief
// shows, so a change of the shown one moves nothing; a row too narrow even
// for the numbers alone is cut at the width.
export function navOf(briefs: Summary[], shown: number | undefined, width: number): NavItem[] {
  const row = briefs.filter((b) => !b.closed || b.id === shown);
  const itemOf = (b: Summary, room: number) => {
    const title = cut(b.title, room);
    return { id: b.id, text: title ? ` ${b.id} ${title} ` : ` ${b.id} ` };
  };
  const rowAt = (room: number) => row.map((b) => itemOf(b, room));
  const length = (items: NavItem[]) =>
    items.reduce((n, item) => n + item.text.length + 1, backText.length);
  let room = Math.max(0, ...row.map((b) => b.title.length));
  while (room > 0 && length(rowAt(room)) > width) room--;
  return [{ text: backText }, ...rowAt(room)];
}

// stepOf is the brief ← or → shows from the shown one in the row, or none
// past its ends.
export function stepOf(items: NavItem[], shown: number | undefined, by: number) {
  const ids = items.flatMap((item) => (item.id === undefined ? [] : [item.id]));
  return ids[ids.indexOf(shown ?? -1) + by];
}

// stripOfBriefs is the strip's parts under the briefs, the first its name,
// fitted to a width: `briefs · 3 open · 1 closed` under the list, `brief 3 ·
// 2/5 · 4 comments` under one; what does not fit is cut with `…`.
export function stripOfBriefs(
  shown: { list: Summary[] } | { id: number; detail?: Summary },
  width: number,
): string[] {
  let parts: string[];
  if ("list" in shown) {
    const closed = shown.list.filter((b) => b.closed).length;
    parts = ["briefs", `${shown.list.length - closed} open`];
    if (closed > 0) parts.push(`${closed} closed`);
  } else {
    parts = [`brief ${shown.id}`];
    const d = shown.detail?.id === shown.id ? shown.detail : undefined;
    if (d) {
      parts.push(`${d.done}/${d.total}`);
      parts.push(`${d.commentCount} comment${d.commentCount === 1 ? "" : "s"}`);
      if (d.closed) parts.push("closed");
    }
  }
  const out: string[] = [];
  let room = width;
  for (const [i, part] of parts.entries()) {
    const sep = i > 0 ? separator.length : 0;
    if (room - sep <= 0) break;
    const text = part.length > room - sep ? `${part.slice(0, room - sep - 1)}…` : part;
    out.push(text);
    room -= sep + text.length;
  }
  return out;
}

// timeOf is an RFC 3339 time as the person's clock reads it, `2026-10-26
// 14:05`; the text itself when it does not parse.
export function timeOf(at: string): string {
  const d = new Date(at);
  if (Number.isNaN(d.getTime())) return at;
  const two = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())} ${two(d.getHours())}:${two(d.getMinutes())}`;
}

// What a poll's result says: the data, or the line shown instead.
export type Answer<T> = { ok: true; value: T } | { ok: false; line: string };

export type Unreadable = { path: string; why: string };

type Listing = { briefs?: Summary[]; unreadable?: Unreadable[] };

// Listed is what brief list answers: the briefs, and the records it could not
// read, which the list shows rather than count as absent.
export type Listed = { briefs: Summary[]; unreadable: Unreadable[] };

export const listOf = (l: Listing): Listed => ({
  briefs: l.briefs ?? [],
  unreadable: l.unreadable ?? [],
});

export const unreadableLine = (u: Unreadable) => `unreadable: ${u.path} — ${u.why}`;

// asMain reads a rewake view as main's run, or says why it cannot.
async function asMain<T>(rewake: string, room: string, args: string[]): Promise<Answer<T>> {
  const as = mainAs();
  if (!as) return { ok: false, line: "main's run is not known yet; asking again" };
  const value = await readRewake<T>(rewake, room, args, as);
  if (value === undefined)
    return { ok: false, line: `rewake ${args.slice(0, 2).join(" ")} did not read` };
  return { ok: true, value };
}

export const readList = (rewake: string, room: string) =>
  asMain<Listing>(rewake, room, ["brief", "list", "--closed", "--json"]).then(
    (a): Answer<Listed> => (a.ok ? { ok: true, value: listOf(a.value) } : a),
  );

export const readBrief = (rewake: string, room: string, id: number) =>
  asMain<Detail>(rewake, room, ["brief", "show", String(id), "--json"]).then(
    (a): Answer<Detail> => {
      if (!a.ok) return a;
      const v = a.value;
      // A failed read must show a line, never throw in the view.
      if (typeof v?.text !== "string") return { ok: false, line: `brief ${id} did not read` };
      return { ok: true, value: { ...v, bound: v.bound ?? [], comments: v.comments ?? [] } };
    },
  );

// poll reads once at once and then once a `every` after each read ends,
// calling show with an answer only when it differs from the last one shown;
// it returns the stop. A read that throws is a line, never a crash.
export function poll<T>(
  read: () => Promise<Answer<T>>,
  show: (answer: Answer<T>) => void,
  every = 1000,
): () => void {
  let stopped = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let last: string | undefined;
  const once = async () => {
    let answer: Answer<T>;
    try {
      answer = await read();
    } catch {
      answer = { ok: false, line: "the briefs did not read" };
    }
    const key = JSON.stringify(answer);
    if (!stopped && key !== last) {
      last = key;
      show(answer);
    }
    if (!stopped) timer = setTimeout(once, every);
  };
  void once();
  return () => {
    stopped = true;
    clearTimeout(timer);
  };
}
