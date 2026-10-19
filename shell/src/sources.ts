// Where a pane's bytes come from (docs/v2/shell.md, "Who owns which pty"):
// main runs on a pty of the shell's own; a worker's pty is its space host's,
// read over the host's socket in the raw mode.
import type { Subprocess } from "bun"

export interface Source {
  // write is the person's keys, or the widget's answers for main.
  write(data: Uint8Array): void
  resize(cols: number, rows: number): void
  close(): void
}

// Queue keeps what a writer could not take yet and gives it on drain, so a
// long paste is never cut.
class Queue {
  private held: Uint8Array[] = []
  constructor(private put: (data: Uint8Array) => number) {}

  write(data: Uint8Array) {
    this.held.push(data)
    this.flush()
  }

  flush() {
    while (this.held.length > 0) {
      const data = this.held[0]!
      const n = Math.max(0, this.put(data))
      if (n < data.length) {
        this.held[0] = data.subarray(n)
        return
      }
      this.held.shift()
    }
  }
}

export type MainRun = Source & { child: Subprocess }

// startMain runs main's command line on a pty of its own: a session and a
// process group of its own, the pty its controlling terminal, so closing the
// pty is a hangup main hears.
export function startMain(command: string[], env: Record<string, string>, cwd: string,
  size: { cols: number; rows: number }, onData: (data: Uint8Array) => void): MainRun {
  let queue: Queue | undefined
  const child = Bun.spawn(command, {
    cwd, env,
    terminal: {
      ...size,
      data(_terminal, data) { onData(data) },
      drain() { queue?.flush() },
    },
  })
  const pty = child.terminal!
  queue = new Queue(data => pty.write(data))
  return {
    child,
    write: data => { if (!pty.closed) queue!.write(data) },
    resize: (cols, rows) => { if (!pty.closed) pty.resize(cols, rows) },
    close: () => { if (!pty.closed) pty.close() },
  }
}

// The socket's raw mode (internal/host/spacehost/bytes.go): the client
// writes the mode byte; the host answers the accepting byte, then the
// preamble and the live bytes. The client writes frames: a kind, a
// big-endian length of two bytes, the payload.
const modeBytes = 0x62
const accepted = 0x41
const frameInput = 0x69
const frameResize = 0x72
const maxFrame = 1 << 15

function frame(kind: number, payload: Uint8Array): Uint8Array {
  const out = new Uint8Array(3 + payload.length)
  out[0] = kind
  out[1] = payload.length >> 8
  out[2] = payload.length & 0xff
  out.set(payload, 3)
  return out
}

// connectWorker attaches to a space host's socket. onData gets the preamble
// and the live bytes; onEnd is called once, when the stream ends for any
// reason, after which the source writes nothing.
export function connectWorker(socket: string, size: { cols: number; rows: number },
  onData: (data: Uint8Array) => void, onEnd: () => void): Source {
  let conn: Bun.Socket | undefined
  let open = true
  let started = false
  let last = size
  const queue = new Queue(data => (conn ? conn.write(data) : 0))
  const end = () => {
    if (!open) return
    open = false
    conn?.end()
    onEnd()
  }
  const resizeFrame = (cols: number, rows: number) =>
    frame(frameResize, new Uint8Array([cols >> 8, cols & 0xff, rows >> 8, rows & 0xff]))
  Bun.connect({
    unix: socket,
    socket: {
      open(s) {
        conn = s
        if (!open) return void s.end()
        queue.write(new Uint8Array([modeBytes]))
        queue.write(resizeFrame(last.cols, last.rows))
      },
      data(_s, data) {
        if (!started) {
          if (data[0] !== accepted) return end()
          started = true
          data = data.subarray(1)
        }
        if (data.length > 0) onData(data)
      },
      drain() { queue.flush() },
      close() { end() },
      error() { end() },
    },
  }).catch(end)
  return {
    write(data) {
      // Keys typed before the host accepted are dropped: they would go
      // ahead of the mode byte.
      if (!started) return
      for (let at = 0; open && at < data.length; at += maxFrame) queue.write(frame(frameInput, data.subarray(at, at + maxFrame)))
    },
    resize(cols, rows) {
      last = { cols, rows }
      if (open && conn) queue.write(resizeFrame(cols, rows))
    },
    close: end,
  }
}
