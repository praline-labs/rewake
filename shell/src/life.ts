// The shell's life (docs/shell.md, "The host's channel" and "Restart, crash
// and close"): the channel with main's host from the hello to the shell's
// exit, and each pane's stream from its host. What it acts on comes from
// outside — the channel, the streams, the screen once it draws, the clock,
// the panes, exit — so the order between them is tested as it runs
// (life.test.ts), and main.tsx only wires it.

import { type Clock, realClock } from "./clock.ts";
import type { KeptFaces } from "./face/faces.ts";
import { isSuspend } from "./keys.ts";
import { closingLayer } from "./lifting.ts";
import type { FromHost, Link, MainRef } from "./link.ts";
import type { PaneTerminal } from "./pane.ts";
import type { Pane } from "./panes.ts";
import type { Source } from "./sources.ts";
import { outerTitle } from "./title.ts";

// Kept is what a shell let go by a restart hands its successor; the host
// carries it without reading it.
export type Kept = { focused?: string; chosen?: string; zoomed?: boolean; faces?: KeptFaces };

// Screen is the renderer once it draws, and what a pane's bytes ask of the
// person's terminal.
export type Screen = {
  destroy(): void;
  closed: Promise<unknown>;
  copy(text: string): void;
  title(text: string): void;
};

// Seed is the colours a widget starts from and the host's share of them
// (colors.ts).
export type Seed = { widget: string; host: string };

export type Size = { cols: number; rows: number };

export type LifeParts = {
  // link opens the channel with main's host (link.ts).
  link(onMessage: (message: FromHost) => void, onEnd: (why: string) => void): Link;
  // connect attaches to a host's stream (sources.ts).
  connect(
    socket: string,
    size: Size,
    onData: (data: Uint8Array) => void,
    onEnd: () => void,
  ): Source;
  main: Pane;
  workers(): Pane[];
  // kept is what the person chose, which a release hands on; restore applies
  // what the last shell handed over.
  kept(): Kept;
  restore(kept: Kept): void;
  // spacesOver resolves once the workers of ids are over (room.ts).
  spacesOver(ids: string[]): Promise<unknown>;
  endFaces(): void;
  reloadFaces(): void;
  say(line: string): void;
  exit(code: number): void;
  clock?: Clock;
};

// suspendWindow is how long after the person's Ctrl+Z went to main a leave
// of its screen is the suspend, not an end. Claude Code 2.1.302 leaves 4 ms
// after the key.
const suspendWindow = 1000;
// reattach is how long a pane still shown waits to attach again after its
// stream ended: a host switching its build comes back within it.
const reattach = 1000;
// leaveBound is how long the screen is waited for once destroyed.
const leaveBound = 1000;

// The pane's size as its program is told it.
const sizeOf = (term: PaneTerminal): Size => ({
  cols: Math.max(term.width, 20),
  rows: Math.max(term.height, 5),
});

