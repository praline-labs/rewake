// rewake's face for one session, run as a Bun Worker the shell starts and may
// end at any moment (docs/face.md, "Hot reload"): it connects to the session's
// mirror socket itself, keeps its copy of the model and posts the lines to
// draw. Everything it does is caught here and posted as an error, so a face
// that breaks leaves its last lines on the screen and the shell untouched.

import { Edge, edgeRows } from "./edge.ts";
import {
  contract,
  type FaceLine,
  type FromFace,
  type Link,
  type Pointing,
  type ToFace,
} from "./messages.ts";
import type { Line, PermissionMode } from "./mirror.gen.ts";
import { bounds } from "./mirror.gen.ts";
import { FaceModel } from "./model.ts";
import { partOf } from "./parts.ts";
import { Flash, Pointer, pointAt, type View } from "./pointer.ts";
import { Tracker } from "./progress.ts";
import { blinked, bodyOf, fill, statusOf } from "./render.ts";
import { Scroll } from "./scroll.ts";
import { copyOf, highlighted, lit } from "./select.ts";
import { Ticker } from "./ticker.ts";
import type { Clock } from "./turn.ts";
import { type Anchor, anchorAt, toggles, topFor, Unfold } from "./unfold.ts";
import { Wheel } from "./wheel.ts";

declare const self: Worker;

const post = (message: FromFace) => self.postMessage(message);
const reconnectAfter = 1000;

let path = "";
let width = 80;
let height = 24;
const scroll = new Scroll();
const wheel = new Wheel();
const pointer = new Pointer();
// laid is the conversation's rows as the last tick laid them out, and stale
// once it or the width changed: only a tick lays them out again.
let laid: FaceLine[] = [];
let stale = true;
// frame is what the last tick committed: every row from the same moment. A
// scroll cuts only it, so a change of the conversation waits for its tick.
let frame:
  | {
      rows: FaceLine[];
      now: number;
      turn: string;
      link: Link;
      prompts: boolean;
      mode?: PermissionMode;
    }
  | undefined;
let link: Link = "connecting";
let model = new FaceModel();
let socket: { write(data: string): number; end(): void } | undefined;
// The frame last posted: a tick that draws the same posts nothing.
let posted = "";
// progress follows the working turn from tick to tick.
const progress = new Tracker();
// unfold is what the person opened, everything by the shell's key or a part
// by a click; anchor is the row to keep where it stood on the screen once
// the conversation is laid out again: its part's key, its place among that
// part's rows and its row in the view.
const unfold = new Unfold();
let anchor: Anchor | undefined;

// turned follows the working turn's progress from the turn's start, which
// the model carries.
function turned(from: string, to: string) {
  if (from !== to && to === "working") progress.reset(model.since ?? Date.now(), model.items);
}

const clockAt = (now: number): Clock => ({
  now,
  since: model.since,
  worked: model.worked,
  progress: model.turn === "working" ? progress.at(model.items, now) : undefined,
});

// draw is the tick's frame: the whole conversation, laid out again when it
// changed (the full model costs a few ms), and the turn's rows under it,
// every row from the same clock.
function draw(now: number) {
  try {
    if (stale) laid = bodyOf(model, width, Infinity, unfold).rows;
    stale = false;
    if (anchor) scroll.top = topFor(laid, anchor) ?? scroll.top;
    anchor = undefined;
    const rows = [...laid, ...statusOf(model, clockAt(now), width)];
    const prompts = model.inputs.includes("prompt");
    frame = { rows, now, turn: model.turn, link, prompts, mode: model.mode };
    paint();
  } catch (err) {
    post({ kind: "error", text: `the face's drawing failed: ${String(err)}` });
  }
}

// viewOf is where the committed frame's rows stand in the face.
const viewOf = (): View => {
  const total = frame?.rows.length ?? 0;
  return {
    top: scroll.start(total, height),
    shown: Math.min(total, height),
    height,
    width,
  };
};

