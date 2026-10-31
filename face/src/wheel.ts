// How many rows a wheel's notch moves the face (docs/face.md, "The scroll"):
// as many as the terminal program's own fullscreen view moves for the same
// notch, so the face and a native pane scroll alike under one hand. Measured
// live and read from its window curve (Claude Code 2.1.310, no Windows
// Terminal, no scroll speed set): a notch alone moves one row; notches closer
// than 40 ms ramp by 0.3 a notch up to 6; the first notch against the way the
// view moved is held, in case it was the wheel's bounce, and one more the
// same way turns it; a notch back the old way within 200 ms makes it a
// wheel's, whose notches ramp faster and decay with time.

// The rows of a notch alone.
const base = 1;
// Notches closer than this ramp.
const rampWithin = 40;
const rampBy = 0.3;
const rampTo = 6;
// A reversal followed within this by a notch back the old way was a bounce.
const bounceWithin = 200;
// A wheel's ramp: it halves every 150 ms, gains 15 a notch at most 3 at a time,
// up to 15; a pause of 1.5 s, or five notches under 5 ms apart, ends it.
const halfLife = 150;
const wheelGain = 15;
const wheelStep = 3;
const wheelTo = 15;
const wheelIdle = 1500;
const burstUnder = 5;
const burstEnds = 5;

export class Wheel {
  private at = 0;
  // way is the way the view last moved, 1 up, -1 down, 0 not yet.
  private way = 0;
  private mult = base;
  private held = false;
  private wheel = false;
  private burst = 0;

  // rows is how far one notch the given way moves the view at a moment.
  rows(way: 1 | -1, now: number): number {
    if (this.wheel && now - this.at > wheelIdle) this.calm();
    if (this.held) {
      this.held = false;
      if (way !== this.way || now - this.at > bounceWithin) {
        this.way = way;
        this.at = now;
        this.mult = base;
        return base;
      }
      this.wheel = true;
    }
    const gap = now - this.at;
    if (way !== this.way && this.way !== 0) {
      this.held = true;
      this.at = now;
      return 0;
    }
    this.way = way;
    this.at = now;
    if (this.wheel) {
      if (gap >= burstUnder) this.burst = 0;
      else if (++this.burst >= burstEnds) this.calm();
      else return 1;
    }
    if (this.wheel) {
      const kept = 0.5 ** (gap / halfLife);
      this.mult = Math.min(
        wheelTo,
        1 + (this.mult - 1) * kept + wheelGain * kept,
        this.mult + wheelStep,
      );
    } else this.mult = gap > rampWithin ? base : Math.min(rampTo, this.mult + rampBy);
    return Math.max(1, Math.floor(this.mult));
  }

  private calm() {
    this.wheel = false;
    this.burst = 0;
    this.mult = base;
  }
}
