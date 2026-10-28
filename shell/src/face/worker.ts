// rewake's face for one session, run as a Bun Worker the shell starts and may
// end at any moment (docs/face.md, "Hot reload"): it connects to the session's
// mirror socket itself, keeps its copy of the model and posts the lines to
// draw. Everything it does is caught here and posted as an error, so a face
// that breaks leaves its last lines on the screen and the shell untouched.
import type { FromFace, Link, ToFace } from "./messages.ts";
import type { Line } from "./mirror.gen.ts";
import { bounds } from "./mirror.gen.ts";
import { FaceModel } from "./model.ts";
import { render } from "./render.ts";

declare const self: Worker;

const post = (message: FromFace) => self.postMessage(message);
// A redraw waits this long for more changes: a stream of deltas is drawn at
// most thirty times a second.
const settle = 33;
const reconnectAfter = 1000;

let path = "";
let width = 80;
let height = 24;
let back = 0;
let link: Link = "connecting";
let model = new FaceModel();
let socket: { write(data: string): number; end(): void } | undefined;
let drawTimer: ReturnType<typeof setTimeout> | undefined;

function draw() {
  drawTimer = undefined;
  try {
    const lines = render(model, width, height, back);
    post({
      kind: "lines",
      lines,
      turn: model.turn,
      link,
      prompts: model.inputs.includes("prompt"),
    });
  } catch (err) {
    post({ kind: "error", text: `the face's drawing failed: ${String(err)}` });
  }
}

function soon() {
  drawTimer ??= setTimeout(draw, settle);
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
              else changed = model.apply(line) || changed;
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
        back = Math.max(0, back + message.by);
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
        back = 0;
        return soon();
      }
      case "ping":
        return post({ kind: "pong", n: message.n });
    }
  } catch (err) {
    post({ kind: "error", text: `the face failed on ${message.kind}: ${String(err)}` });
  }
};
