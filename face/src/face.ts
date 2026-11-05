// rewake's face for one session as a module (docs/face.md, "Hot reload"): it
// takes what the shell sends, keeps its copy of the model from the session's
// mirror socket and posts the lines to draw. Its post, its socket and its
// clock are handed in, so worker.ts only wires a Bun Worker to it and a test
// drives it in process. Everything it does is caught and posted as an error,
// so a face that breaks leaves its last lines on the screen and the shell
// untouched.

import { Edge, edgeRows } from "./edge.ts";
import { type Connect, MirrorLink } from "./link.ts";
import {
  contract,
  type FaceLine,
  type FromFace,
  type Link,
  type Pointing,
  refusal,
  type ToFace,
} from "./messages.ts";
import type { InputKind, Line } from "./mirror.gen.ts";
import { FaceModel } from "./model.ts";
import { type Anchor, anchorAt, clickOn, lit, topFor } from "./parts.ts";
import { Flash, Pointer, pointAt, type View } from "./pointer.ts";
import { Tracker } from "./progress.ts";
import { blinked, bodyOf, fill, pinned, statusOf } from "./render.ts";
import { Scroll } from "./scroll.ts";
import { copyOf, highlighted } from "./select.ts";
import { realTimers, Ticker, type Timers } from "./ticker.ts";
import type { Clock } from "./turn.ts";
import { Unfold } from "./unfold.ts";
import { Wheel } from "./wheel.ts";

type Said = Omit<Extract<FromFace, { kind: "lines" }>, "kind" | "lines">;

export class Face {
  private path = "";
  private width = 80;
  private height = 24;
  private readonly scroll = new Scroll();
  private readonly wheel = new Wheel();
  private readonly pointer = new Pointer();
  // laid is the conversation's rows as the last tick laid them out, and stale
  // once it or the width changed: only a tick lays them out again.
  private laid: FaceLine[] = [];
  private stale = true;
  private hideThinking = false;
  // frame is what the last tick committed: every row from the same moment,
  // and what the rows are posted with. A scroll cuts only it, so a change of
  // the conversation waits for its tick.
  private frame: ({ rows: FaceLine[]; now: number } & Said) | undefined;
  private link: Link = "connecting";
  private model = new FaceModel();
  private readonly mirror: MirrorLink;
  // The frame last posted: a tick that draws the same posts nothing.
  private posted = "";
  // progress follows the working turn from tick to tick.
  private readonly progress = new Tracker();
  // began is the start of the working turn the tracker follows, as the
  // model said it.
  private began: number | undefined;
  // unfold is what the person opened, everything by the shell's key or a part
  // by a click; anchor is the row to keep where it stood on the screen once
  // the conversation is laid out again (parts.ts).
  private readonly unfold = new Unfold();
  private anchor: Anchor | undefined;
  // refused is a start of another contract: the face stays as it was told
  // then, drawing nothing, so the shell keeps its last frame and the error;
  // it answers only pings.
  private refused = false;
  private readonly ticker: Ticker;
  // flash is the part a click copied, lit for a moment with the notice.
  private readonly flash: Flash;
  // A drag past the body's edge scrolls the view, the selection's end with it.
  private readonly edge: Edge;

  constructor(
    private readonly post: (message: FromFace) => void,
    connect: Connect,
    private readonly timers: Timers = realTimers,
  ) {
    this.mirror = new MirrorLink(connect, timers, {
      fresh: () => this.fresh(),
      line: (line) => this.heard(line),
      changed: () => this.soon(),
      lost: (why) => this.lost(why),
    });
    this.ticker = new Ticker(
      (now) => this.draw(now),
      () => this.moving(),
      timers,
    );
    this.flash = new Flash(() => this.paint(), timers);
    this.edge = new Edge(
      () => this.height,
      (way) => this.edged(way),
      timers,
    );
  }

