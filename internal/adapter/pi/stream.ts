// The stream of pi's own events to the wrapper, which translates them for
// rewake's face (translate.go; docs/research-pi.md, "The face's mirror"). It
// is transport alone: it keeps the events in order within its bounds and
// posts them in batches, one in flight; it never waits in pi's work and never
// throws into it.
//
// It holds at most bounds.held bytes unsent: past it the oldest delta goes
// first, which the block's end heals, then the oldest other event; each
// leaves a lost marker where it stood, counting what it stood for, and
// markers side by side merge. An event longer than a part goes as parts of
// its serialized text, put together by the wrapper; one whose lines would
// pass bounds.event has its longest strings cut, head and tail kept, then
// its longest arrays, their head kept, and one that still would is lost.

// Forwarded is one of pi's events as the stream carries it: its name and
// the fields read of it.
export type Forwarded = { event: string } & Record<string, unknown>;

export const bounds = {
  // One line of a batch, a part of a long event.
  part: 40960,
  // One batch, which so always holds at least one line.
  batch: 49152,
  // What the stream holds unsent.
  held: 262144,
  // One event serialized, its strings cut past it.
  event: 196608,
};

type Lost = { event: "lost"; deltas: number; events: number };
// An entry is an event's lines, one or its parts, with each line's size.
type Entry = { lines: Forwarded[]; sizes: number[]; size: number; delta: boolean; lost?: Lost };

// The stream's own words around a part's text.
const partEnvelope = 64;

// What the stream needs of the link: whether it can take a batch now, how to
// post one (true once the other end took it), and a timer.
export type Carrier = {
  ready: () => boolean;
  post: (lines: Forwarded[]) => Promise<boolean>;
  after: (ms: number, run: () => void) => void;
};

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

// pieces splits a text into pieces each within budget inside a JSON string,
// between characters.
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
// mark, and its tail within a quarter.
export function fit(text: string, budget: number): string {
  if (jsonSize(text) <= budget) return text;
  const [head = ""] = pieces(text, budget / 2);
  let tail = text.length;
  for (let size = 0; tail > head.length; ) {
    const at = tail >= 2 && isPair(text, tail - 2) ? tail - 2 : tail - 1;
    const next = charSize(text, at);
    if (size + next > budget / 4) break;
    size += next;
    tail = at;
  }
  return head + cutMark + text.slice(tail);
}

// cut is an event serialized within max, and the event it serializes, or
// undefined for one that cannot be: its longest strings are cut until it
// fits, then its longest arrays keep their head, on a copy, so pi's own
// objects are never touched. Many short strings, a call's hunks, no string
// cut shortens.
export function cut(event: Forwarded, max: number): [string, Forwarded] | undefined {
  let line = JSON.stringify(event);
  let size = Buffer.byteLength(line);
  if (size <= max) return [line, event];
  const copy = JSON.parse(line) as Forwarded;
  for (let i = 0; size > max && i < 64; i++) {
    const longest = longestString(copy);
    if (!longest) break;
    const [holder, key, text] = longest;
    const cutTo = fit(text, Math.max(256, jsonSize(text) - (size - max) - 1024));
    if (cutTo.length >= text.length) break;
    holder[key] = cutTo;
    line = JSON.stringify(copy);
    size = Buffer.byteLength(line);
  }
  for (let i = 0; size > max && i < 64; i++) {
    const longest = longestArray(copy);
    if (!longest) break;
    const [holder, key, list] = longest;
    const keep = Math.min(list.length - 1, Math.floor((list.length * max) / size));
    holder[key] = list.slice(0, Math.max(0, keep));
    line = JSON.stringify(copy);
    size = Buffer.byteLength(line);
  }
  return size <= max ? [line, copy] : undefined;
}

type Holder = Record<string | number, unknown>;

// longestArray is the array of most elements in the value.
function longestArray(value: unknown): [Holder, string | number, unknown[]] | undefined {
  let best: [Holder, string | number, unknown[]] | undefined;
  const walk = (holder: Holder) => {
    for (const [key, child] of Object.entries(holder)) {
      if (Array.isArray(child)) {
        if (child.length > 0 && (!best || child.length > best[2].length))
          best = [holder, key, child];
        walk(child as unknown as Holder);
      } else if (child && typeof child === "object") walk(child as Holder);
    }
  };
  walk(value as Holder);
  return best;
}

function longestString(value: unknown): [Holder, string | number, string] | undefined {
  let best: [Holder, string | number, string] | undefined;
  const walk = (holder: Holder) => {
    for (const [key, child] of Object.entries(holder)) {
      if (typeof child === "string") {
        if (!best || child.length > best[2].length) best = [holder, key, child];
      } else if (child && typeof child === "object") walk(child as Holder);
    }
  };
  walk(value as Holder);
  return best;
}

