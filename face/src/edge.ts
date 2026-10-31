// A drag past the view's top or bottom scrolls it on a clock of its own,
// two rows every 50 ms for at most ten seconds, as the native pane does, the
// selection's end on the edge's row (docs/face.md, "Copying").
const edgeEvery = 50;
const edgeTicks = 200;
export const edgeRows = 2;

export class Edge {
  // at is where the drag's pointer last was, in the view's cells.
  at = { x: 0, y: 0 };
  private timer: ReturnType<typeof setInterval> | undefined;
  private ticks = 0;

  // step scrolls a tick's rows the way given, 1 up and -1 down, and says
  // whether the drag can go on.
  constructor(
    private readonly height: () => number,
    private readonly step: (way: number) => boolean,
  ) {}

  private way() {
    return this.at.y < 0 ? 1 : this.at.y >= this.height() ? -1 : 0;
  }

  stop() {
    clearInterval(this.timer);
    this.timer = undefined;
  }

  // follow starts the clock while the pointer is past an edge, and stops it
  // once it is back.
  follow() {
    if (this.way() === 0) return this.stop();
    if (this.timer) return;
    this.ticks = 0;
    this.timer = setInterval(() => {
      const way = this.way();
      if (++this.ticks > edgeTicks || way === 0 || !this.step(way)) this.stop();
    }, edgeEvery);
  }
}