export function shellLife(parts: LifeParts) {
  const clock = parts.clock ?? realClock;
  const allPanes = () => [parts.main, ...parts.workers()];
  // main is main's wrapper and its host's socket, once main is registered.
  let main: MainRef | undefined;
  let ending = false;
  let kept: Kept | undefined;
  let screen: Screen | undefined;
  let seed: Seed = { widget: "", host: "" };
  let suspendSentAt = Number.NEGATIVE_INFINITY;
  const stops: (() => void)[] = [];
  let stopScreen: (() => void) | undefined;
  const layer = closingLayer(() => ending, clock);
  // The host gives the shell the terminal with go: before it, another shell
  // may still be drawing on it.
  const given = Promise.withResolvers<Kept | undefined>();
  const link = parts.link(fromHost, (why) => {
    parts.say(`rewake-shell: ${why}`);
    void leave(1);
  });

  function fromHost(m: FromHost) {
    switch (m.t) {
      case "welcome":
        if (m.main) main = m.main;
        return;
      case "main":
        main = m.main;
        return attach(parts.main);
      case "go":
        kept = (m.state as Kept | undefined) ?? undefined;
        return given.resolve(kept);
      case "release":
        return void release();
      case "closing":
        return stopAll(true);
      case "main-ended":
        return void finish(m.code);
      case "error":
        return parts.say(`rewake-shell: the host says ${m.why}`);
      case "reload":
        // rewake restart face: every face reloaded from its files, as the
        // watch of them does.
        if (m.part === "face") {
          parts.reloadFaces();
          link.send({ t: "reloaded", part: m.part });
        }
    }
  }

  // feed passes a program's bytes to its pane, after the scanner took what
  // the widget would swallow.
  function feed(pane: Pane, data: Uint8Array) {
    for (const seen of pane.scanner.scan(data)) {
      if (pane === parts.main) {
        // Ctrl+Z leaves the screen too, and the harness is continued at once.
        if (seen.left && clock.now() - suspendSentAt > suspendWindow) layer.begin("left");
        if (seen.closing) layer.begin("mark");
        if (seen.entered) layer.lift();
      }
      if (seen.copy !== undefined) screen?.copy(seen.copy);
      if (seen.title !== undefined) {
        pane.title = seen.title;
        const out = outerTitle(pane === parts.main, seen.title);
        if (out !== undefined) screen?.title(out);
      }
      // While the program tracks the mouse a drag is the program's, not a
      // selection of OpenTUI's.
      if (seen.mouse !== undefined && pane.term) pane.term.selectable = !seen.mouse;
    }
    pane.term?.write(data);
    pane.shadow?.write(data);
  }

  // attach connects a pane to its host — main's, or a worker's space host —
  // again after a stream that ended while the pane is still shown: a fresh
  // preamble, on a widget reset to the seeded colours.
  function attach(pane: Pane) {
    const term = pane.term;
    const socket = pane === parts.main ? main?.socket : pane.space?.socket;
    if (!term || !socket || ending) return;
    pane.scanner.reset();
    term.write(`\x1bc${seed.widget}`);
    pane.shadow?.write(`\x1bc${seed.widget}`);
    pane.source = parts.connect(
      socket,
      sizeOf(term),
      (data) => feed(pane, data),
      () => {
        pane.source = undefined;
        clock.after(reattach, () => {
          if (pane === parts.main || parts.workers().includes(pane)) attach(pane);
        });
      },
    );
  }

  function startPane(pane: Pane, term: PaneTerminal) {
    pane.term = term;
    if (pane === parts.main && seed.widget) term.write(seed.widget);
    // The widget answers the program's queries too; the host answers them
    // already, so the widget's answers to a host-fed program are dropped.
    term.onData = (data, source) => {
      if (source === "response") return;
      if (pane === parts.main && isSuspend(data)) suspendSentAt = clock.now();
      pane.source?.write(data);
    };
    term.onTerminalResize = (cols, rows) => pane.source?.resize(cols, rows);
    attach(pane);
  }

  // stopAll lets go of every stream, the room's poll and the faces; with
  // closing, main's end has begun and the closing layer shows.
  function stopAll(closing: boolean) {
    if (closing) layer.begin();
    if (ending) return;
    ending = true;
    for (const stop of stops) stop();
    stopScreen?.();
    parts.endFaces();
    for (const pane of allPanes()) pane.source?.close();
  }

  // finish follows main's end, which the host saw: the closing layer stays
  // until the workers the shell showed are over, and a minimum time — main's
  // process ends only after its screen has gone, so without it the layer was
  // never drawn and the workers ended unseen.
  async function finish(code: number) {
    stopAll(true);
    const ids = parts.workers().map((p) => p.id);
    await Promise.all([
      parts.spacesOver(ids),
      // Begun by a sign of main's, the layer may have shown long enough already.
      new Promise((resolve) => clock.after(layer.remaining(), () => resolve(undefined))),
    ]);
    await leave(code);
  }

  // release is a restart's: the screen given back, then what the person had
  // chosen handed to the host for the next shell, which the host reads to
  // the last line after the exit.
  async function release() {
    const state = parts.kept();
    stopAll(false);
    await leave(undefined);
    link.send({ t: "released", state });
    parts.exit(0);
  }

  // leave gives the screen back and, given a code, exits with it.
  async function leave(code: number | undefined) {
    if (screen) {
      screen.destroy();
      await Promise.race([
        screen.closed,
        new Promise((resolve) => clock.after(leaveBound, () => resolve(undefined))),
      ]);
    }
    if (code !== undefined) parts.exit(code);
  }

  return {
    // given is what the last shell handed over, once the host gives the
    // terminal.
    given: given.promise,
    closing: layer.closing,
    beginClosing: () => layer.begin(),
    mainPid: () => main?.pid,
    send: link.send,
    // drawing is the renderer, once made; seeded the colours it asked for.
    drawing(drawn: Screen) {
      screen = drawn;
    },
    seeded(colors: Seed) {
      seed = colors;
    },
    // startMain starts main's pane; the host starts main at its size, with
    // the person's colours, and a later shell's answer changes neither.
    startMain(term: PaneTerminal) {
      startPane(parts.main, term);
      link.send({ t: "ready", colors: seed.host, ...sizeOf(term) });
    },
    startPane,
    // restore applies what the last shell handed over, once, on the first
    // view of the room, which shows the workers' panes again.
    restore() {
      if (!kept) return;
      const was = kept;
      kept = undefined;
      parts.restore(was);
    },
    // atStop is what the shell's end lets go of besides the streams.
    atStop(stop: () => void) {
      if (ending) stop();
      else stops.push(stop);
    },
    // serveScreen starts what answers rewake screen while the shell goes on:
    // once, or again at the next view when it could not listen.
    serveScreen(listen: () => (() => void) | undefined) {
      if (!stopScreen && !ending) stopScreen = listen();
    },
    // terminalGone follows the person's terminal (ending.ts): a stop signal
    // asks the host to close; a terminal gone is told, and the shell leaves,
    // with nothing left to draw on.
    terminalGone(lost: boolean) {
      if (!lost) return link.send({ t: "close" });
      link.send({ t: "lost" });
      stopScreen?.();
      parts.exit(1);
    },
  };
}
