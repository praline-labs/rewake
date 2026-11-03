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
  faceEmbedded,
  faceShown,
  keptFaces,
  reloadFaces,
  restoreFaces,
  takeMirrors,
  toggleFace,
  workerUrl,
} from "./face/faces.ts";
import { type Command, prefixHandler } from "./keys.ts";
import { shellLife } from "./life.ts";
import { openLink } from "./link.ts";
import { paneHooks } from "./pane.ts";
import {
  allPanes,
  choice,
  chosen,
  cycle,
  focused,
  mainPane,
  paneOf,
  restoreChoice,
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
import { contracts, helloContracts, takeUpdate } from "./update.ts";
import { type Env, type View, Window } from "./window.tsx";

const args = process.argv.slice(2);
if (args[0] === "--face-check") {
  const why = await checkFace();
  process.stdout.write(why ? `${why}\n` : `the face answers: ${workerUrl}\n`);
  process.exit(why ? 1 : 0);
}
// --contracts prints what this shell and its face speak and require, which
// the tools that write a manifest read from the shell itself.
if (args[0] === "--contracts") {
  process.stdout.write(`${JSON.stringify(contracts())}\n`);
  process.exit(0);
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

// The shell's life (life.ts): the host's channel and the panes' streams, on
// this process's descriptor, sockets, clock and exit.
const life = shellLife({
  link: (onMessage, onEnd) => openLink(hostFd, onMessage, onEnd, helloContracts(faceEmbedded)),
  connect: connectHost,
  main: mainPane,
  workers,
  kept: () => ({ ...choice(), faces: keptFaces() }),
  restore: restoreChoice,
  spacesOver: (ids) => spacesOver(rewake, room, ids, 10_000),
  endFaces,
  reloadFaces,
  say: (line) => process.stderr.write(`${line}\n`),
  exit: (code) => process.exit(code),
});
const { closing } = life;

restoreFaces((await life.given)?.faces);
const renderer = await createCliRenderer({
  exitOnCtrlC: false,
  exitSignals: [],
  useMouse: true,
  prependInputHandlers: [prefixHandler(run, setPrefixWaits)],
});
life.drawing({
  destroy: () => renderer.destroy(),
  get closed() {
    return renderer.closed;
  },
  copy: (text) => renderer.copyToClipboardOSC52(text),
  title: (text) => renderer.setTerminalTitle(text),
});
const colors = await askColors(renderer);
const seed = seedOf(colors);
life.seeded(seed);
paneHooks.standIn = seed.standIn;
const theme = themeOf(colors);

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
      return life.send({ t: "restart" });
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
  mainRef: life.startMain,
  workerRef: life.startPane,
};

const takeScreen = await shadowOf(renderer, base, colors);
const screenPath = process.env.REWAKE_SHELL_SOCK;

watchTerminal((_why, lost) => life.terminalGone(lost));
renderer.on("selection", (selection: { getSelectedText?: () => string } | null) => {
  const text = selection?.getSelectedText?.() ?? "";
  if (text) renderer.copyToClipboardOSC52(text);
});
// A face's copy takes the same path: the person's terminal, by OSC 52.
copyWith((text) => renderer.copyToClipboardOSC52(text));
await render(() => <App />, renderer);
const stopRoom = pollRoom(rewake, room, life.mainPid, (view, asked) => {
  if (view) {
    takeSpaces(shownSpaces(view), endedSpaces(view), closing, life.beginClosing);
    life.restore();
    showSessions(new Map(view.sessions.map((s) => [s.name, s])));
    takeUpdate(view.update, view.available);
    takeMirrors(workers().map((p) => [p.id, stripOf(p.id).status?.mirror]));
  }
  takeBriefs(view, asked);
  if (screenPath) life.serveScreen(() => listenScreen(screenPath, takeScreen));
});
life.atStop(stopRoom);
