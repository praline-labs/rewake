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
  extra: { pane: string; fg?: string; bg?: string; cursor?: { x: number; y: number } },
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
  if (extra.cursor) frame.cursor = extra.cursor;
  return frame;
}

const requestMax = 4096;
const answerWithin = 5000;

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
    const server = Bun.listen<{ held: string }>({
      unix: path,
      socket: {
        open(socket) {
          socket.data = { held: "" };
        },
        data(socket, bytes) {
          socket.data.held += Buffer.from(bytes).toString("utf8");
          if (socket.data.held.length > requestMax) return void socket.end();
          const at = socket.data.held.indexOf("\n");
          if (at < 0) return;
          const line = socket.data.held.slice(0, at);
          socket.data.held = "";
          void answer(line, take).then((text) => {
            socket.write(`${text}\n`);
            socket.end();
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
    return JSON.stringify(frame);
  } catch (e) {
    return JSON.stringify({ error: e instanceof Error ? e.message : String(e) });
  }
}