  take(message: ToFace) {
    if (this.refused && message.kind !== "ping") return;
    try {
      switch (message.kind) {
        case "start":
          // A shell of another contract would misread what this face posts.
          if (message.contract !== contract) {
            this.refused = true;
            return this.post({ kind: "error", text: refusal(contract, message.contract) });
          }
          this.path = message.path;
          this.width = message.width;
          this.height = message.height;
          this.mirror.start(this.path);
          return this.soon();
        case "size":
          this.width = message.width;
          this.height = message.height;
          return this.soon();
        case "scroll":
          return this.scrolled(message.by);
        case "wheel": {
          const moved = this.wheel.rows(message.up ? 1 : -1, message.at);
          return this.scrolled(message.up ? moved : -moved);
        }
        case "prompt": {
          if (!this.mirror.open)
            return this.post({
              kind: "answer",
              id: message.id,
              ok: false,
              detail: "the mirror is not connected",
            });
          this.mirror.write({ id: message.id, kind: "prompt", text: message.text });
          this.scroll.follow();
          return this.soon();
        }
        case "mouse":
          return this.pointed(message.type, message.x, message.y, message.focused);
        case "unselect":
          if (!this.pointer.selection) return;
          this.pointer.clear();
          return this.paint();
        case "escape":
          return this.escaped(message.id);
        case "unfold":
          return this.unfolded(message.all);
        // Mode and thinking use separate inputs; visibility is local drawing.
        case "workmode":
          return this.asked(message.id, "workmode");
        case "thinkingVisible":
          this.hideThinking = !this.hideThinking;
          this.pointer.clear();
          return this.soon();
        case "thinking":
          return this.asked(message.id, "thinking");
        case "ping":
          return this.post({ kind: "pong", n: message.n });
      }
    } catch (err) {
      this.post({ kind: "error", text: `the face failed on ${message.kind}: ${String(err)}` });
    }
  }

  // turned follows the working turn's progress from the turn's start, which
  // the model carries. A compaction inside the turn leaves it going on: the
  // model keeps its start, and the tracker its own.
  private turned(from: string, to: string) {
    if (from === to || to !== "working") return;
    const since = this.model.since;
    if (from !== "compacting" || since === undefined || since !== this.began)
      this.progress.reset(since ?? this.timers.now(), this.model.items);
    this.began = since;
  }

  private clockAt(now: number): Clock {
    const m = this.model;
    return {
      now,
      since: m.since,
      worked: m.worked,
      progress: m.turn === "working" ? this.progress.at(m.items, now, m.retry) : undefined,
    };
  }

  // draw is the tick's frame: the whole conversation, laid out again when it
  // changed (the full model costs a few ms), and the turn's rows pinned under
  // it at the pane's bottom, every row from the same clock.
  private draw(now: number) {
    try {
      const m = this.model;
      if (this.stale)
        this.laid = bodyOf(m, this.width, Infinity, this.unfold, this.hideThinking).rows;
      this.stale = false;
      if (this.anchor) this.scroll.top = topFor(this.laid, this.anchor) ?? this.scroll.top;
      this.anchor = undefined;
      const rows = pinned(this.laid, statusOf(m, this.clockAt(now), this.width), this.height);
      const prompts = m.inputs.includes("prompt");
      this.frame = {
        rows,
        now,
        turn: m.turn,
        link: this.link,
        prompts,
        mode: m.mode,
        level: m.level,
        workMode: m.workMode,
      };
      this.paint();
    } catch (err) {
      this.post({ kind: "error", text: `the face's drawing failed: ${String(err)}` });
    }
  }

  // viewOf is where the committed frame's rows stand in the face.
  private viewOf(): View {
    const total = this.frame?.rows.length ?? 0;
    return {
      top: this.scroll.start(total, this.height),
      shown: Math.min(total, this.height),
      height: this.height,
      width: this.width,
    };
  }

  // paint posts the view of the committed frame, unless it is what was
  // posted: the selection marked, and the part a click just copied lit.
  private paint() {
    if (!this.frame) return;
    try {
      const { rows, now, ...said } = this.frame;
      const shown = this.scroll.view(rows, this.height);
      const view = this.viewOf();
      const marked = lit(highlighted(shown, view.top, this.pointer.selection), this.flash.part);
      const lines = fill(blinked(marked, now), this.width, this.height);
      const message: FromFace = { kind: "lines", lines, ...said };
      const sent = JSON.stringify(message);
      if (sent === this.posted) return;
      this.posted = sent;
      this.post(message);
    } catch (err) {
      this.post({ kind: "error", text: `the face's drawing failed: ${String(err)}` });
    }
  }

  // What moves with time alone: a working turn's line, a running call's dot.
  private moving() {
    const m = this.model;
    return (
      m.turn === "working" ||
      m.turn === "compacting" ||
      m.items.some((item) => item.status === "running")
    );
  }

  // soon paints a change of the conversation at the next tick.
  private soon() {
    this.stale = true;
    this.ticker.change();
  }

