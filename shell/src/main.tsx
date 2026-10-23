// rewake's shell (docs/v2/shell.md): main on the left, from 120 columns the
// chosen worker on the right, under main's pane a bar choosing the worker and
// a line of key hints; below 120 columns they take the whole width and the
// bar switches the whole window.
// Started by rewake --shell as
//   rewake-shell --room <room> -- <main's command line>

import { createCliRenderer } from "@opentui/core";
import { render, useTerminalDimensions } from "@opentui/solid";
import { createEffect, createSignal, For, Show } from "solid-js";
import { Nav } from "./bar.tsx";
import { Closing } from "./closing.tsx";
import { askColors, seedOf } from "./colors.ts";
import { endMain, tailText, watchTerminal } from "./ending.ts";
import { type Command, isSuspend, prefixHandler } from "./keys.ts";
import { type PaneTerminal, paneHooks } from "./pane.ts";
import { pollRoom, type Space, spacesOver } from "./room.ts";
import { Scanner } from "./scan.ts";
import { connectWorker, type MainRun, type Source, startMain } from "./sources.ts";
import { Strip, showSessions, showSpaces } from "./strip.tsx";
import { themeOf } from "./theme.ts";

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

// A pane is a program on the screen: main, or a space's worker.
type Pane = {
  id: string;
  label: string;
  term?: PaneTerminal;
  source?: Source;
  scanner: Scanner;
  title: string;
  space?: Space;
};

const mainPane: Pane = { id: "main", label: "main", scanner: new Scanner(), title: "" };
const [workers, setWorkers] = createSignal<Pane[]>([]);
const [focused, setFocused] = createSignal("main");
const [chosen, setChosen] = createSignal<string>();
const [zoomed, setZoomed] = createSignal(false);
// prefixWaits is a typed Ctrl+\ waiting for its command.
const [prefixWaits, setPrefixWaits] = createSignal(false);
// closing is set when main's end begins: from then until the shell exits the
// screen is the closing layer alone, and once main's end is sure the keys
// reach no program. "left" is a layer begun by main leaving its screen alone,
// under which main keeps its focus and keys: a leave may be no end.
const [closing, setClosing] = createSignal<"" | "left" | "sure">("");
// closingAt is when the closing layer began, so it is shown at least
// closingShown before the shell exits.
let closingAt = 0;
const closingShown = 600;
const workersBound = 10_000;
// closingSign is a closing begun by a sign of main's alone — main leaving its
// screen, the earliest, or main's closing mark — lifted when main comes back
// to its screen, or still runs closingLift after the last sign: a screen that
// hid a main going on would leave the person no way to it.
let closingSign = false;
let liftTimer: ReturnType<typeof setTimeout> | undefined;
const closingLift = 5000;
function beginClosing(sign?: "left" | "mark") {
  if (closing() === "sure" && !closingSign) return;
  clearTimeout(liftTimer);
  closingSign = sign !== undefined;
  if (closingSign) liftTimer = setTimeout(liftClosing, closingLift);
  if (!closing()) closingAt = Date.now();
  if (sign !== "left") setClosing("sure");
  else if (!closing()) setClosing("left");
}
function liftClosing() {
  if (!closingSign || ending) return;
  clearTimeout(liftTimer);
  closingSign = false;
  setClosing("");
}
// suspendSentAt is when the person's Ctrl+Z last went to main: a leave of the
// screen within suspendWindow after it is the suspend, not an end. Claude
// Code 2.1.302 leaves 4 ms after the key.
let suspendSentAt = 0;
const suspendWindow = 1000;
const allPanes = () => [mainPane, ...workers()];
const paneOf = (id: string | undefined) => allPanes().find((p) => p.id === id);

let main: MainRun | undefined;
let mainTail = "";
let ending = false;
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
}