// flash is the part a click copied, lit for a moment with the notice.
const flash = new Flash(() => paint());

// paint posts the view of the committed frame, unless it is what was posted:
// the selection marked, and the part a click just copied lit.
function paint() {
  if (!frame) return;
  try {
    const shown = scroll.view(frame.rows, height);
    const view = viewOf();
    const marked = lit(highlighted(shown, view.top, pointer.selection), flash.part);
    const message: FromFace = {
      kind: "lines",
      lines: fill(blinked(marked, frame.now), width, height),
      turn: frame.turn,
      link: frame.link,
      prompts: frame.prompts,
      mode: frame.mode,
    };
    const sent = JSON.stringify(message);
    if (sent === posted) return;
    posted = sent;
    post(message);
  } catch (err) {
    post({ kind: "error", text: `the face's drawing failed: ${String(err)}` });
  }
}

// What moves with time alone: a working turn's line, a running call's dot.
const moving = () =>
  model.turn === "working" ||
  model.turn === "compacting" ||
  model.items.some((item) => item.status === "running");

const ticker = new Ticker(draw, moving);
// soon paints a change of the conversation at the next tick.
const soon = () => {
  stale = true;
  ticker.change();
};

// scrolled moves the view over the committed frame and paints it at once: a
// scroll is the person's hand, not a change of the conversation, so it waits
// for no tick and lays nothing out.
function scrolled(by: number) {
  if (!frame) return;
  scroll.by(by, frame.rows.length, height);
  paint();
}

// A drag past the body's edge scrolls the view, the selection's end with it.
const edge = new Edge(
  () => height,
  (way) => {
    if (!frame) return false;
    scroll.by(way * edgeRows, frame.rows.length, height);
    const { x, y } = edge.at;
    pointer.drag(pointAt(x, y, viewOf()), x, y);
    paint();
    return true;
  },
);

// pin keeps a row of the committed frame where it stands in the view once
// the conversation is laid out again, by the part it is a row of.
function pin(row: number) {
  if (frame) anchor = anchorAt(frame.rows, row, viewOf().top);
}

// unfolded opens or closes everything; a view scrolled up keeps the first
// row of a part it shows where it was, one at the bottom stays there.
function unfolded(all: boolean) {
  if (all === unfold.all) return;
  unfold.every(all);
  if (frame && scroll.top !== undefined) {
    const { top, shown } = viewOf();
    const row = frame.rows.findIndex((r, i) => i >= top && i < top + shown && r.part);
    if (row >= 0) pin(row);
  }
  soon();
}

// pointed takes the person's hand: a release copies the selection it made,
// or the part a click was on; a click on a run's line or a cut row opens or
// closes it instead, the row staying under the pointer.
function pointed(type: Pointing, x: number, y: number, focused: boolean) {
  if (!frame) return;
  const at = pointAt(x, y, viewOf());
  switch (type) {
    case "down":
      edge.stop();
      flash.off();
      pointer.down(viewOf(), x, y, focused);
      break;
    case "drag":
      edge.at = { x, y };
      if (pointer.drag(at, x, y)) edge.follow();
      break;
    case "up": {
      edge.stop();
      const gesture = pointer.up();
      if (gesture && "selected" in gesture) {
        const text = copyOf(frame.rows, gesture.selected);
        if (text.trim()) post({ kind: "copy", text, what: "selection" });
      } else if (gesture) {
        const key = frame.rows[gesture.clicked]?.part;
        if (key !== undefined && toggles(key)) {
          unfold.toggle(key);
          pin(gesture.clicked);
          soon();
          break;
        }
        const part = key === undefined ? undefined : partOf(model.items, key);
        if (key !== undefined && part) {
          post({ kind: "copy", text: part.text, what: part.what });
          flash.on(key);
        }
      }
      break;
    }
  }
  paint();
}

