// rewake's shell (docs/shell.md): main on the left, from 120 columns the
// chosen worker, or the briefs, on the right, each pane with its status line under it; the
// key hints under main's, the bar choosing the worker under the worker's.
// Below 120 columns the shown pane takes the whole width, the hints and the
// bar under its status, and the bar switches the whole window.
// Started by rewake --shell as
//   rewake-shell --room <room> -- <main's command line>

import { createCliRenderer } from "@opentui/core";
import { render, useTerminalDimensions } from "@opentui/solid";
import { createEffect, createSignal, For, Show } from "solid-js";
import { Bar, Hints } from "./bar.tsx";
import { BriefsStrip, BriefsView, briefOpen } from "./briefs.tsx";
import { Closing } from "./closing.tsx";
import { askColors, seedOf } from "./colors.ts";
import { endMain, tailText, watchTerminal } from "./ending.ts";
import { type Command, isSuspend, prefixHandler } from "./keys.ts";
import { layoutOf } from "./layout.ts";
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
import { pollRoom, type Space, spacesOver } from "./room.ts";
import { Scanner } from "./scan.ts";
import { connectWorker, type MainRun, mainEnv, type Source, startMain } from "./sources.ts";
import { mainStrip, Strip, showSessions, showSpaces, stripOf } from "./strip.tsx";
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
  // On the next tick: a pane shown by this change is mounted only then.
  setTimeout(() => pane?.term?.focus(), 0);
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
  const lay = () =>
    layoutOf(dims().width, dims().height, {
      chosen: chosen() !== undefined,
      zoomed: zoomed(),
      workers: workers().length,
    });
  const wide = () => lay().wide;
  const shown = (p: Pane) => (wide() ? p === mainPane || p.id === chosen() : p.id === focused());
  // The briefs show in the worker's place, chosen and focused as a worker is.
  const briefsShown = () => (wide() ? chosen() : focused()) === briefs;
  const widthOf = (p: Pane) => (p === mainPane ? lay().main : lay().worker);
  const rowHeight = () => lay().rowHeight;
  const height = () => lay().height;
  // A pane sits in a box with a column of padding on each side, so the text
  // does not touch the edge; the widget's size, and so the program's, is the
  // inside. A press on the padding chooses the pane like one on its text.
  const padded = (id: string, width: number) => ({
    width,
    height: height(),
    paddingLeft: 1,
    paddingRight: 1,
    backgroundColor: seed.transparent ? undefined : theme.background,
    onMouseDown: () => paneHooks.pressed(id),
  });
  const inner = (p: Pane) => Math.max(widthOf(p) - 2, 1);
  const focusedPane = () => paneOf(focused()) ?? mainPane;
  const onBriefs = () => focused() === briefs;
  const bar = () => (
    <Bar
      tabs={workers().map((p) => ({ id: p.id, label: p.label }))}
      focused={focused()}
      chosen={wide() && focused() === "main" ? chosen() : undefined}
      zoomed={zoomed()}
      theme={theme}
      pick={show}
      briefs={briefs}
    />
  );
  const hints = () => (
    <Hints
      context={{
        focus: onBriefs() ? "briefs" : focusedPane() === mainPane ? "main" : "worker",
        name: onBriefs() ? "briefs" : focusedPane().label,
        brief: briefOpen(),
        prefix: prefixWaits(),
        zoomed: zoomed(),
        workers: workers().length,
      }}
      width={wide() ? lay().main : dims().width}
      theme={theme}
      accent={onBriefs() ? theme.text : focusedPane() === mainPane ? theme.main : theme.worker}
    />
  );
  // The work is hidden as a whole under the closing layer, so a terminal
  // background the shell does not know never lets a pane show through.
  return (
    <box width="100%" height="100%">
      <box flexDirection="column" width="100%" height="100%" visible={!closing()}>
        <box flexDirection="row" height={rowHeight()}>
          <box flexDirection="column" visible={shown(mainPane)} width={widthOf(mainPane)}>
            <box {...padded(mainPane.id, widthOf(mainPane))}>
              <pane_terminal
                id="main"
                ref={(t: PaneTerminal) => startMainPane(t)}
                transparentBackground={seed.transparent}
                width={inner(mainPane)}
                height={height()}
              />
            </box>
            <Strip
              state={mainStrip}
              width={widthOf(mainPane)}
              colour={theme.main}
              theme={theme}
              pick={() => paneHooks.pressed("main")}
            />
            <Show when={wide()}>{hints()}</Show>
          </box>
          <Show when={wide()}>
            <box width={1} height={rowHeight()} backgroundColor="#444444" />
          </Show>
          <box
            flexDirection="column"
            visible={workers().some(shown) || briefsShown()}
            width={lay().worker}
            height={rowHeight()}
          >
            <For each={workers()}>
              {(pane) => (
                <box visible={shown(pane)} flexDirection="column" width={widthOf(pane)}>
                  <box {...padded(pane.id, widthOf(pane))}>
                    <pane_terminal
                      id={pane.id}
                      ref={(t: PaneTerminal) => startWorkerPane(pane, t)}
                      transparentBackground={seed.transparent}
                      width={inner(pane)}
                      height={height()}
                    />
                  </box>
                  <Strip
                    state={() => stripOf(pane.id)}
                    width={widthOf(pane)}
                    colour={theme.worker}
                    theme={theme}
                    pick={() => paneHooks.pressed(pane.id)}
                  />
                </box>
              )}
            </For>
            <Show when={briefsShown() && !closing()}>
              <box flexDirection="column" width={lay().worker}>
                <box {...padded(briefs, lay().worker)}>
                  <BriefsView
                    rewake={rewake}
                    room={room}
                    theme={theme}
                    width={Math.max(lay().worker - 2, 1)}
                    height={height()}
                    active={onBriefs}
                    choose={() => paneHooks.pressed(briefs)}
                  />
                </box>
                <BriefsStrip
                  width={lay().worker}
                  theme={theme}
                  pick={() => paneHooks.pressed(briefs)}
                />
              </box>
            </Show>
            <Show when={wide() && workers().length > 0}>{bar()}</Show>
          </box>
        </box>
        <Show when={!wide()}>
          {hints()}
          <Show when={workers().length > 0}>{bar()}</Show>
        </Show>
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

watchTerminal((_why, lost) => void finish(undefined, lost));
renderer.on("selection", (selection: { getSelectedText?: () => string } | null) => {
  const text = selection?.getSelectedText?.() ?? "";
  if (text) renderer.copyToClipboardOSC52(text);
});
await render(() => <App />, renderer);
stopRoom = pollRoom(
  rewake,
  room,
  (spaces, ended) => takeSpaces(spaces, ended, closing, beginClosing),
  () => main?.child.pid,
  showSessions,
);
