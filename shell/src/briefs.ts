// The briefs the modal shows (docs/shell.md, "The briefs"): the JSON of
// rewake brief list and show, read as main's run once a second while the
// modal shows one of them, and the pure parts of drawing them.
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

// A piece of a line: faint is drawn in the faint colour.
export type Piece = { text: string; faint: boolean };

// ended says whether a binding has ended: its run, or the brief itself.
export const ended = (brief: Summary, b: Bound) => brief.closed || b.run === "ended";

// rowOf is a list row as pieces: `3 · Auth refactor · 2/5 · 4 comments ·
// write-claude, review-claude (ended)`. A closed brief is faint whole, and an
// ended binding is faint in an open one.
export function rowOf(brief: Summary): Piece[] {
  const faint = brief.closed;
  const comments = `${brief.commentCount} comment${brief.commentCount === 1 ? "" : "s"}`;
  const head = [`${brief.id}`, brief.title, `${brief.done}/${brief.total}`, comments];
  const pieces: Piece[] = [{ text: `${head.join(" · ")}`, faint }];
  if (brief.closed) pieces.push({ text: " · closed", faint });
  brief.bound.forEach((b, i) => {
    const gone = ended(brief, b);
    pieces.push({ text: i === 0 ? " · " : ", ", faint });
    pieces.push({ text: gone && !brief.closed ? `${b.name} (ended)` : b.name, faint: gone });
  });
  return pieces;
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
