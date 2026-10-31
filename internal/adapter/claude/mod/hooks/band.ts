// The session's name and context, added to the hint line under Claude Code's
// input, the line the face draws as the input's rule (docs/face.md, "The
// look"; shell/src/face/rule.ts), so a native pane and a face pane tell the
// same. The terminal program keeps its own line and its pills and cuts our
// tail first where the row ends. It only informs: its code answers no verdict
// and no prompt, and a hook of it that fails is dropped with the tail by the
// engine (docs/claude.md, "The band").
import type { Register } from "claude-code";

// kilo is a count as the strip writes it: 950, 46k, 1M, 1.5M.
export function kilo(n: number): string {
  if (n >= 1_000_000) return `${Math.round(n / 100_000) / 10}M`;
  if (n >= 1000) return `${Math.round(n / 1000)}k`;
  return String(n);
}

// contextOf is the context as the face's rule shows it: 46k/272k, and –/272k
// before the harness counted it; nothing without a window.
export function contextOf(tokens: number | undefined, window: number | undefined) {
  if (!window) return undefined;
  return `${tokens === undefined ? "–" : kilo(tokens)}/${kilo(window)}`;
}

// The session's facts, set by the hooks that hear them; the render hook only
// reads them, so it waits for nothing.
export class Band {
  private name?: string;
  private tokens?: number;
  private window?: number;
  private limit = 0;
  private redraw: () => void = () => {};

  // begin makes the band live for a rewake run: name is the session's.
  // limit is the window the launch limited the conversation to, in tokens, 0
  // for none: the figure shown is measured against it where it is below the
  // model's window, as the strip's is (observer.go).
  begin(name: string | undefined, redraw: () => void, limit?: string) {
    this.name = name || undefined;
    const held = Number(limit);
    this.limit = Number.isFinite(held) && held > 0 ? held : 0;
    this.redraw = redraw;
    this.redraw();
  }

  // measured takes the context the harness counted.
  measured(tokens: number | undefined, window: number | undefined) {
    if (this.limit > 0 && (!window || this.limit < window)) window = this.limit;
    if (tokens === this.tokens && window === this.window) return;
    this.tokens = tokens;
    this.window = window;
    this.redraw();
  }

  // tail is what the band adds to the hint line: the name, and the context
  // where there is a window; nothing outside a rewake run.
  get tail(): string | undefined {
    if (!this.name) return undefined;
    const context = contextOf(this.tokens, this.window);
    return context ? `${this.name} · ${context}` : this.name;
  }
}

// serveBand adds the band to the hint line; its facts are told by sample.
export function serveBand(on: Parameters<Register>[0], band: Band) {
  on("ui.render", { component: "PromptHint" }, ($, e, next) => {
    const tail = band.tail;
    if (!tail) return next(e);
    return next({ ...e, props: { ...e.props, tail } });
  });
}

// sample reads the context the harness counted now, for the band.
export async function sample(
  usage: () => Promise<{ context?: { tokens?: number; window?: number } }>,
  band: Band,
) {
  try {
    const { context } = await usage();
    band.measured(context?.tokens, context?.window);
  } catch {
    // The band keeps the last figures.
  }
}
