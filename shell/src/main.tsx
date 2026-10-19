// rewake's shell (docs/v2/shell.md): main on the left, from 120 columns the
// chosen worker on the right, a bar below choosing the worker; below 120
// columns the bar switches the whole window. Started by rewake --shell as
//   rewake-shell --room <room> -- <main's command line>

import {
  createCliRenderer,
  type EmbeddedTerminalOptions,
  EmbeddedTerminalRenderable,
  type RenderContext,
} from "@opentui/core";
import { extend, render, useTerminalDimensions } from "@opentui/solid";
import { createEffect, createSignal, For, Show } from "solid-js";
import { seedOf } from "./colors.ts";
import { endMain, tailText, watchTerminal } from "./ending.ts";
import { type Command, prefixHandler } from "./keys.ts";
import { pollRoom, type Space } from "./room.ts";
import { Scanner } from "./scan.ts";
import { connectWorker, type MainRun, type Source, startMain } from "./sources.ts";

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

// Solid makes a renderable with { id } only, so what the widget reads only
// in its constructor is fixed here: assigning onMouseDown later replaces the
// widget's own mouse forwarding.
// The id is read at the press: Solid sets the id prop after construction.
let pressed: (id: string) => void = () => {};
class PaneTerminal extends EmbeddedTerminalRenderable {
  constructor(ctx: RenderContext, options: EmbeddedTerminalOptions) {
    const self: { term?: PaneTerminal } = {};
    super(ctx, {
      ...options,
      maxScrollback: 10_000,
      onMouseDown: () => {
        if (self.term) pressed(self.term.id);
      },
    });
    self.term = this;
  }
}
extend({ pane_terminal: PaneTerminal });
declare module "@opentui/solid" {
  interface OpenTUIComponents {
    pane_terminal: typeof PaneTerminal;
  }
}

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
  prependInputHandlers: [prefixHandler(run)],
});
const colors = await renderer.getPalette({ size: 16, timeout: 500 }).catch(() => undefined);
const seed = seedOf(colors);

// feed passes a program's bytes to its pane, after the scanner took what
// the widget would swallow.
function feed(pane: Pane, data: Uint8Array) {
  for (const seen of pane.scanner.scan(data)) {
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
  term.onData = (data) => main?.write(data);
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
  term.write("\x1bc" + seed.widget);
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

// The room's spaces become the worker panes, in the order they opened.
function takeSpaces(spaces: Space[]) {
  const was = workers();
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
  const id = focused();
  for (const pane of allPanes()) if (pane.id !== id) pane.term?.blur();
  const pane = paneOf(id);
  // On the next tick: a pane shown by this change is mounted only then.
  setTimeout(() => pane?.term?.focus(), 0);
  if (pane?.title) renderer.setTerminalTitle(pane.title);
});

function run(c: Command) {
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

pressed = (id) => {
  if (id !== focused()) show(id);
};

function Bar(props: { width: number }) {
  const tabs = () =>
    allPanes().map((p, i) => {
      const label = ` ${i} ${p.label} `;
      return p.id === focused() ? `[${label.trim()}]` : label;
    });
  const hint = () => `  Ctrl+\\ n p 0-9 ← → z${zoomed() ? "  zoomed" : ""}`;
  // A click on a tab shows it.
  const click = (x: number) => {
    let col = 0;
    tabs().forEach((tab, i) => {
      if (x >= col && x < col + tab.length) show(allPanes()[i]?.id);
      col += tab.length + 1;
    });
  };
  return (
    <text
      height={1}
      width={props.width}
      content={(tabs().join(" ") + hint()).slice(0, props.width)}
      onMouseDown={(e: { x: number }) => click(e.x)}
    />
  );
}

function App() {
  const dims = useTerminalDimensions();
  const wide = () => dims().width >= 120 && chosen() !== undefined && !zoomed();
  const right = () => Math.floor((dims().width - 1) / 2);
  const shown = (p: Pane) => (wide() ? p === mainPane || p.id === chosen() : p.id === focused());
  const widthOf = (p: Pane) =>
    !wide() ? dims().width : p === mainPane ? dims().width - 1 - right() : right();
  const height = () => Math.max(dims().height - 1, 1);
  return (
    <box flexDirection="column" width="100%" height="100%">
      <box flexDirection="row" height={height()}>
        <pane_terminal
          id="main"
          ref={(t: PaneTerminal) => startMainPane(t)}
          visible={shown(mainPane)}
          width={widthOf(mainPane)}
          height={height()}
        />
        <Show when={wide()}>
          <box width={1} height={height()} backgroundColor="#444444" />
        </Show>
        <For each={workers()}>
          {(pane) => (
            <pane_terminal
              id={pane.id}
              ref={(t: PaneTerminal) => startWorkerPane(pane, t)}
              visible={shown(pane)}
              width={widthOf(pane)}
              height={height()}
            />
          )}
        </For>
      </box>
      <Bar width={dims().width} />
    </box>
  );
}

// finish ends the shell with main's code: main hung up and waited for, the
// workers' streams closed, the screen given back.
async function finish(code?: number) {
  if (ending) return;
  ending = true;
  stopRoom();
  for (const pane of workers()) pane.source?.close();
  if (main) code = await endMain(main.child, main.close);
  renderer.destroy();
  await Promise.race([renderer.closed, new Promise((resolve) => setTimeout(resolve, 1000))]);
  if (code !== 0 && process.stderr.isTTY) {
    const said = tailText(mainTail);
    if (said) process.stderr.write(said + "\n");
  }
  process.exit(code ?? 1);
}

watchTerminal(() => void finish());
renderer.on("selection", (selection: { getSelectedText?: () => string } | null) => {
  const text = selection?.getSelectedText?.() ?? "";
  if (text) renderer.copyToClipboardOSC52(text);
});
await render(() => <App />, renderer);
stopRoom = pollRoom(rewake, room, takeSpaces);
