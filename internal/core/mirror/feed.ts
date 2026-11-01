// The feed of a session's mirror (docs/face.md, "The feed"): the one module
// every adapter's harness side sends its conversation through, in the
// mirror's neutral events. An adapter maps its harness's events to these and
// pushes them; the feed bounds their texts, holds them in a bounded outbox and
// posts them in batches, one in flight. Pushing never waits and never throws
// into the harness's work.
//
// The outbox is bounded in UTF-8 bytes of the events serialized: past its
// bound the oldest deltas go first, which the block's settle heals, then the
// oldest other events, each item leaving a lost marker, and when the markers
// fill their reserve they collapse into one resync. A text is cut as the
// model's clip cuts one (clip.go): its head within half the bound, its tail
// within a quarter, the cut mark between.
//
// It names no harness and uses no runtime's own library: it runs inside a
// harness's extension host, which may have neither Node nor a module loader
// beyond static relative imports.

export type Block = "text" | "thinking";

// MirrorEvent is one thing the session showed, in the model's neutral words
// (internal/core/mirror/event.go); a harness sends the kinds it has.
export type MirrorEvent = {
  kind:
    | "prompt"
    | "delta"
    | "settle"
    | "tool"
    | "input"
    | "result"
    | "turn"
    | "mode"
    | "level"
    | "notice"
    | "lost"
    | "resync"
    | "taken";
  id?: string;
  turn?: string;
  block?: Block;
  text?: string;
  origin?: string;
  name?: string;
  status?: "done" | "error" | "refused" | "queued";
  state?: "working" | "idle" | "compacting";
  part?: number;
  of?: number;
  lost?: number;
  // retry is how many of the working turn's requests failed in a row, on a
  // turn event saying working.
  retry?: number;
  permit?: { by: string; allowed: boolean; reason?: string };
  mode?: string;
  // level is the session's thinking level in the model's words, on a level
  // event: the one the face's shift+tab moves.
  level?: string;
};

export const bounds = {
  // One event serialized; a longer text is split or cut within it.
  event: 40960,
  // One batch, which so always holds at least one event.
  batch: 49152,
  // What the outbox holds unsent, markers kept apart within it.
  outbox: 262144,
  markers: 16384,
  // A block put together for its settle, cut to its head and tail past it.
  block: 262144,
};

// textBudget is room for an event's text, its other fields aside.
export const textBudget = bounds.event - 4096;

// A batch's envelope, the session's words around its events.
const envelope = 256;

function isPair(text: string, i: number): boolean {
  const c = text.charCodeAt(i);
  if (c < 0xd800 || c > 0xdbff || i + 1 >= text.length) return false;
  const d = text.charCodeAt(i + 1);
  return d >= 0xdc00 && d <= 0xdfff;
}

// charSize is what the character at i takes inside a JSON string: its UTF-8
// bytes, with the escapes JSON.stringify writes.
function charSize(text: string, i: number): number {
  const c = text.charCodeAt(i);
  if (c === 0x22 || c === 0x5c || c === 0x0a || c === 0x0d || c === 0x09) return 2;
  if (c < 0x20) return 6;
  if (c < 0x80) return 1;
  if (c < 0x800) return 2;
  if (isPair(text, i)) return 4;
  if (c >= 0xd800 && c <= 0xdfff) return 6;
  return 3;
}

// jsonSize is a text's size inside a JSON string, without the quotes.
export function jsonSize(text: string): number {
  let size = 0;
  for (let i = 0; i < text.length; i += isPair(text, i) ? 2 : 1) size += charSize(text, i);
  return size;
}

// utf8Size is a serialized value's size on the wire: JSON.stringify leaves
// no control character or lone surrogate unescaped, so its UTF-8 bytes are
// the body's.
export function utf8Size(serialized: string): number {
  let size = 0;
  for (let i = 0; i < serialized.length; i++) {
    const c = serialized.charCodeAt(i);
    if (c < 0x80) size += 1;
    else if (c < 0x800) size += 2;
    else if (isPair(serialized, i)) {
      size += 4;
      i++;
    } else size += 3;
  }
  return size;
}

// pieces splits a text into pieces each within budget, between characters;
// an empty text is one empty piece.
export function pieces(text: string, budget: number): string[] {
  const out: string[] = [];
  let start = 0;
  let size = 0;
  for (let i = 0; i < text.length; i += isPair(text, i) ? 2 : 1) {
    const next = charSize(text, i);
    if (size + next > budget && i > start) {
      out.push(text.slice(start, i));
      start = i;
      size = 0;
    }
    size += next;
  }
  if (start < text.length || out.length === 0) out.push(text.slice(start));
  return out;
}

const cutMark = "\n… cut …\n";

// fit is a text within budget: whole, or its head within half of it, the cut
// mark, and its tail within a quarter, as the model's clip keeps one.
export function fit(text: string, budget: number): string {
  if (jsonSize(text) <= budget) return text;
  let head = 0;
  for (let size = 0; head < text.length; ) {
    const next = charSize(text, head);
    if (size + next > budget / 2) break;
    size += next;
    head += isPair(text, head) ? 2 : 1;
  }
  let tail = text.length;
  for (let size = 0; tail > head; ) {
    const at = tail >= 2 && isPair(text, tail - 2) ? tail - 2 : tail - 1;
    const next = charSize(text, at);
    if (size + next > budget / 4) break;
    size += next;
    tail = at;
  }
  return text.slice(0, head) + cutMark + text.slice(tail);
}

export type Settle = {
  kind: "settle";
  id: string;
  turn?: string;
  block: Block;
  part: number;
  of: number;
  text: string;
};