// linesOf is an event's lines within max in all, one or its parts, each
// part escaping the event's text again; undefined for one that cannot fit.
function linesOf(event: Forwarded, max: number): Omit<Entry, "delta"> | undefined {
  let budget = max;
  for (let i = 0; i < 4 && budget > 0; i++) {
    const fitted = cut(event, budget);
    if (!fitted) return undefined;
    const [line, cutEvent] = fitted;
    let lines: Forwarded[] = [cutEvent];
    let sizes = [Buffer.byteLength(line)];
    if (sizes[0] > bounds.part) {
      const texts = pieces(line, bounds.part - partEnvelope);
      lines = texts.map((text, part) => ({ event: "part", part, of: texts.length, text }));
      sizes = lines.map((one) => Buffer.byteLength(JSON.stringify(one)));
    }
    const size = sizes.reduce((sum, one) => sum + one, 0);
    if (size <= max) return { lines, sizes, size };
    budget = Math.floor((budget * max) / size) - 1024;
  }
  return undefined;
}

export class Stream {
  // on is the hello's answer: rewake's face draws this run.
  on = false;
  private queue: Entry[] = [];
  private held = 0;
  private sending = false;
  private scheduled = false;

  constructor(private carrier: Carrier) {}

  // bytes is what the stream holds unsent.
  get bytes() {
    return this.held;
  }

  // push takes an event, a delta one the first to go past the bound, cut so
  // its lines fit max; one that cannot be is lost where it would stand.
  push(event: Forwarded, delta = false, max = bounds.event) {
    if (!this.on) return;
    try {
      const entry = linesOf(event, Math.min(max, bounds.event));
      if (!entry) {
        this.mark(this.queue.length, delta ? 1 : 0, delta ? 0 : 1);
        this.kick();
        return;
      }
      while (this.held + entry.size > bounds.held && this.dropOne()) {}
      this.queue.push({ ...entry, delta });
      this.held += entry.size;
      this.kick();
    } catch {}
  }

  // dropOne drops the oldest delta, or else the oldest other event, leaving
  // a lost marker where it stood.
  private dropOne(): boolean {
    let at = this.queue.findIndex((q) => q.delta);
    if (at < 0) at = this.queue.findIndex((q) => !q.lost);
    if (at < 0) return false;
    const [dropped] = this.queue.splice(at, 1);
    this.held -= dropped.size;
    this.mark(at, dropped.delta ? 1 : 0, dropped.delta ? 0 : 1);
    return true;
  }

  // mark puts a lost marker at the queue's index, merged into one beside it.
  private mark(at: number, deltas: number, events: number) {
    const near = [this.queue[at - 1], this.queue[at]].find((q) => q?.lost);
    const lost = near?.lost ?? { event: "lost", deltas: 0, events: 0 };
    lost.deltas += deltas;
    lost.events += events;
    const size = Buffer.byteLength(JSON.stringify(lost));
    if (near) {
      this.held += size - near.size;
      near.size = size;
      near.sizes = [size];
      return;
    }
    this.queue.splice(at, 0, { lines: [lost], sizes: [size], size, delta: false, lost });
    this.held += size;
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

  // send posts the oldest lines, at most a batch; a batch that fails or
  // does not answer within 5 s is not sent again, and stands as lost first.
  async send() {
    if (this.sending || this.queue.length === 0 || !this.carrier.ready()) return;
    const lines: Forwarded[] = [];
    let size = 0;
    let deltas = 0;
    let events = 0;
    const counted = new Set<Entry>();
    while (this.queue.length > 0) {
      const entry = this.queue[0];
      const line = entry.lines[0];
      const lineSize = entry.sizes[0];
      if (lines.length > 0 && size + lineSize > bounds.batch) break;
      lines.push(line);
      size += lineSize;
      // A failed batch loses each event it carried a line of once.
      if (entry.lost) {
        deltas += entry.lost.deltas;
        events += entry.lost.events;
      } else if (!counted.has(entry)) {
        counted.add(entry);
        if (entry.delta) deltas++;
        else events++;
      }
      // A part sent leaves the rest of its event.
      entry.lines.shift();
      entry.sizes.shift();
      entry.size -= lineSize;
      this.held -= lineSize;
      if (entry.lines.length === 0) this.queue.shift();
    }
    this.sending = true;
    let done = false;
    const failed = () => {
      if (done) return;
      done = true;
      this.sending = false;
      this.mark(0, deltas, events);
      this.kick();
    };
    this.carrier.after(5000, failed);
    let ok = false;
    try {
      ok = await this.carrier.post(lines);
    } catch {}
    if (!ok) return failed();
    if (done) return;
    done = true;
    this.sending = false;
    this.kick();
  }
}
