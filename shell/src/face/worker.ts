// rewake's face for one session, run as a Bun Worker the shell starts and may
// end at any moment (docs/face.md, "Hot reload"): it connects to the session's
// mirror socket itself, keeps its copy of the model and posts the lines to
// draw. Everything it does is caught here and posted as an error, so a face
// that breaks leaves its last lines on the screen and the shell untouched.
import type { FromFace, Link, ToFace } from "./messages.ts";
import type { Line } from "./mirror.gen.ts";
import { bounds } from "./mirror.gen.ts";
import { FaceModel } from "./model.ts";
import { blinked, fill, render, rowsOf } from "./render.ts";
import { Scroll } from "./scroll.ts";
import { Ticker } from "./ticker.ts";
import type { Clock } from "./turn.ts";

declare const self: Worker;

const post = (message: FromFace) => self.postMessage(message);
const reconnectAfter = 1000;

let path = "";
let width = 80;
let height = 24;
const scroll = new Scroll();
let link: Link = "connecting";
let model = new FaceModel();
let socket: { write(data: string): number; end(): void } | undefined;
// The frame last posted: a tick that draws the same posts nothing.
let posted = "";
// The turn's time as this face saw it, never from the model: a face that
// connected mid-turn does not know when it began.
let since: number | undefined;
let worked: { took: number; at: number } | undefined;

// turned keeps the clock as the turn changes.
function turned(from: string, to: string) {
  if (from === to) return;
  if (to === "working") {
    since = Date.now();
    worked = undefined;
  } else if (from === "working" && since !== undefined && to === "idle") {
    worked = { took: Date.now() - since, at: Date.now() };
    since = undefined;
  } else since = undefined;
}

const clockAt = (now: number): Clock => ({ now, since, worked });

// draw is one frame of the face at a moment, every row of it from the same
// clock.
function draw(now: number) {
  try {
    const clock = clockAt(now);
    const shown =
      scroll.top === undefined
        ? render(model, width, height, 0, clock)
        : scroll.view(rowsOf(model, width, clock), height);
    const message: FromFace = {
      kind: "lines",
      lines: fill(blinked(shown, now), width, height),
      turn: model.turn,
      link,
      prompts: model.inputs.includes("prompt"),
    };
    const frame = JSON.stringify(message);
    if (frame === posted) return;
    posted = frame;
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
const soon = () => ticker.change();

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
  scroll.follow();
  since = worked = undefined;
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
        scroll.by(message.by, rowsOf(model, width, clockAt(Date.now())).length, height);
        return soon();
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
      case "ping":
        return post({ kind: "pong", n: message.n });
    }
  } catch (err) {
    post({ kind: "error", text: `the face failed on ${message.kind}: ${String(err)}` });
  }
};
