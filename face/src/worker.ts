// rewake's face for one session, run as a Bun Worker the shell starts and may
// end at any moment (docs/face.md, "Hot reload"): it connects to the session's
// mirror socket itself, keeps its copy of the model and posts the lines to
// draw. Everything it does is caught here and posted as an error, so a face
// that breaks leaves its last lines on the screen and the shell untouched.
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
import { Pointer, pointAt, type View } from "./pointer.ts";
import { Tracker } from "./progress.ts";
import { blinked, bodyOf, fill, statusOf } from "./render.ts";
import { Scroll } from "./scroll.ts";
import { copyOf, highlighted, lit } from "./select.ts";
import { Ticker } from "./ticker.ts";
import type { Clock } from "./turn.ts";
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
// The turn's time as this face saw it, never from the model: a face that
// connected mid-turn does not know when it began.
let since: number | undefined;
let worked: { took: number; at: number } | undefined;
// progress follows the working turn from tick to tick.
const progress = new Tracker();

// turned keeps the clock as the turn changes.
function turned(from: string, to: string) {
  if (from === to) return;
  if (to === "working") {
    since = Date.now();
    worked = undefined;
    progress.reset(since, model.items);
  } else if (from === "working" && since !== undefined && to === "idle") {
    worked = { took: Date.now() - since, at: Date.now() };
    since = undefined;
  } else since = undefined;
}

const clockAt = (now: number): Clock => ({
  now,
  since,
  worked,
  progress: model.turn === "working" ? progress.at(model.items, now) : undefined,
});

// draw is the tick's frame: the whole conversation, laid out again when it
// changed (the full model costs a few ms), and the turn's rows under it,
// every row from the same clock.
function draw(now: number) {
  try {
    if (stale) laid = bodyOf(model, width).rows;
    stale = false;
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

// flash is the part a click copied, lit for a moment with the notice so the
// person sees what went; nothing lights as the pointer moves.
const flashFor = 1000;
let flash: { part: string; timer: ReturnType<typeof setTimeout> } | undefined;

function unflash() {
  clearTimeout(flash?.timer);
  flash = undefined;
}

function flashed(part: string) {
  unflash();
  flash = {
    part,
    timer: setTimeout(() => {
      flash = undefined;
      paint();
    }, flashFor),
  };
}

// paint posts the view of the committed frame, unless it is what was posted:
// the selection marked, and the part a click just copied lit.
function paint() {
  if (!frame) return;
  try {
    const shown = scroll.view(frame.rows, height);
    const view = viewOf();
    const marked = lit(highlighted(shown, view.top, pointer.selection), flash?.part);
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

// A drag past the view's top or bottom scrolls it on a clock of its own,
// two rows every 50 ms for at most ten seconds, as the native pane does, the
// selection's end on the edge's row.
const edgeRows = 2;
const edgeEvery = 50;
const edgeTicks = 200;
let edge: { timer: ReturnType<typeof setInterval>; ticks: number } | undefined;
let dragged = { x: 0, y: 0 };

function stopEdge() {
  clearInterval(edge?.timer);
  edge = undefined;
}

function followEdge() {
  const away = dragged.y < 0 ? 1 : dragged.y >= height ? -1 : 0;
  if (away === 0) return stopEdge();
  if (edge) return;
  edge = {
    ticks: 0,
    timer: setInterval(() => {
      if (!frame || !edge || ++edge.ticks > edgeTicks) return stopEdge();
      const way = dragged.y < 0 ? 1 : dragged.y >= height ? -1 : 0;
      if (way === 0) return stopEdge();
      scroll.by(way * edgeRows, frame.rows.length, height);
      pointer.drag(pointAt(dragged.x, dragged.y, viewOf()), dragged.x, dragged.y);
      paint();
    }, edgeEvery),
  };
}

// pointed takes the person's hand: a release copies the selection it made,
// or the part a click was on.
function pointed(type: Pointing, x: number, y: number, focused: boolean) {
  if (!frame) return;
  const at = pointAt(x, y, viewOf());
  switch (type) {
    case "down":
      stopEdge();
      unflash();
      pointer.down(viewOf(), x, y, focused);
      break;
    case "drag":
      dragged = { x, y };
      if (pointer.drag(at, x, y)) followEdge();
      break;
    case "up": {
      stopEdge();
      const gesture = pointer.up();
      if (gesture && "selected" in gesture) {
        const text = copyOf(frame.rows, gesture.selected);
        if (text.trim()) post({ kind: "copy", text, what: "selection" });
      } else if (gesture) {
        const key = frame.rows[gesture.clicked]?.part;
        const part = key === undefined ? undefined : partOf(model.items, key);
        if (key !== undefined && part) {
          post({ kind: "copy", text: part.text, what: part.what });
          flashed(key);
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
  pointer.clear();
  unflash();
  since = worked = undefined;
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
      case "ping":
        return post({ kind: "pong", n: message.n });
    }
  } catch (err) {
    post({ kind: "error", text: `the face failed on ${message.kind}: ${String(err)}` });
  }
};
