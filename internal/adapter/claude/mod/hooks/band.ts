// The run's room work, added to the hint line under Claude Code's input, the
// line the face draws as the input's rule (docs/face.md, "The look";
// shell/src/face/rule.ts), so a native pane and a face pane tell the same:
// the briefs it was given with their ticks and the reports it waits on, as
// the host's heartbeat answers them (internal/core/work). The terminal
// program keeps its own line and its pills and cuts our tail first where the
// row ends, so nothing is only here: the briefs view and the commands say the
// same. It only informs: its code answers no verdict and no prompt, and a
// hook of it that fails is dropped with the tail by the engine (docs/claude.md,
// "The band").
import type { Register } from "claude-code";

// The run's work, set as the heartbeat hears it; the render hook only reads
// it, so it waits for nothing.
export class Band {
  private live = false;
  private text = "";
  private redraw: () => void = () => {};

  // begin makes the band live for a rewake run.
  begin(redraw: () => void) {
    this.live = true;
    this.redraw = redraw;
  }

  // told takes the work a heartbeat answered with; an answer without one
  // leaves the last.
  told(work: string | undefined) {
    if (!this.live || work === undefined || work === this.text) return;
    this.text = work;
    this.redraw();
  }

  // shown is the work the band shows, which each poll names so the host
  // answers a change at once; undefined outside a rewake run.
  get shown(): string | undefined {
    return this.live ? this.text : undefined;
  }

  // tail is what the band adds to the hint line: the work, nothing when
  // there is none or outside a rewake run.
  get tail(): string | undefined {
    return this.live && this.text ? this.text : undefined;
  }
}

// serveBand adds the band to the hint line; its work is told by the heartbeat.
export function serveBand(on: Parameters<Register>[0], band: Band) {
  on("ui.render", { component: "PromptHint" }, ($, e, next) => {
    const tail = band.tail;
    if (!tail) return next(e);
    return next({ ...e, props: { ...e.props, tail } });
  });
}
