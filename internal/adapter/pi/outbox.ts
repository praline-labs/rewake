// The outbox of a session rewake's own face draws (docs/face.md): the mirror's
// events wait here until the host takes them, sent in batches, one in flight.
// Pushing never waits and never throws into pi's work. It is bounded in UTF-8
// bytes: past its bound the oldest deltas go first, which the block's settle
// heals, then the oldest other events, each leaving a lost marker, and when
// the markers fill their reserve they collapse into one resync. The bounds
// are the Claude Code mod's (docs/claude.md, "The mirror").

export type MirrorEvent = {
  kind:
    | "prompt"
    | "delta"
    | "settle"
    | "tool"
    | "input"
    | "result"
    | "turn"
    | "notice"
    | "lost"
    | "resync";
  id?: string;
  turn?: string;
  block?: "text" | "thinking";
  text?: string;
  origin?: string;
  name?: string;
  status?: "done" | "error" | "refused";
  state?: "working" | "idle";
  part?: number;
  of?: number;
  lost?: number;
  permit?: { by: "hook"; allowed: boolean; reason?: string };
};

export const bounds = {
  // One event serialized; a longer text is split or cut within it.
  event: 40960,
  // One batch, which so always holds at least one event.
  batch: 49152,
  // What the outbox holds unsent, markers kept apart within it.
  outbox: 262144,
  markers: 16384,
  // A reply's block, cut to its head and tail past it as the face's item is.
  block: 262144,
  // A tool's input and its output, as the face's item keeps each.
  field: 65536,
};

// textBudget is room for an event's text, its other fields aside.
export const textBudget = bounds.event - 4096;

// jsonSize is a character's size inside a JSON string: its UTF-8 bytes with
// the escapes JSON.stringify writes.
function charSize(c: number): number {
  if (c === 0x22 || c === 0x5c || c === 0x0a || c === 0x0d || c === 0x09) return 2;
  if (c < 0x20) return 6;
  if (c < 0x80) return 1;
  if (c < 0x800) return 2;
  if (c > 0xffff) return 4;
  if (c >= 0xd800 && c <= 0xdfff) return 6;
  return 3;
}

// pieces splits a text into pieces each within budget, between characters;
// an empty text is one empty piece.
export function pieces(text: string, budget: number): string[] {
  const out: string[] = [];
  let piece = "";
  let size = 0;
  for (const ch of text) {
    const n = charSize(ch.codePointAt(0) ?? 0);
    if (size + n > budget && piece !== "") {
      out.push(piece);
      piece = "";
      size = 0;
    }
    piece += ch;
    size += n;
  }
  out.push(piece);
  return out;
}

const cutMark = "\n… cut …\n";

const sizeOf = (ch: string) => charSize(ch.codePointAt(0) ?? 0);

// fit is a text within budget: whole, or its head and its tail with a mark
// between, as the face's item keeps a long one.
export function fit(text: string, budget: number): string {
  const chars = [...text];
  let size = 0;
  for (const ch of chars) size += sizeOf(ch);
  if (size <= budget) return text;
  const half = Math.floor((budget - 32) / 2);
  const head = pieces(text, half)[0];
  let from = chars.length;
  for (let n = 0; from > 0 && n + sizeOf(chars[from - 1]) <= half; from--)
    n += sizeOf(chars[from - 1]);
  return head + cutMark + chars.slice(from).join("");
}

const isMarker = (event: MirrorEvent) =>
  event.kind === "turn" || event.kind === "lost" || event.kind === "resync";

type Queued = { event: MirrorEvent; size: number; marker: boolean };

// What the outbox needs of the link: whether it can take a batch now, how to
// post one (true once the host took it), and a timer.
export type Carrier = {
  ready: () => boolean;
  post: (events: MirrorEvent[]) => Promise<boolean>;
  after: (ms: number, run: () => void) => void;
};

export class Outbox {
  private queue: Queued[] = [];
  private events = 0;
  private markers = 0;
  // lost counts what was dropped since the last resync was queued.
  private lost = 0;
  private sending = false;
  private scheduled = false;

  constructor(private carrier: Carrier) {}

  get bytes() {
    return this.events + this.markers;
  }

  push(event: MirrorEvent) {
    const size = Buffer.byteLength(JSON.stringify(event));
    const marker = isMarker(event);
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

  // unqueue takes out what a later event replaces whole: a call's input.
  private unqueue(match: (q: Queued) => boolean) {
    this.queue = this.queue.filter((q) => {
      if (!match(q)) return true;
      this.events -= q.size;
      return false;
    });
  }

  // dropOne drops the oldest delta, or else the oldest other event with
  // every event of its item, leaving a lost marker for it.
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
    const resync: MirrorEvent = { kind: "resync", lost: this.lost + collapsed };
    this.lost = 0;
    const size = Buffer.byteLength(JSON.stringify(resync));
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
    const events: MirrorEvent[] = [];
    let size = 0;
    for (let q = this.queue[0]; q; q = this.queue[0]) {
      if (events.length > 0 && size + q.size > bounds.batch - 256) break;
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
