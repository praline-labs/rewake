// What rewake screen asks of the shell (docs/screen.md): a window as cells,
// answered over a unix socket of the room, one request a connection.

import { existsSync, unlinkSync } from "node:fs";
import type { CapturedFrame, RGBA } from "@opentui/core";

// Run and Frame are internal/core/screen's model, as JSON.
export type Run = { t: string; w: number; fg?: string; bg?: string; a?: number };
export type Frame = {
  pane?: string;
  cols: number;
  rows: number;
  fg?: string;
  bg?: string;
  palette?: string[];
  cursor?: { x: number; y: number };
  lines: Run[][];
};

// OpenTUI's attribute bits, and the model's.
const bits: [number, number][] = [
  [1, 1], // bold
  [2, 2], // dim
  [4, 4], // italic
  [8, 8], // underline
  [32, 16], // inverse
  [128, 32], // strikethrough
];

const hex = (n: number) => Math.round(n).toString(16).padStart(2, "0");

// colourOf is "#rrggbb" for a colour that is one; the terminal's own default
// and a transparent cell stay empty, for the picture's defaults.
export function colourOf(c: RGBA | undefined): string | undefined {
  if (!c || c.intent === "default") return undefined;
  const [r, g, b, a] = c.toInts();
  if (a === 0) return undefined;
  return `#${hex(r)}${hex(g)}${hex(b)}`;
}

function attrsOf(attributes: number): number {
  let out = 0;
  for (const [from, to] of bits) if (attributes & from) out |= to;
  return out;
}

// frameOf is a captured window as the model: the runs of each line, a wide
// glyph's columns counted by the renderer, equal neighbours not merged.
export function frameOf(
  captured: CapturedFrame,
  extra: {
    pane: string;
    fg?: string;
    bg?: string;
    palette?: (string | null)[];
    cursor?: { x: number; y: number };
  },
): Frame {
  const lines = captured.lines.map((line) =>
    line.spans.map((span) => {
      const run: Run = { t: span.text, w: span.width };
      const fg = colourOf(span.fg);
      const bg = colourOf(span.bg);
      if (fg) run.fg = fg;
      if (bg) run.bg = bg;
      const a = attrsOf(span.attributes);
      if (a) run.a = a;
      return run;
    }),
  );
  const frame: Frame = { pane: extra.pane, cols: captured.cols, rows: captured.rows, lines };
  if (extra.fg) frame.fg = extra.fg;
  if (extra.bg) frame.bg = extra.bg;
  // The terminal's own sixteen, an entry empty where it gave none.
  if (extra.palette?.some(Boolean)) frame.palette = extra.palette.map((c) => c ?? "");
  if (extra.cursor) frame.cursor = extra.cursor;
  return frame;
}

const requestMax = 4096;
const answerWithin = 5000;
// The model's bounds (internal/core/screen), held here too so a window the
// CLI would refuse is not written.
const answerMax = 4 << 20;
const maxCols = 400;
const maxRows = 200;

type Conn = { held: string; out?: Buffer; sent: number; timer?: ReturnType<typeof setTimeout> };

// flush writes the answer as the socket takes it and ends the connection
// when all of it is out: a write of a large frame is partial, the rest waits
// for drain.
function flush(socket: { write(d: Buffer): number; end(): void; data: Conn }) {
  const c = socket.data;
  if (!c.out) return;
  while (c.sent < c.out.length) {
    const n = socket.write(c.out.subarray(c.sent));
    if (n <= 0) return;
    c.sent += n;
  }
  clearTimeout(c.timer);
  socket.end();
}

// listenScreen serves the requests at path: a line {"pane": "<id>|main"}, the
// answer a line with the frame, or {"error": "..."}. It returns what stops
// the server and removes the socket, or undefined while the path cannot be
// listened on yet — the room's folder is made by the main this shell starts.
export function listenScreen(
  path: string,
  take: (pane: string) => Promise<Frame>,
): (() => void) | undefined {
  try {
    // A socket left by a shell that was killed; the room has one main.
    if (existsSync(path)) unlinkSync(path);
    const server = Bun.listen<Conn>({
      unix: path,
      socket: {
        open(socket) {
          // A request that never completes is dropped.
          socket.data = { held: "", sent: 0, timer: setTimeout(() => socket.end(), answerWithin) };
        },
        close(socket) {
          clearTimeout(socket.data.timer);
        },
        drain(socket) {
          flush(socket);
        },
        data(socket, bytes) {
          socket.data.held += Buffer.from(bytes).toString("utf8");
          if (socket.data.held.length > requestMax) return void socket.end();
          const at = socket.data.held.indexOf("\n");
          if (at < 0) return;
          const line = socket.data.held.slice(0, at);
          socket.data.held = "";
          void answer(line, take).then((text) => {
            socket.data.out = Buffer.from(`${text}\n`);
            flush(socket);
          });
        },
      },
    });
    return () => {
      server.stop(true);
      try {
        unlinkSync(path);
      } catch {}
    };
  } catch {
    return undefined;
  }
}

async function answer(line: string, take: (pane: string) => Promise<Frame>): Promise<string> {
  let pane: unknown;
  try {
    pane = (JSON.parse(line) as { pane?: unknown }).pane;
  } catch {
    return JSON.stringify({ error: "the request is not a JSON line" });
  }
  if (typeof pane !== "string" || pane === "")
    return JSON.stringify({ error: 'the request names no pane: {"pane": "<id>|main"}' });
  try {
    const frame = await Promise.race([
      take(pane),
      new Promise<never>((_, no) =>
        setTimeout(() => no(new Error("the shell did not draw it in time")), answerWithin),
      ),
    ]);
    if (frame.cols > maxCols || frame.rows > maxRows)
      throw new Error(
        `the window is ${frame.cols}x${frame.rows} cells, over ${maxCols}x${maxRows}`,
      );
    const text = JSON.stringify(frame);
    if (Buffer.byteLength(text) >= answerMax)
      throw new Error(
        `the window is ${Buffer.byteLength(text)} bytes as an answer, over ${answerMax - 1}`,
      );
    return text;
  } catch (e) {
    return JSON.stringify({ error: e instanceof Error ? e.message : String(e) });
  }
}
