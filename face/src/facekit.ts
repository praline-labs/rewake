// What the face's in-process tests share (face.test.ts): a Face driven as
// the shell drives the worker, served by a mirror socket of the test's own
// and moved on a clock held in the test's hand, so no worker, socket or
// sleep stands between a message and what the face posts.
import { Face } from "./face.ts";
import type { Socket, Wire } from "./link.ts";
import { contract, type FaceLine, type FromFace, type ToFace } from "./messages.ts";
import { bounds } from "./mirror.gen.ts";
import type { Timers } from "./ticker.ts";

// epoch is the moment a test's clock starts at.
export const epoch = 1_000_000;

// Hand is a clock that moves only when told: advance runs each timer due on
// the way, in order, at its own moment.
export class Hand implements Timers {
  at = epoch;
  private next = 1;
  private readonly timers = new Map<number, { at: number; every?: number; run: () => void }>();

  now = () => this.at;
  repeat = (run: () => void, ms: number) => this.add(run, ms, ms);
  after = (run: () => void, ms: number) => this.add(run, ms);
  stop = (timer: unknown) => {
    this.timers.delete(timer as number);
  };
  private add(run: () => void, ms: number, every?: number) {
    this.timers.set(this.next, { at: this.at + ms, every, run });
    return this.next++;
  }
  advance(ms: number) {
    const to = this.at + ms;
    for (;;) {
      const due = [...this.timers].filter(([, t]) => t.at <= to).sort((a, b) => a[1].at - b[1].at);
      const [id, timer] = due[0] ?? [];
      if (id === undefined || !timer) break;
      this.at = timer.at;
      if (timer.every) timer.at += timer.every;
      else this.timers.delete(id);
      timer.run();
    }
    this.at = to;
  }
}

export type Served = {
  // inputs the session takes; turn and the rest go on the begin line.
  inputs?: string[];
  begin?: object;
  items?: object[];
  // refuse answers every input with this refusal; else each is taken.
  refuse?: string;
  // closed refuses to open the socket.
  closed?: boolean;
};

const encoder = new TextEncoder();
const linesOf = (lines: object[]) =>
  encoder.encode(lines.map((l) => `${JSON.stringify(l)}\n`).join(""));

// replies is a conversation of one text reply for each text given.
export const replies = (texts: string[]) =>
  texts.map((text, i) => ({ id: `r${i}`, kind: "text", status: "settled", text }));

// faced is a face of the size given, started on a mirror serving what is
// given, each connection a snapshot of it.
export function faced(served: Served = {}, width = 80, height = 24) {
  const hand = new Hand();
  const posts: FromFace[] = [];
  const heard: { id: string; kind: string; text: string }[] = [];
  const items = served.items ?? [];
  let wire: Wire | undefined;
  let socket: Socket | undefined;
  let connects = 0;
  const face = new Face(
    (message) => posts.push(message),
    (_path, w) => {
      connects++;
      if (served.closed) return Promise.reject(new Error("no such socket"));
      wire = w;
      const s: Socket = {
        write(data) {
          for (const raw of data.split("\n").filter(Boolean)) {
            const input = JSON.parse(raw);
            heard.push(input);
            const answer = served.refuse
              ? { op: "answer", id: input.id, ok: false, detail: served.refuse }
              : { op: "answer", id: input.id, ok: true };
            w.data(s, linesOf([answer]));
          }
          return data.length;
        },
        end() {},
      };
      socket = s;
      w.open(s);
      w.data(
        s,
        linesOf([
          { op: "hello", v: bounds.version, inputs: served.inputs ?? ["prompt"] },
          { op: "begin", items: items.length, dropped: 0, turn: "idle", ...served.begin },
          ...items.map((item) => ({ op: "item", item })),
          { op: "end" },
        ]),
      );
      return Promise.resolve();
    },
    hand,
  );
  const take = (message: ToFace) => face.take(message);
  take({ kind: "start", contract, path: "/m.sock", width, height });
  // shown is the last frame posted, as text rows trimmed at their end.
  const frame = () => {
    const last = posts.findLast((p) => p.kind === "lines");
    if (last?.kind !== "lines") throw new Error("no frame posted");
    return last;
  };
  const shown = () => frame().lines.map(textOf);
  return {
    face,
    take,
    hand,
    posts,
    heard,
    frame,
    shown,
    connects: () => connects,
    // send writes a line of the mirror's to the face connected last.
    send: (...lines: object[]) => socket && wire?.data(socket, linesOf(lines)),
    // raw writes bytes of the mirror's as they come.
    raw: (bytes: Uint8Array) => socket && wire?.data(socket, bytes),
    // cut is the mirror closing the connection.
    cut: () => wire?.close(),
    // tick moves the clock one tick on.
    tick: () => hand.advance(50),
    rowOf: (text: string) => shown().findIndex((l) => l.startsWith(text)),
    mouse: (type: "down" | "drag" | "up", x: number, y: number) =>
      take({ kind: "mouse", type, x, y, focused: true }),
  };
}

export const textOf = (line: FaceLine) =>
  line.spans
    .map((s) => s.text)
    .join("")
    .trimEnd();