function lost(why: string) {
  socket = undefined;
  link = "lost";
  soon();
  if (why) post({ kind: "error", text: why });
  setTimeout(connect, reconnectAfter);
}

// A face reads lines of at most the protocol's bound; more without a newline
// is no stream of the wrapper's.
const maxPending = bounds.maxLineBytes * 2;

function connect() {
  let pending = "";
  let ended = false;
  // A character may be split between two reads.
  const decoder = new TextDecoder();
  const end = (why: string) => {
    if (ended) return;
    ended = true;
    lost(why);
  };
  model = new FaceModel();
  stale = true;
  scroll.follow();
  unfold.forget();
  anchor = undefined;
  pointer.clear();
  flash.off();
  progress.reset();
  link = "connecting";
  Bun.connect({
    unix: path,
    socket: {
      open(s) {
        socket = s;
      },
      data(s, chunk) {
        pending += decoder.decode(chunk, { stream: true });
        let at = pending.indexOf("\n");
        let changed = false;
        try {
          while (at >= 0) {
            const raw = pending.slice(0, at);
            pending = pending.slice(at + 1);
            if (raw) {
              const line = JSON.parse(raw) as Line;
              if (line.op === "answer")
                post({
                  kind: "answer",
                  id: line.id ?? "",
                  ok: line.ok === true,
                  detail: line.detail,
                });
              else {
                const was = model.turn;
                changed = model.apply(line) || changed;
                if (line.op === "turn") turned(was, model.turn);
              }
              if (line.op === "end") link = "live";
            }
            at = pending.indexOf("\n");
          }
          if (pending.length > maxPending) throw new Error("a line past the protocol's bound");
        } catch (err) {
          s.end();
          return end(`the mirror's stream does not read: ${String(err)}`);
        }
        if (changed) soon();
      },
      close() {
        end("");
      },
      error(_s, err) {
        end(`the mirror's socket failed: ${String(err)}`);
      },
    },
  }).catch((err) => end(`the mirror's socket does not open: ${String(err)}`));
}

// escaped is the person's Esc: a selection goes first, as the terminal
// program's does; then a working turn stops, as the harness's own Esc stops
// it, where the session takes an interrupt. An idle session hears nothing.
function escaped(id: string) {
  if (pointer.selection) {
    pointer.clear();
    return paint();
  }
  if (model.turn !== "working" || !model.inputs.includes("interrupt") || !socket) return;
  socket.write(`${JSON.stringify({ id, kind: "interrupt", text: "" })}\n`);
}

self.onmessage = (event: MessageEvent<ToFace>) => {
  const message = event.data;
  try {
    switch (message.kind) {
      case "start":
        // A shell of another contract would misread what this face posts.
        if (message.contract !== contract)
          return post({
            kind: "error",
            text: `this face speaks contract ${contract}, the shell ${message.contract}`,
          });
        path = message.path;
        width = message.width;
        height = message.height;
        connect();
        soon();
        return;
      case "size":
        width = message.width;
        height = message.height;
        return soon();
      case "scroll":
        return scrolled(message.by);
      case "wheel": {
        const moved = wheel.rows(message.up ? 1 : -1, message.at);
        return scrolled(message.up ? moved : -moved);
      }
      case "prompt": {
        if (!socket)
          return post({
            kind: "answer",
            id: message.id,
            ok: false,
            detail: "the mirror is not connected",
          });
        socket.write(`${JSON.stringify({ id: message.id, kind: "prompt", text: message.text })}\n`);
        scroll.follow();
        return soon();
      }
      case "mouse":
        return pointed(message.type, message.x, message.y, message.focused);
      case "unselect":
        if (!pointer.selection) return;
        pointer.clear();
        return paint();
      case "escape":
        return escaped(message.id);
      case "unfold":
        return unfolded(message.all);
      case "ping":
        return post({ kind: "pong", n: message.n });
    }
  } catch (err) {
    post({ kind: "error", text: `the face failed on ${message.kind}: ${String(err)}` });
  }
};
