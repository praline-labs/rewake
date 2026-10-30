// rewake's shell (docs/shell.md): main on the left, from 120 columns the
// chosen worker, or the briefs, on the right, each pane with its status line under it; the
// key hints under main's, the bar choosing the worker under the worker's.
// Below 120 columns the shown pane takes the whole width, the hints and the
// bar under its status, and the bar switches the whole window.
// Started by rewake --shell as
//   rewake-shell --room <room> -- <main's command line>

import { createCliRenderer } from "@opentui/core";
import { render, testRender, useTerminalDimensions } from "@opentui/solid";
import { createEffect, createSignal } from "solid-js";
import { takeBriefs } from "./briefs.tsx";
import { askColors, seedOf } from "./colors.ts";
import { endMain, tailText, watchTerminal } from "./ending.ts";
import { copyWith, endFaces, faceShown, takeMirrors, toggleFace } from "./face/faces.ts";
import { type Command, isSuspend, prefixHandler } from "./keys.ts";
import { closingLayer } from "./lifting.ts";
import { type PaneTerminal, paneHooks } from "./pane.ts";
import {
  allPanes,
  briefs,
  chosen,
  cycle,
  focused,
  mainPane,
  type Pane,
  paneOf,
  setChosen,
  setFocused,
  setWorkers,
  setZoomed,
  show,
  takeSpaces,
  toggleBriefs,
  workers,
  zoomed,
} from "./panes.ts";
import { endedSpaces, pollRoom, shownSpaces, spacesOver } from "./room.ts";
import { Scanner } from "./scan.ts";
import { type Frame, frameOf, listenScreen } from "./screen.ts";
import { connectWorker, type MainRun, mainEnv, type Source, startMain } from "./sources.ts";
import { showSessions, stripOf } from "./strip.tsx";
import { themeOf } from "./theme.ts";
import { type Env, type View, Window } from "./window.tsx";

const args = process.argv.slice(2);
const split = args.indexOf("--");
const room = args[args.indexOf("--room") + 1] ?? "default";
const command = args.slice(split + 1);
if (split < 0 || command.length === 0) {
  process.stderr.write("usage: rewake-shell --room <room> -- <main's command line>\n");
  process.exit(2);
}
const rewake = command[0]!;
const cwd = process.env.REWAKE_SHELL_CWD || process.cwd();

// prefixWaits is a typed Ctrl+\ waiting for its command.
const [prefixWaits, setPrefixWaits] = createSignal(false);
const workersBound = 10_000;
// suspendSentAt is when the person's Ctrl+Z last went to main: a leave of the
// screen within suspendWindow after it is the suspend, not an end. Claude
// Code 2.1.302 leaves 4 ms after the key.
let suspendSentAt = 0;
const suspendWindow = 1000;

let main: MainRun | undefined;
let mainTail = "";
let ending = false;
const {
  closing,
  begin: beginClosing,
  lift: liftClosing,
  remaining: closingRemaining,
} = closingLayer(() => ending);
let stopRoom = () => {};

const renderer = await createCliRenderer({
  exitOnCtrlC: false,
  exitSignals: [],
  useMouse: true,
  prependInputHandlers: [prefixHandler(run, setPrefixWaits)],
});
const colors = await askColors(renderer);
const seed = seedOf(colors);
paneHooks.standIn = seed.standIn;
const theme = themeOf(colors);

// feed passes a program's bytes to its pane, after the scanner took what
// the widget would swallow.
function feed(pane: Pane, data: Uint8Array) {
  for (const seen of pane.scanner.scan(data)) {
    if (pane === mainPane) {
      // Ctrl+Z leaves the screen too, and the harness is continued at once.
      if (seen.left && Date.now() - suspendSentAt > suspendWindow) beginClosing("left");
      if (seen.closing) beginClosing("mark");
      if (seen.entered) liftClosing();
    }
    if (seen.copy !== undefined) renderer.copyToClipboardOSC52(seen.copy);
    if (seen.title !== undefined) {
      pane.title = seen.title;
      if (focused() === pane.id) renderer.setTerminalTitle(seen.title);
    }
    // While the program tracks the mouse a drag is the program's, not a
    // selection of OpenTUI's.
    if (seen.mouse !== undefined && pane.term) pane.term.selectable = !seen.mouse;
  }
  pane.term?.write(data);
  pane.shadow?.write(data);
}