function startMainPane(term: PaneTerminal) {
  mainPane.term = term;
  if (seed.widget) term.write(seed.widget);
  term.onData = (data) => {
    if (isSuspend(data)) suspendSentAt = Date.now();
    main?.write(data);
  };
  term.onTerminalResize = (cols, rows) => main?.resize(cols, rows);
  const env: Record<string, string> = {
    ...(process.env as Record<string, string>),
    TERM: "xterm-256color",
    COLORTERM: "truecolor",
  };
  delete env.REWAKE_SHELL_CWD;
  // The person's colours reach every space host through main's environment,
  // so a worker's program hears them from its host (docs/v2/shell.md).
  if (seed.host) env.REWAKE_HOST_COLORS = seed.host;
  // pi copies through the machine's clipboard where it finds one, which never
  // reaches the person's terminal from inside the shell (on WSL without
  // WT_SESSION, PowerShell); this makes it send OSC 52 as well, which the
  // scanner carries out. Main's workers inherit it like the colours.
  env.PI_CLIPBOARD_OSC52 ??= "1";
  main = startMain(
    command,
    env,
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

// The room's spaces become the worker panes, in the order they opened. A
// space shown here whose main's run reads ended is main's end begun.
function takeSpaces(spaces: Space[], ended: string[]) {
  if (closing()) return;
  showSpaces(spaces);
  const was = workers();
  if (was.some((p) => ended.includes(p.id))) return beginClosing();
  const next = spaces.map(
    (space) =>
      was.find((p) => p.id === space.id) ?? {
        id: space.id,
        label: space.name,
        scanner: new Scanner(),
        title: "",
        space,
      },
  );
  for (const pane of was) if (!next.includes(pane)) pane.source?.close();
  if (next.length === was.length && next.every((p, i) => p === was[i])) return;
  setWorkers(next);
  if (!paneOf(chosen()) || chosen() === "main") setChosen(next[0]?.id);
  if (!paneOf(focused())) show("main");
}

function show(id: string | undefined) {
  const pane = paneOf(id);
  if (!pane) return;
  if (pane !== mainPane) setChosen(pane.id);
  setFocused(pane.id);
}

createEffect(() => {
  const id = closing() === "sure" ? undefined : focused();
  for (const pane of allPanes()) if (pane.id !== id) pane.term?.blur();
  if (id === undefined) return;
  const pane = paneOf(id);
  // On the next tick: a pane shown by this change is mounted only then.
  setTimeout(() => pane?.term?.focus(), 0);
  if (pane?.title) renderer.setTerminalTitle(pane.title);
});

function run(c: Command) {
  if (closing() === "sure") return;
  const panes = allPanes();
  const at = Math.max(
    0,
    panes.findIndex((p) => p.id === focused()),
  );
  switch (c.kind) {
    case "next":
      return show(panes[(at + 1) % panes.length]?.id);
    case "prev":
      return show(panes[(at + panes.length - 1) % panes.length]?.id);
    case "main":
    case "left":
      return show("main");
    case "right":
      return show(chosen());
    case "index":
      return show(panes[c.index]?.id);
    case "zoom":
      return setZoomed(!zoomed());
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
  const wide = () => dims().width >= 120 && chosen() !== undefined && !zoomed();
  const right = () => Math.floor((dims().width - 1) / 2);
  const shown = (p: Pane) => (wide() ? p === mainPane || p.id === chosen() : p.id === focused());
  const widthOf = (p: Pane) =>
    !wide() ? dims().width : p === mainPane ? dims().width - 1 - right() : right();
  // The bar and the hints take two lines under main's pane in the wide
  // layout, and under the whole row in the narrow one; a worker's column
  // takes the full height in the wide layout, so both columns end together.
  const navHeight = 2;
  const rowHeight = () => Math.max(dims().height - (wide() ? 0 : navHeight), 1);
  const height = (p: Pane) =>
    p === mainPane ? Math.max(dims().height - navHeight, 1) : rowHeight();
  // A pane sits in a box with a column of padding on each side, so the text
  // does not touch the edge; the widget's size, and so the program's, is the
  // inside. A press on the padding chooses the pane like one on its text.
  const padded = (p: Pane) => ({
    visible: shown(p),
    width: widthOf(p),
    height: height(p),
    paddingLeft: 1,
    paddingRight: 1,
    backgroundColor: seed.transparent ? undefined : theme.background,
    onMouseDown: () => paneHooks.pressed(p.id),
  });
  const inner = (p: Pane) => Math.max(widthOf(p) - 2, 1);
  // The navigation is one: under main's pane, in main's column, while both
  // panes show; across the whole width while one does.
  const nav = (width: number) => (
    <Nav
      tabs={allPanes().map((p) => ({ id: p.id, label: p.label, main: p === mainPane }))}
      focused={focused()}
      chosen={wide() && focused() === "main" ? chosen() : undefined}
      zoomed={zoomed()}
      prefix={prefixWaits()}
      width={width}
      theme={theme}
      pick={show}
    />
  );
  // The work is hidden as a whole under the closing layer, so a terminal
  // background the shell does not know never lets a pane show through.
  return (
    <box width="100%" height="100%">
      <box flexDirection="column" width="100%" height="100%" visible={!closing()}>
        <box flexDirection="row" height={rowHeight()}>
          <box flexDirection="column" visible={shown(mainPane)} width={widthOf(mainPane)}>
            <box {...padded(mainPane)}>
              <pane_terminal
                id="main"
                ref={(t: PaneTerminal) => startMainPane(t)}
                transparentBackground={seed.transparent}
                width={inner(mainPane)}
                height={height(mainPane)}
              />
            </box>
            <Show when={wide()}>{nav(widthOf(mainPane))}</Show>
          </box>
          <Show when={wide()}>
            <box width={1} height={rowHeight()} backgroundColor="#444444" />
          </Show>
          <For each={workers()}>
            {(pane) => (
              <box {...padded(pane)} flexDirection="column">
                <pane_terminal
                  id={pane.id}
                  ref={(t: PaneTerminal) => startWorkerPane(pane, t)}
                  transparentBackground={seed.transparent}
                  width={inner(pane)}
                  height={Math.max(height(pane) - 1, 1)}
                />
                <Strip id={pane.id} width={inner(pane)} theme={theme} />
              </box>
            )}
          </For>
        </box>
        <Show when={!wide()}>{nav(dims().width)}</Show>
      </box>
      <Show when={closing()}>
        <Closing theme={theme} />
      </Show>
    </box>
  );
}

// finish ends the shell with main's code: main hung up and waited for, the
// workers' streams closed, the screen given back. Unless the terminal is
// lost, the closing layer shows from its start.
async function finish(code?: number, lost = false) {
  if (ending) return;
  ending = true;
  if (!lost) beginClosing();
  stopRoom();
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
      new Promise((resolve) =>
        setTimeout(resolve, Math.max(0, closingAt + closingShown - Date.now())),
      ),
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

watchTerminal((_why, lost) => void finish(undefined, lost));
renderer.on("selection", (selection: { getSelectedText?: () => string } | null) => {
  const text = selection?.getSelectedText?.() ?? "";
  if (text) renderer.copyToClipboardOSC52(text);
});
await render(() => <App />, renderer);
stopRoom = pollRoom(rewake, room, takeSpaces, () => main?.child.pid, showSessions);