  // scrolled moves the view over the committed frame and paints it at once:
  // a scroll is the person's hand, not a change of the conversation, so it
  // waits for no tick and lays nothing out.
  private scrolled(by: number) {
    if (!this.frame) return;
    this.scroll.by(by, this.frame.rows.length, this.height);
    this.paint();
  }

  private edged(way: number) {
    if (!this.frame) return false;
    this.scroll.by(way * edgeRows, this.frame.rows.length, this.height);
    const { x, y } = this.edge.at;
    this.pointer.drag(pointAt(x, y, this.viewOf()), x, y);
    this.paint();
    return true;
  }

  // pin keeps a row of the committed frame where it stands in the view once
  // the conversation is laid out again, by the part it is a row of.
  private pin(row: number) {
    if (this.frame) this.anchor = anchorAt(this.frame.rows, row, this.viewOf().top);
  }

  // unfolded opens or closes everything; a view scrolled up keeps the first
  // row of a part it shows where it was, one at the bottom stays there. A
  // selection is held by the rows' places, which this moves: it is dropped.
  private unfolded(all: boolean) {
    if (all === this.unfold.all) return;
    this.unfold.every(all);
    this.pointer.clear();
    if (this.frame && this.scroll.top !== undefined) {
      const { top, shown } = this.viewOf();
      const row = this.frame.rows.findIndex((r, i) => i >= top && i < top + shown && r.part);
      if (row >= 0) this.pin(row);
    }
    this.soon();
  }

  // pointed takes the person's hand: a release copies the selection it made,
  // or does what a click on its row does (parts.ts): copies the part, or
  // opens or closes it, the row staying under the pointer.
  private pointed(type: Pointing, x: number, y: number, focused: boolean) {
    const frame = this.frame;
    if (!frame) return;
    const at = pointAt(x, y, this.viewOf());
    switch (type) {
      case "down":
        this.edge.stop();
        this.flash.off();
        this.pointer.down(this.viewOf(), x, y, focused);
        break;
      case "drag":
        this.edge.at = { x, y };
        if (this.pointer.drag(at, x, y)) this.edge.follow();
        break;
      case "up": {
        this.edge.stop();
        const gesture = this.pointer.up();
        if (gesture && "selected" in gesture) {
          const text = copyOf(frame.rows, gesture.selected);
          if (text.trim()) this.post({ kind: "copy", text, what: "selection" });
        } else if (gesture) {
          const click = clickOn(this.model.items, frame.rows, gesture.clicked, this.viewOf().top);
          if (click && "toggle" in click) {
            this.pointer.clear();
            this.unfold.toggle(click.toggle);
            this.anchor = click.anchor;
            this.soon();
            break;
          }
          if (click) {
            this.post({ kind: "copy", text: click.copy.text, what: click.copy.what });
            this.flash.on(click.light);
          }
        }
        break;
      }
    }
    this.paint();
  }

  // fresh starts over as the link connects: a new model from the mirror's
  // snapshot, the view at the bottom with nothing opened, selected or lit.
  private fresh() {
    this.model = new FaceModel();
    this.stale = true;
    this.scroll.follow();
    this.unfold.forget();
    this.anchor = undefined;
    this.pointer.clear();
    this.flash.off();
    this.progress.reset();
    this.link = "connecting";
  }

  // heard takes a line of the mirror's: an answer to an input goes to the
  // shell, anything else to the model; it says whether what is drawn changed.
  private heard(line: Line) {
    let changed = false;
    if (line.op === "answer")
      this.post({ kind: "answer", id: line.id ?? "", ok: line.ok === true, detail: line.detail });
    else {
      const was = this.model.turn;
      changed = this.model.apply(line);
      if (line.op === "turn") this.turned(was, this.model.turn);
      if (line.op === "begin") this.began = this.model.since;
    }
    if (line.op === "end") this.link = "live";
    return changed;
  }

  private lost(why: string) {
    this.link = "lost";
    this.soon();
    if (why) this.post({ kind: "error", text: why });
  }

  // asked sends the session an input with no text, where it takes that kind.
  private asked(id: string, kind: InputKind) {
    if (this.model.inputs.includes(kind)) this.mirror.write({ id, kind, text: "" });
  }

  // escaped is the person's Esc: a selection goes first, as the terminal
  // program's does; then a working turn stops, as the harness's own Esc
  // stops it, where the session takes an interrupt. An idle session hears
  // nothing.
  private escaped(id: string) {
    if (this.pointer.selection) {
      this.pointer.clear();
      return this.paint();
    }
    if (this.model.turn === "working") this.asked(id, "interrupt");
  }
}
