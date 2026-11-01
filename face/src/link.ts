// The face's link to the session's mirror socket (docs/face.md, "The
// socket"): it connects, reads the stream's lines, writes the person's
// inputs, and connects again a second after the link is lost. The socket is
// handed in, so a test serves the lines itself.
import type { InputKind, Line } from "./mirror.gen.ts";
import { bounds } from "./mirror.gen.ts";
import type { Timers } from "./ticker.ts";

// Socket is the mirror's socket once open; Wire is what the link hears of it.
export type Socket = { write(data: string): number; end(): void };
export type Wire = {
  open(socket: Socket): void;
  data(socket: Socket, chunk: Uint8Array): void;
  close(): void;
  error(socket: Socket, err: Error): void;
};
// Connect opens the unix socket at path and tells wire what it does; it
// rejects when the socket does not open.
export type Connect = (path: string, wire: Wire) => Promise<unknown>;

// Hearing is what the face does with the link: fresh as a connection
// starts, line for each line read, saying whether what is drawn changed,
// changed once after a read that changed it, lost with why, empty when the
// mirror closed.
export type Hearing = {
  fresh(): void;
  line(line: Line): boolean;
  changed(): void;
  lost(why: string): void;
};

const reconnectAfter = 1000;
// A face reads lines of at most the protocol's bound; more without a
// newline is no stream of the wrapper's.
const maxPending = bounds.maxLineBytes * 2;

export class MirrorLink {
  private socket: Socket | undefined;

  constructor(
    private readonly connect: Connect,
    private readonly timers: Timers,
    private readonly hear: Hearing,
  ) {}

  get open() {
    return this.socket !== undefined;
  }

  // start connects to the socket at path afresh.
  start(path: string) {
    let pending = "";
    let ended = false;
    // A character may be split between two reads.
    const decoder = new TextDecoder();
    const end = (why: string) => {
      if (ended) return;
      ended = true;
      this.socket = undefined;
      this.hear.lost(why);
      this.timers.after(() => this.start(path), reconnectAfter);
    };
    this.hear.fresh();
    this.connect(path, {
      open: (s) => {
        this.socket = s;
      },
      data: (s, chunk) => {
        pending += decoder.decode(chunk, { stream: true });
        let at = pending.indexOf("\n");
        let changed = false;
        try {
          while (at >= 0) {
            const raw = pending.slice(0, at);
            pending = pending.slice(at + 1);
            if (raw) changed = this.hear.line(JSON.parse(raw) as Line) || changed;
            at = pending.indexOf("\n");
          }
          if (pending.length > maxPending) throw new Error("a line past the protocol's bound");
        } catch (err) {
          s.end();
          return end(`the mirror's stream does not read: ${String(err)}`);
        }
        if (changed) this.hear.changed();
      },
      close: () => end(""),
      error: (_s, err) => end(`the mirror's socket failed: ${String(err)}`),
    }).catch((err) => end(`the mirror's socket does not open: ${String(err)}`));
  }

  write(input: { id: string; kind: InputKind; text: string }) {
    this.socket?.write(`${JSON.stringify(input)}\n`);
  }
}