function startMainPane(term: PaneTerminal) {
  mainPane.term = term;
  if (seed.widget) term.write(seed.widget);
  term.onData = (data) => {
    if (isSuspend(data)) suspendSentAt = Date.now();
    main?.write(data);
  };
  term.onTerminalResize = (cols, rows) => main?.resize(cols, rows);
  main = startMain(
    command,
    mainEnv(seed.host),
    cwd,
    { cols: Math.max(term.width, 20), rows: Math.max(term.height, 5) },
    (data) => {
      mainTail = (mainTail + Buffer.from(data).toString("utf8")).slice(-16_384);
      feed(mainPane, data);
    },
  );
  void main.child.exited.then((code) => finish(code));
}

// attach connects a worker's pane to its space host, again after a stream
// that ended while the space is still open: a fresh preamble, on a widget
// reset to the seeded colours.
function attach(pane: Pane) {
  const term = pane.term;
  if (!term || !pane.space || ending) return;
  pane.scanner.reset();
  term.write(`\x1bc${seed.widget}`);
  pane.shadow?.write(`\x1bc${seed.widget}`);
  pane.source = connectWorker(
    pane.space.socket,
    { cols: Math.max(term.width, 20), rows: Math.max(term.height, 5) },
    (data) => feed(pane, data),
    () => {
      pane.source = undefined;
      setTimeout(() => {
        if (workers().includes(pane)) attach(pane);
      }, 1000);
    },
  );
}

function startWorkerPane(pane: Pane, term: PaneTerminal) {
  pane.term = term;
  // The widget answers the program's queries too; the host answers them
  // already, so the widget's answers to a host-fed program are dropped.
  term.onData = (data, source) => {
    if (source !== "response") pane.source?.write(data);
  };
  term.onTerminalResize = (cols, rows) => pane.source?.resize(cols, rows);
  attach(pane);
}

createEffect(() => {
  // The briefs have no terminal: while they have the focus every pane is
  // blurred and their view takes the keys.
  const id = closing() === "sure" ? undefined : focused();
  for (const pane of allPanes()) if (pane.id !== id) pane.term?.blur();
  if (id === undefined) return;
  const pane = paneOf(id);
  // On the next tick: a pane shown by this change is mounted only then. A
  // pane under rewake's face leaves the keys to the face's input.
  const face = faceShown(id);
  setTimeout(() => (face ? pane?.term?.blur() : pane?.term?.focus()), 0);
  if (pane?.title) renderer.setTerminalTitle(pane.title);
});

function run(c: Command) {
  if (closing() === "sure") return;
  const panes = allPanes();
  const ring = cycle();
  const at = Math.max(0, ring.indexOf(focused()));
  switch (c.kind) {
    case "next":
      return show(ring[(at + 1) % ring.length]);
    case "prev":
      return show(ring[(at + ring.length - 1) % ring.length]);
    case "main":
    case "left":
      return show("main");
    case "right":
      return show(chosen());
    case "index":
      return show(panes[c.index]?.id);
    case "zoom":
      return setZoomed(!zoomed());
    case "briefs":
      return toggleBriefs();
    case "face":
      return toggleFace(focused());
    // The prefix twice types it.
    case "prefix":
      return (focused() === "main" ? main : paneOf(focused())?.source)?.write(
        new Uint8Array([0x1c]),
      );
  }
}

paneHooks.pressed = (id) => {
  if (id !== focused() && closing() !== "sure") show(id);
};

function App() {
  const dims = useTerminalDimensions();
  const view = (): View => ({
    focused: focused(),
    chosen: chosen(),
    zoomed: zoomed(),
    width: dims().width,
    height: dims().height,
    prefix: prefixWaits(),
  });
  return <Window env={liveEnv} view={view} />;
}

const base = { seed, theme, rewake, room };
const liveEnv: Env = {
  ...base,
  live: true,
  closing,
  mainRef: startMainPane,
  workerRef: startWorkerPane,
};

// finish ends the shell with main's code: main hung up and waited for, the
// workers' streams closed, the screen given back. Unless the terminal is
// lost, the closing layer shows from its start.
async function finish(code?: number, lost = false) {
  if (ending) return;
  ending = true;
  if (!lost) beginClosing();
  stopRoom();
  stopScreen?.();
  endFaces();
  for (const pane of workers()) pane.source?.close();
  if (main) code = await endMain(main.child, main.close);
  // Main's process ends only after its screen has gone, and the shell's
  // own end followed within milliseconds, so the layer was never drawn and
  // the workers ended unseen after it. It stays until the workers the
  // shell showed are over, and a minimum time.
  if (!lost) {
    const ids = workers().map((p) => p.id);
    await Promise.all([
      spacesOver(rewake, room, ids, workersBound),
      // Begun by a sign of main's, the layer may have shown long enough already.
      new Promise((resolve) => setTimeout(resolve, closingRemaining())),
    ]);
  }
  renderer.destroy();
  await Promise.race([renderer.closed, new Promise((resolve) => setTimeout(resolve, 1000))]);
  if (code !== 0 && process.stderr.isTTY) {
    const said = tailText(mainTail);
    if (said) process.stderr.write(`${said}\n`);
  }
  process.exit(code ?? 1);
}

