// Where a pane's bytes come from (docs/shell.md, "Who owns which pty"):
// main's pty is its host's, the process of the person's pane; a worker's is
// its space host's. Each is read over its host's socket in the raw mode.

export interface Source {
  // write is the person's keys.
  write(data: Uint8Array): void;
  resize(cols: number, rows: number): void;
  close(): void;
}

// Queue keeps what a writer could not take yet and gives it on drain, so a
// long paste is never cut.
class Queue {
  private held: Uint8Array[] = [];
  constructor(private put: (data: Uint8Array) => number) {}

  write(data: Uint8Array) {
    this.held.push(data);
    this.flush();
  }

  flush() {
    while (this.held.length > 0) {
      const data = this.held[0]!;
      const n = Math.max(0, this.put(data));
      if (n < data.length) {
        this.held[0] = data.subarray(n);
        return;
      }
      this.held.shift();
    }
  }
}

// The socket's raw mode (internal/host/stream/bytes.go): the client
// writes the mode byte; the host answers the accepting byte, then the
// preamble and the live bytes. The client writes frames: a kind, a
// big-endian length of two bytes, the payload.
const modeBytes = 0x62;
const accepted = 0x41;
const frameInput = 0x69;
const frameResize = 0x72;
const maxFrame = 1 << 15;

function frame(kind: number, payload: Uint8Array): Uint8Array {
  const out = new Uint8Array(3 + payload.length);
  out[0] = kind;
  out[1] = payload.length >> 8;
  out[2] = payload.length & 0xff;
  out.set(payload, 3);
  return out;
}

// connectHost attaches to a host's socket: main's host or a space host. onData gets the preamble
// and the live bytes; onEnd is called once, when the stream ends for any
// reason, after which the source writes nothing.
export function connectHost(
  socket: string,
  size: { cols: number; rows: number },
  onData: (data: Uint8Array) => void,
  onEnd: () => void,
): Source {
  let conn: Bun.Socket | undefined;
  let open = true;
  let started = false;
  let last = size;
  const queue = new Queue((data) => (conn ? conn.write(data) : 0));
  const end = () => {
    if (!open) return;
    open = false;
    conn?.end();
    onEnd();
  };
  const resizeFrame = (cols: number, rows: number) =>
    frame(frameResize, new Uint8Array([cols >> 8, cols & 0xff, rows >> 8, rows & 0xff]));
  Bun.connect({
    unix: socket,
    socket: {
      open(s) {
        conn = s;
        if (!open) return void s.end();
        queue.write(new Uint8Array([modeBytes]));
        queue.write(resizeFrame(last.cols, last.rows));
      },
      data(_s, data) {
        if (!started) {
          if (data[0] !== accepted) return end();
          started = true;
          data = data.subarray(1);
        }
        if (data.length > 0) onData(data);
      },
      drain() {
        queue.flush();
      },
      close() {
        end();
      },
      error() {
        end();
      },
    },
  }).catch(end);
  return {
    write(data) {
      // Keys typed before the host accepted are dropped: they would go
      // ahead of the mode byte.
      if (!started) return;
      for (let at = 0; open && at < data.length; at += maxFrame)
        queue.write(frame(frameInput, data.subarray(at, at + maxFrame)));
    },
    resize(cols, rows) {
      last = { cols, rows };
      if (open && conn) queue.write(resizeFrame(cols, rows));
    },
    close: end,
  };
}
