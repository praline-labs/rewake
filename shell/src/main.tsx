// rewake's shell (docs/shell.md): main on the left, from 120 columns the
// chosen worker, or the briefs, on the right, each pane with its status line under it; the
// key hints under main's, the bar choosing the worker under the worker's.
// Below 120 columns the shown pane takes the whole width, the hints and the
// bar under its status, and the bar switches the whole window.
// Started by main's host (internal/host/mainhost), the process of the
// person's pane, as
//   rewake-shell --room <room> -- <main's command line>
// with the host's channel on REWAKE_HOST_FD; the host holds main's pty, so
// the shell can be replaced while main runs (docs/shell.md). Started by the
// release gate as rewake-shell --face-check, which starts the face it
// carries and says whether it answers.

import { createCliRenderer } from "@opentui/core";
import { render, useTerminalDimensions } from "@opentui/solid";
import { createEffect, createSignal } from "solid-js";
import { takeBriefs } from "./briefs.tsx";
import { askColors, seedOf } from "./colors.ts";
import { watchTerminal } from "./ending.ts";
import {
  checkFace,
  copyWith,
  endFaces,
  faceShown,
  type KeptFaces,
  keptFaces,
  restoreFaces,
  takeMirrors,
  toggleFace,
  workerUrl,
} from "./face/faces.ts";
import { type Command, isSuspend, prefixHandler } from "./keys.ts";
import { closingLayer } from "./lifting.ts";
import { type FromHost, type MainRef, openLink } from "./link.ts";
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
  setZoomed,
  show,
  takeSpaces,
  toggleBriefs,
  workers,
  zoomed,
} from "./panes.ts";
import { endedSpaces, pollRoom, shownSpaces, spacesOver } from "./room.ts";
import { listenScreen } from "./screen.ts";
import { shadowOf } from "./shadow.tsx";
import { connectHost } from "./sources.ts";
import { showSessions, stripOf } from "./strip.tsx";
import { themeOf } from "./theme.ts";
import { outerTitle } from "./title.ts";
import { type Env, type View, Window } from "./window.tsx";

const args = process.argv.slice(2);
if (args[0] === "--face-check") {
  const why = await checkFace();
  process.stdout.write(why ? `${why}\n` : `the face answers: ${workerUrl}\n`);
  process.exit(why ? 1 : 0);
}
const split = args.indexOf("--");
const room = args[args.indexOf("--room") + 1] ?? "default";
const command = args.slice(split + 1);
if (split < 0 || command.length === 0) {
  process.stderr.write("usage: rewake-shell --room <room> -- <main's command line>\n");
  process.exit(2);
}
const rewake = command[0]!;
const hostFd = Number(process.env.REWAKE_HOST_FD);
if (!Number.isInteger(hostFd) || hostFd < 3) {
  process.stderr.write(
    "rewake-shell runs under main's host, which rewake starts: this rewake is older than the shell, and the two must be of one version\n",
  );
  process.exit(2);
}

// prefixWaits is a typed Ctrl+\ waiting for its command.
const [prefixWaits, setPrefixWaits] = createSignal(false);
const workersBound = 10_000;
// suspendSentAt is when the person's Ctrl+Z last went to main: a leave of the
// screen within suspendWindow after it is the suspend, not an end. Claude
// Code 2.1.302 leaves 4 ms after the key.
let suspendSentAt = 0;
const suspendWindow = 1000;

// main is main's wrapper and its host's socket, once main is registered.
let main: MainRef | undefined;
let ending = false;
// Kept is what a shell let go by a restart hands its successor.
type Kept = { focused?: string; chosen?: string; zoomed?: boolean; faces?: KeptFaces };
let kept: Kept | undefined;
const {
  closing,
  begin: beginClosing,
  lift: liftClosing,
  remaining: closingRemaining,
} = closingLayer(() => ending);
let stopRoom = () => {};
let stopScreen: (() => void) | undefined;
// shown is the renderer once it draws: the host's channel may end before.
let shown: { destroy(): void; closed: Promise<unknown> } | undefined;

// The host gives the shell the terminal with go: before it, another shell
// may still be drawing on it.
const given = Promise.withResolvers<unknown>();
const link = openLink(hostFd, fromHost, (why) => {
  process.stderr.write(`rewake-shell: ${why}\n`);
  leave(1);
});

function fromHost(m: FromHost) {
  switch (m.t) {
    case "welcome":
      if (m.main) main = m.main;
      return;
    case "main":
      main = m.main;
      return attach(mainPane);
    case "go":
      return given.resolve(m.state);
    case "release":
      return void release();
    case "closing":
      return stopAll(true);
    case "main-ended":
      return void finish(m.code);
    case "error":
      process.stderr.write(`rewake-shell: the host says ${m.why}\n`);
  }
}

kept = ((await given.promise) as Kept | undefined) ?? undefined;
restoreFaces(kept?.faces);
const renderer = await createCliRenderer({
  exitOnCtrlC: false,
  exitSignals: [],
  useMouse: true,
  prependInputHandlers: [prefixHandler(run, setPrefixWaits)],
});
shown = renderer;
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
      const out = outerTitle(pane === mainPane, seen.title);
      if (out !== undefined) renderer.setTerminalTitle(out);
    }
    // While the program tracks the mouse a drag is the program's, not a
    // selection of OpenTUI's.
    if (seen.mouse !== undefined && pane.term) pane.term.selectable = !seen.mouse;
  }
  pane.term?.write(data);
  pane.shadow?.write(data);
}

