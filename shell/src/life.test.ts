import { expect, test } from "bun:test";
import type { Clock } from "./clock.ts";
import { type Kept, type Screen, shellLife } from "./life.ts";
import type { FromHost, ToHost } from "./link.ts";
import type { PaneTerminal } from "./pane.ts";
import type { Pane } from "./panes.ts";
import { Scanner } from "./scan.ts";

// A stream from a host, as the life asked for it.
type Stream = {
  socket: string;
  size: { cols: number; rows: number };
  data(bytes: string): void;
  end(): void;
  written: string[];
  closed: boolean;
};

// world is a life on a channel, streams, a screen and a clock of the test's
// own; log is what the life did to them, in order.
function world() {
  const log: string[] = [];
  const sent: ToHost[] = [];
  let toShell: (m: FromHost) => void = () => {};
  let channelEnded: (why: string) => void = () => {};
  let now = 0;
  const timers: { at: number; fn: () => void; gone: boolean }[] = [];
  const clock: Clock = {
    now: () => now,
    after(ms, fn) {
      const timer = { at: now + ms, fn, gone: false };
      timers.push(timer);
      return () => {
        timer.gone = true;
      };
    },
  };
  const streams: Stream[] = [];
  const over = Promise.withResolvers<void>();
  const overAsked: string[][] = [];
  const pane = (id: string): Pane => ({ id, label: id, scanner: new Scanner(), title: "" });
  const main = pane("main");
  const workers: Pane[] = [];
  const life = shellLife({
    link(onMessage, onEnd) {
      toShell = onMessage;
      channelEnded = onEnd;
      return {
        send(message) {
          sent.push(message);
          log.push(`sent ${message.t}`);
        },
      };
    },
    connect(socket, size, onData, onEnd) {
      const stream: Stream = {
        socket,
        size,
        data: (bytes) => onData(new TextEncoder().encode(bytes)),
        end: onEnd,
        written: [],
        closed: false,
      };
      streams.push(stream);
      return {
        write: (data) => stream.written.push(new TextDecoder().decode(data)),
        resize() {},
        close() {
          stream.closed = true;
          log.push(`closed ${socket}`);
        },
      };
    },
    main,
    workers: () => workers,
    kept: () => ({ focused: "w1", chosen: "w1", zoomed: true }),
    restore: (kept) => log.push(`restored ${kept.focused}`),
    spacesOver(ids) {
      overAsked.push(ids);
      return over.promise;
    },
    endFaces: () => log.push("faces ended"),
    reloadFaces: () => log.push("faces reloaded"),
    say: (line) => log.push(`said ${line}`),
    exit: (code) => log.push(`exit ${code}`),
    clock,
  });
  const closed = Promise.withResolvers<void>();
  const screen: Screen = {
    destroy: () => log.push("screen destroyed"),
    closed: closed.promise,
    copy: () => {},
    title: () => {},
  };
  // tick moves the clock, running what came due, each followed by what it
  // set going.
  const tick = async (ms: number) => {
    now += ms;
    for (;;) {
      const due = timers.filter((t) => !t.gone && t.at <= now).sort((a, b) => a.at - b.at)[0];
      if (!due) break;
      due.gone = true;
      due.fn();
      await settle();
    }
    await settle();
  };
  const term = () => {
    const written: string[] = [];
    const t = { width: 10, height: 3, write: (s: string) => written.push(s), written };
    return t as unknown as PaneTerminal & { written: string[] };
  };
  return {
    life,
    log,
    sent,
    streams,
    screen,
    closed,
    over,
    overAsked,
    main,
    workers,
    pane,
    term,
    tick,
    host: (m: FromHost) => toShell(m),
    channelEnded: (why: string) => channelEnded(why),
  };
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 0));

// started is a world whose shell was given the terminal, draws, and shows
// main's pane attached to main's host.
async function started(state?: Kept) {
  const w = world();
  w.host({ t: "welcome", generation: 1, why: "first" });
  w.host({ t: "go", state });
  const given = await w.life.given;
  w.life.drawing(w.screen);
  w.life.seeded({ widget: "W", host: "fg;bg" });
  const term = w.term();
  w.life.startMain(term);
  w.host({ t: "main", main: { pid: 7, socket: "/host.sock" } });
  return { ...w, given, mainTerm: term };
}

// The first launch's order (docs/shell.md, "The host's channel"): the host
// starts main at the size and colours of the shell's ready, and main's pane
// attaches only once main is registered.
test("the shell answers ready from main's pane, and attaches it once main is registered", async () => {
  const w = world();
  w.host({ t: "welcome", generation: 1, why: "first" });
  w.host({ t: "go" });
  expect(await w.life.given).toBeUndefined();
  w.life.drawing(w.screen);
  w.life.seeded({ widget: "W", host: "fg;bg" });
  const term = w.term();
  w.life.startMain(term);
  expect(w.sent).toEqual([{ t: "ready", colors: "fg;bg", cols: 20, rows: 5 }]);
  expect(w.streams).toHaveLength(0);
  w.host({ t: "main", main: { pid: 7, socket: "/host.sock" } });
  expect(w.streams.map((s) => [s.socket, s.size])).toEqual([["/host.sock", { cols: 20, rows: 5 }]]);
  expect(w.life.mainPid()).toBe(7);
  expect(term.written).toEqual(["W", "\x1bcW"]);
});

