// The face's one clock (docs/face.md, "One clock"): a change only marks the
// face dirty, and the next tick paints everything the face shows at once —
// the stream's text, a call's start and end, the glyph, a
// running dot's blink and the turn's time, all from that tick's one now. Seen
// live before it: each painted on its own moment, the glyph on the wall
// clock's second, the time on the turn's, a delta 33 ms after it came.

// tickEvery is the terminal program's own animation frame, 20 a second.
export const tickEvery = 50;

type Timers = {
  every: (run: () => void, ms: number) => unknown;
  stop: (timer: unknown) => void;
  now: () => number;
};

const real: Timers = {
  every: (run, ms) => setInterval(run, ms),
  stop: (timer) => clearInterval(timer as ReturnType<typeof setInterval>),
  now: () => Date.now(),
};

export class Ticker {
  private dirty = false;
  private timer: unknown;

  // paint draws the face at a moment; moving says whether something on it
  // moves with time alone, so the clock goes on without a change.
  constructor(
    private readonly paint: (now: number) => void,
    private readonly moving: () => boolean,
    private readonly timers: Timers = real,
  ) {}

  // change marks the face to paint at the next tick, starting the clock when
  // it stood.
  change() {
    this.dirty = true;
    this.timer ??= this.timers.every(() => this.tick(), tickEvery);
  }

  // tick paints when anything changed or moves, and stops the clock when
  // nothing does.
  tick() {
    if (!this.dirty && !this.moving()) return this.halt();
    this.dirty = false;
    this.paint(this.timers.now());
  }

  halt() {
    if (this.timer !== undefined) this.timers.stop(this.timer);
    this.timer = undefined;
  }
}