// settles is a block's settle in parts, each within an event: the block's
// whole text, cut past bounds.block, so the deltas lost on the way heal.
export function settles(
  id: string,
  turn: string | undefined,
  block: Block,
  text: string,
): Settle[] {
  const parts = pieces(fit(text, bounds.block), textBudget);
  return parts.map((part, i) => ({
    kind: "settle",
    id,
    ...(turn === undefined ? {} : { turn }),
    block,
    part: i,
    of: parts.length,
    text: part,
  }));
}

// A marker is what the outbox itself says: an item it dropped, or a resync
// standing for every event it lost.
export type Marker =
  | { kind: "lost"; id: string; lost?: undefined }
  | { kind: "resync"; id?: undefined; lost: number };

// What the outbox reads of an event; the adapter's own type may say more.
export type Fed = { kind: string; id?: string; lost?: number };

// What the outbox needs of the link: whether it can take a batch now, how to
// post one (true once the other end took it), and a timer.
export type Carrier<E extends Fed = MirrorEvent> = {
  ready: () => boolean;
  post: (events: (E | Marker)[]) => Promise<boolean>;
  after: (ms: number, run: () => void) => void;
};

type Queued<E> = { event: E | Marker; size: number; marker: boolean };

const isMarker = (event: Fed) =>
  event.kind === "turn" || event.kind === "lost" || event.kind === "resync";

export class Outbox<E extends Fed = MirrorEvent> {
  private queue: Queued<E>[] = [];
  private events = 0;
  private markers = 0;
  // lost counts what was dropped since the last resync was queued.
  private lost = 0;
  private sending = false;
  private scheduled = false;

  constructor(private carrier: Carrier<E>) {}

  // bytes is what the outbox holds, events and markers.
  get bytes() {
    return this.events + this.markers;
  }

  push(event: E | Marker) {
    const size = utf8Size(JSON.stringify(event));
    // Every text is fitted within an event before it comes here; one that
    // still is not would pass the batch's cap, so its item stands as lost.
    if (size > bounds.event) {
      this.lost++;
      if (event.id !== undefined) this.push({ kind: "lost", id: event.id });
      return;
    }
    const marker = isMarker(event);
    // A call's input replaces one still queued: each carries all of it.
    if (event.kind === "input")
      this.unqueue((q) => q.event.kind === "input" && q.event.id === event.id);
    if (marker) {
      while (this.markers + size > bounds.markers && this.collapse()) {}
      this.markers += size;
    } else {
      while (this.events + size > bounds.outbox - bounds.markers && this.dropOne()) {}
      this.events += size;
    }
    this.queue.push({ event, size, marker });
    this.kick();
  }

  private unqueue(match: (q: Queued<E>) => boolean) {
    this.queue = this.queue.filter((q) => {
      if (!match(q)) return true;
      this.events -= q.size;
      return false;
    });
  }

  // dropOne drops the oldest delta, or else the oldest other event with
  // every event of its item (of a settle, its other parts), leaving a lost
  // marker for the item; an event naming no item goes alone.
  private dropOne(): boolean {
    let at = this.queue.findIndex((q) => q.event.kind === "delta");
    if (at >= 0) {
      this.events -= this.queue[at].size;
      this.queue.splice(at, 1);
      this.lost++;
      return true;
    }
    at = this.queue.findIndex((q) => !q.marker);
    if (at < 0) return false;
    const { id, kind } = this.queue[at].event;
    this.queue = this.queue.filter((q, i) => {
      const same =
        i === at ||
        (!q.marker &&
          id !== undefined &&
          q.event.id === id &&
          (kind !== "settle" || q.event.kind === kind));
      if (same) {
        this.events -= q.size;
        this.lost++;
      }
      return !same;
    });
    if (id !== undefined) this.push({ kind: "lost", id });
    return true;
  }

  // collapse turns every queued marker into one resync standing for them.
  private collapse(): boolean {
    if (this.markers === 0) return false;
    let collapsed = 0;
    this.queue = this.queue.filter((q) => {
      if (q.marker) {
        collapsed += q.event.kind === "resync" ? (q.event.lost ?? 0) : 1;
        this.markers -= q.size;
      }
      return !q.marker;
    });
    const resync: Marker = { kind: "resync", lost: this.lost + collapsed };
    this.lost = 0;
    const size = utf8Size(JSON.stringify(resync));
    this.markers += size;
    this.queue.unshift({ event: resync, size, marker: true });
    return true;
  }

  // kick sends what waits 50 ms from now, the link willing; one that is down
  // keeps it until the next push or kick.
  kick() {
    if (this.scheduled || this.sending || this.queue.length === 0) return;
    this.scheduled = true;
    this.carrier.after(50, () => {
      this.scheduled = false;
      void this.send();
    });
  }

  // send posts the oldest events, at most a batch; a batch that fails or
  // does not answer within 5 s is dropped, never sent again, and stands as
  // lost behind a resync.
  async send() {
    if (this.sending || this.queue.length === 0 || !this.carrier.ready()) return;
    const events: (E | Marker)[] = [];
    let size = 0;
    for (let q = this.queue[0]; q; q = this.queue[0]) {
      if (events.length > 0 && size + q.size > bounds.batch - envelope) break;
      this.queue.shift();
      if (q.marker) this.markers -= q.size;
      else this.events -= q.size;
      size += q.size;
      events.push(q.event);
    }
    this.sending = true;
    let done = false;
    const failed = () => {
      if (done) return;
      done = true;
      this.sending = false;
      this.lost += events.length;
      const lost = this.lost;
      this.lost = 0;
      this.push({ kind: "resync", lost });
    };
    this.carrier.after(5000, failed);
    let ok = false;
    try {
      ok = await this.carrier.post(events);
    } catch {}
    if (!ok) return failed();
    if (done) return;
    done = true;
    this.sending = false;
    this.kick();
  }
}