// A stream that ends while its pane shows — main's host switching its build
// — is attached again from a fresh preamble; none once the shell ends.
test("a pane whose stream ended attaches again after a second, and not once the shell ends", async () => {
  const w = await started();
  w.streams[0]?.end();
  await w.tick(999);
  expect(w.streams).toHaveLength(1);
  await w.tick(1);
  expect(w.streams).toHaveLength(2);
  w.host({ t: "closing" });
  w.streams[1]?.end();
  await w.tick(1000);
  expect(w.streams).toHaveLength(2);
});

// A restart's release (85ffc35, 24388c7): the screen is the person's again
// before the state goes, and the shell exits the moment it is written; no
// stream is attached again meanwhile.
test("a release gives the screen back, then hands the person's choice over and exits", async () => {
  const w = await started({ focused: "w1", chosen: "w1" });
  expect(w.given?.chosen).toBe("w1");
  w.log.length = 0;
  w.host({ t: "release" });
  await w.tick(0);
  expect(w.log).toEqual(["faces ended", "closed /host.sock", "screen destroyed"]);
  w.closed.resolve();
  await settle();
  expect(w.log.slice(3)).toEqual(["sent released", "exit 0"]);
  expect(w.sent.at(-1)).toEqual({
    t: "released",
    state: { focused: "w1", chosen: "w1", zoomed: true },
  });
});

test("what the last shell handed over is restored once", async () => {
  const w = await started({ focused: "w1" });
  w.life.restore();
  w.life.restore();
  expect(w.log.filter((l) => l.startsWith("restored"))).toEqual(["restored w1"]);
});

// Main's end (docs/shell.md, "The closing screen"): the layer stays until
// the workers the shell showed are over and its minimum time has passed,
// and only then is the screen given back and the shell exits with main's
// code.
test("main's end keeps the closing layer until its workers are over, then exits with main's code", async () => {
  const w = await started();
  w.workers.push(w.pane("w1"));
  w.host({ t: "main-ended", code: 3 });
  expect(w.life.closing()).toBe("sure");
  expect(w.overAsked).toEqual([["w1"]]);
  await w.tick(600);
  expect(w.log).not.toContain("screen destroyed");
  w.over.resolve();
  await settle();
  w.closed.resolve();
  await settle();
  expect(w.log.slice(-2)).toEqual(["screen destroyed", "exit 3"]);
});

// A channel that ends leaves the shell, saying why; a screen that does not
// close is waited for a second at most.
test("the host's channel ending leaves with 1, within a second of the screen's destroy", async () => {
  const w = await started();
  w.channelEnded("the host's channel ended");
  await settle();
  expect(w.log).toContain("said rewake-shell: the host's channel ended");
  expect(w.log).not.toContain("exit 1");
  await w.tick(1000);
  expect(w.log.slice(-1)).toEqual(["exit 1"]);
});

// A leave of main's screen begins the closing layer, unless it follows the
// person's Ctrl+Z within a second: Claude Code leaves its screen at a
// suspend too, and is continued at once.
test("main leaving its screen right after a Ctrl+Z begins no closing; later it does", async () => {
  const w = await started();
  const stream = w.streams[0];
  w.mainTerm.onData?.(new TextEncoder().encode("\x1a"), "input");
  expect(stream?.written).toEqual(["\x1a"]);
  stream?.data("\x1b[?1049hMAIN\x1b[?1049l");
  expect(w.life.closing()).toBe("");
  await w.tick(1001);
  stream?.data("\x1b[?1049hMAIN\x1b[?1049l");
  expect(w.life.closing()).toBe("left");
  stream?.data("\x1b[?1049hMAIN");
  expect(w.life.closing()).toBe("");
});

// The terminal's end (ending.ts): a stop signal asks the host to close, and
// the shell stays to show the closing; a terminal lost is told, and the
// shell exits with nothing to draw on.
test("a stop signal asks the host to close; a lost terminal is told and the shell exits", async () => {
  const w = await started();
  w.life.terminalGone(false);
  expect(w.sent.at(-1)).toEqual({ t: "close" });
  expect(w.log).not.toContain("exit 1");
  w.life.terminalGone(true);
  expect(w.log.slice(-2)).toEqual(["sent lost", "exit 1"]);
});

// rewake restart face is answered, so the door does not wait out its bound.
test("a reload of the face reloads every face and answers", async () => {
  const w = await started();
  w.host({ t: "reload", part: "face" });
  expect(w.log.slice(-2)).toEqual(["faces reloaded", "sent reloaded"]);
  expect(w.sent.at(-1)).toEqual({ t: "reloaded", part: "face" });
});