// The pane's size as its program is told it.
const sizeOf = (term: PaneTerminal) => ({
  cols: Math.max(term.width, 20),
  rows: Math.max(term.height, 5),
});

function startMainPane(term: PaneTerminal) {
  startPane(mainPane, term);
  // The host starts main at this size, with the person's colours; a later
  // shell's answer changes neither.
  link.send({ t: "ready", colors: seed.host, ...sizeOf(term) });
}

// attach connects a pane to its host — main's, or a worker's space host —
// again after a stream that ended while the pane is still shown: a fresh
// preamble, on a widget reset to the seeded colours.
function attach(pane: Pane) {
  const term = pane.term;
  const socket = pane === mainPane ? main?.socket : pane.space?.socket;
  if (!term || !socket || ending) return;
  pane.scanner.reset();
  term.write(`\x1bc${seed.widget}`);
  pane.shadow?.write(`\x1bc${seed.widget}`);
  pane.source = connectHost(
    socket,
    sizeOf(term),
    (data) => feed(pane, data),
    () => {
      pane.source = undefined;
      setTimeout(() => {
        if (pane === mainPane || workers().includes(pane)) attach(pane);
      }, 1000);
    },
  );
}

function startPane(pane: Pane, term: PaneTerminal) {
  pane.term = term;
  if (pane === mainPane && seed.widget) term.write(seed.widget);
  // The widget answers the program's queries too; the host answers them
  // already, so the widget's answers to a host-fed program are dropped.
  term.onData = (data, source) => {
    if (source === "response") return;
    if (pane === mainPane && isSuspend(data)) suspendSentAt = Date.now();
    pane.source?.write(data);
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
  const out = outerTitle(true, mainPane.title);
  if (out !== undefined) renderer.setTerminalTitle(out);
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
    case "restart":
      return link.send({ t: "restart" });
    // The prefix twice types it.
    case "prefix":
      return paneOf(focused())?.source?.write(new Uint8Array([0x1c]));
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
  workerRef: startPane,
};

// stopAll lets go of every stream, the room's poll and the faces; with
// closing, main's end has begun and the closing layer shows.
function stopAll(closing: boolean) {
  if (closing) beginClosing();
  if (ending) return;
  ending = true;
  stopRoom();
  stopScreen?.();
  endFaces();
  for (const pane of allPanes()) pane.source?.close();
}

// finish follows main's end, which the host saw: the closing layer stays
// until the workers the shell showed are over, and a minimum time — main's
// process ends only after its screen has gone, so without it the layer was
// never drawn and the workers ended unseen.
async function finish(code: number) {
  stopAll(true);
  const ids = workers().map((p) => p.id);
  await Promise.all([
    spacesOver(rewake, room, ids, workersBound),
    // Begun by a sign of main's, the layer may have shown long enough already.
    new Promise((resolve) => setTimeout(resolve, closingRemaining())),
  ]);
  await leave(code);
}

// release is a restart's: the screen given back, then what the person had
// chosen handed to the host for the next shell.
async function release() {
  const state: Kept = {
    focused: focused(),
    chosen: chosen(),
    zoomed: zoomed(),
    faces: keptFaces(),
  };
  stopAll(false);
  await leave(undefined);
  link.send({ t: "released", state });
  process.exit(0);
}

// leave gives the screen back and, given a code, exits with it.
async function leave(code: number | undefined) {
  if (shown) {
    shown.destroy();
    await Promise.race([shown.closed, new Promise((resolve) => setTimeout(resolve, 1000))]);
  }
  if (code !== undefined) process.exit(code);
}

// restore applies what the last shell handed over, on the first view of the
// room, which shows the workers' panes again; one closed meanwhile is not
// chosen.
function restore() {
  if (!kept) return;
  const was = kept;
  kept = undefined;
  if (was.chosen === briefs ? workers().length > 0 : was.chosen !== "main" && paneOf(was.chosen))
    setChosen(was.chosen);
  setZoomed(was.zoomed ?? false);
  show(was.focused);
}

const takeScreen = await shadowOf(renderer, base, colors);
const screenPath = process.env.REWAKE_SHELL_SOCK;

// A stop signal asks the host to close; a terminal gone is told, and the
// shell leaves, with nothing left to draw on.
watchTerminal((_why, lost) => {
  if (!lost) return link.send({ t: "close" });
  link.send({ t: "lost" });
  stopScreen?.();
  process.exit(1);
});
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
  () => main?.pid,
  (view, asked) => {
    if (view) {
      takeSpaces(shownSpaces(view), endedSpaces(view), closing, beginClosing);
      restore();
      showSessions(new Map(view.sessions.map((s) => [s.name, s])));
      takeMirrors(workers().map((p) => [p.id, stripOf(p.id).status?.mirror]));
    }
    takeBriefs(view, asked);
    if (screenPath && !stopScreen && !ending) stopScreen = listenScreen(screenPath, takeScreen);
  },
);