// The shadow window: the same drawing in a tree of its own that is never put
// on the person's screen, with a widget beside every pane's that is fed the
// same bytes. rewake screen asks it for a window with another session in
// its place, so nothing on the person's screen changes (docs/screen.md).
const [shadowView, setShadowView] = createSignal<View>({
  focused: "main",
  chosen: undefined,
  zoomed: false,
  width: renderer.width,
  height: renderer.height,
  prefix: false,
});
const seedShadow = (t: PaneTerminal) => {
  if (seed.widget) t.write(seed.widget);
};
const shadowEnv: Env = {
  ...base,
  live: false,
  closing: () => false,
  mainRef: (t) => {
    mainPane.shadow = t;
    seedShadow(t);
  },
  workerRef: (pane, t) => {
    pane.shadow = t;
    seedShadow(t);
  },
};
const shadow = await testRender(() => <Window env={shadowEnv} view={shadowView} />, {
  width: renderer.width,
  height: renderer.height,
});
// The shadow widgets are laid out whenever the person's are — a pane that
// appears, the window's size, the zoom — so each keeps the size its program
// draws for: the bytes they are fed are of that size, and a widget left at
// its default would be fed a screen it cannot hold.
const [windowSize, setWindowSize] = createSignal({
  width: renderer.width,
  height: renderer.height,
});
renderer.on("resize", (width: number, height: number) => setWindowSize({ width, height }));
createEffect(() => {
  workers();
  const { width, height } = windowSize();
  setShadowView((v) => ({ ...v, width, height, chosen: chosen(), zoomed: zoomed() }));
  shadow.resize(width, height);
  void shadow.renderOnce();
});

// draw is the window with the pane in its place: main's own, or the worker's
// beside main (or alone when the shell is narrow or zoomed).
async function draw(id: string): Promise<Frame> {
  const target = paneOf(id);
  if (!target?.shadow) throw new Error(`no window ${id} to draw`);
  const { width, height } = renderer;
  setShadowView({
    focused: id,
    chosen: id === "main" ? chosen() : id,
    zoomed: zoomed(),
    width,
    height,
    prefix: false,
  });
  if (shadow.renderer.width !== width || shadow.renderer.height !== height)
    shadow.resize(width, height);
  await shadow.renderOnce();
  await Bun.sleep(30);
  await shadow.renderOnce();
  const { cursor } = target.shadow.screen();
  return frameOf(shadow.captureSpans(), {
    pane: id,
    fg: colors?.defaultForeground ?? undefined,
    bg: colors?.defaultBackground ?? undefined,
    palette: colors?.palette,
    // Under rewake's face the native cursor is covered; the face draws none.
    cursor:
      cursor.visible && !faceShown(id)
        ? { x: target.shadow.screenX + cursor.x, y: target.shadow.screenY + cursor.y }
        : undefined,
  });
}

// One picture at a time: the shadow window has one state.
let drawing: Promise<unknown> = Promise.resolve();
function takeScreen(id: string): Promise<Frame> {
  const next = drawing.then(() => draw(id));
  drawing = next.catch(() => {});
  return next;
}
let stopScreen: (() => void) | undefined;
const screenPath = process.env.REWAKE_SHELL_SOCK;

watchTerminal((_why, lost) => void finish(undefined, lost));
renderer.on("selection", (selection: { getSelectedText?: () => string } | null) => {
  const text = selection?.getSelectedText?.() ?? "";
  if (text) renderer.copyToClipboardOSC52(text);
});
// A face's copy takes the same path: the person's terminal, by OSC 52.
copyWith((text) => renderer.copyToClipboardOSC52(text));
await render(() => <App />, renderer);
stopRoom = pollRoom(
  rewake,
  room,
  () => main?.child.pid,
  (view, asked) => {
    if (view) {
      takeSpaces(shownSpaces(view), endedSpaces(view), closing, beginClosing);
      showSessions(new Map(view.sessions.map((s) => [s.name, s])));
      takeMirrors(workers().map((p) => [p.id, stripOf(p.id).status?.mirror]));
    }
    takeBriefs(view, asked);
    if (screenPath && !stopScreen && !ending) stopScreen = listenScreen(screenPath, takeScreen);
  },
);
