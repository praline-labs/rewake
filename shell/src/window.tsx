// The shell's window: main's column, the chosen worker's or the briefs, the
// strips, the hints and the bar (docs/shell.md, "The bar"). One drawing for
// two trees: the person's screen, and the shadow tree rewake screen takes its
// pictures from, which draws the same window with another session in its
// place (docs/screen.md).

import { For, Show } from "solid-js";
import { Bar, Hints } from "./bar.tsx";
import { BriefsStrip, BriefsView, briefOpen } from "./briefs.tsx";
import { Closing } from "./closing.tsx";
import type { Seed } from "./colors.ts";
import { faceShown, modeOf } from "./face/faces.ts";
import { FaceView } from "./face/view.tsx";
import { layoutOf } from "./layout.ts";
import { type PaneTerminal, paneHooks } from "./pane.ts";
import { briefs, mainPane, type Pane, paneOf, show, workers } from "./panes.ts";
import { mainStrip, markOf, Strip, stripOf } from "./strip.tsx";
import type { Theme } from "./theme.ts";

// View is what decides how the window looks: who has the keys, which worker
// stands beside main, the zoom, the size, and whether the prefix waits.
export type View = {
  focused: string;
  chosen: string | undefined;
  zoomed: boolean;
  width: number;
  height: number;
  prefix: boolean;
};

export type Env = {
  // live is the person's window; the shadow one has no program to feed, no
  // briefs and no closing layer, and keeps the terminals of its own.
  live: boolean;
  seed: Seed;
  theme: Theme;
  rewake: string;
  room: string;
  closing: () => unknown;
  // The terminals' refs: the live window starts the programs, the shadow
  // one keeps its widgets beside the panes' for the bytes they are fed.
  mainRef: (t: PaneTerminal) => void;
  workerRef: (pane: Pane, t: PaneTerminal) => void;
};

export function Window(props: { env: Env; view: () => View }) {
  const { seed, theme, live } = props.env;
  const view = props.view;
  const lay = () =>
    layoutOf(view().width, view().height, {
      chosen: view().chosen !== undefined,
      zoomed: view().zoomed,
      workers: workers().length,
    });
  const wide = () => lay().wide;
  const shown = (p: Pane) =>
    wide() ? p === mainPane || p.id === view().chosen : p.id === view().focused;
  // The briefs show in the worker's place, chosen and focused as a worker is.
  const briefsShown = () => live && (wide() ? view().chosen : view().focused) === briefs;
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
  const focusedPane = () => paneOf(view().focused) ?? mainPane;
  const onBriefs = () => view().focused === briefs;
  const bar = () => (
    <Bar
      tabs={workers().map((p) => ({ id: p.id, label: p.label }))}
      focused={view().focused}
      chosen={wide() && view().focused === "main" ? view().chosen : undefined}
      zoomed={view().zoomed}
      theme={theme}
      pick={show}
      briefs={briefs}
      mark={(id) => markOf(stripOf(id).status)}
    />
  );
  const hints = () => (
    <Hints
      context={{
        focus: onBriefs() ? "briefs" : focusedPane() === mainPane ? "main" : "worker",
        name: onBriefs() ? "briefs" : focusedPane().label,
        brief: live && briefOpen(),
        prefix: view().prefix,
        zoomed: view().zoomed,
        workers: workers().length,
        face: onBriefs() ? undefined : modeOf(focusedPane().id),
      }}
      width={wide() ? lay().main : view().width}
      theme={theme}
      accent={onBriefs() ? theme.text : focusedPane() === mainPane ? theme.main : theme.worker}
    />
  );
  // The work is hidden as a whole under the closing layer, so a terminal
  // background the shell does not know never lets a pane show through.
  return (
    <box width="100%" height="100%">
      <box flexDirection="column" width="100%" height="100%" visible={!props.env.closing()}>
        <box flexDirection="row" height={rowHeight()}>
          <box flexDirection="column" visible={shown(mainPane)} width={widthOf(mainPane)}>
            <box {...padded(mainPane.id, widthOf(mainPane))}>
              <pane_terminal
                id="main"
                ref={(t: PaneTerminal) => props.env.mainRef(t)}
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
                      ref={(t: PaneTerminal) => props.env.workerRef(pane, t)}
                      transparentBackground={seed.transparent}
                      width={inner(pane)}
                      height={height()}
                    />
                    {/* Over the native face, which stays mounted and fed under it. */}
                    <Show when={faceShown(pane.id)}>
                      <FaceView
                        id={pane.id}
                        width={inner(pane)}
                        height={height()}
                        theme={theme}
                        live={live}
                        active={() => live && view().focused === pane.id && !props.env.closing()}
                      />
                    </Show>
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
            <Show when={briefsShown() && !props.env.closing()}>
              <box flexDirection="column" width={lay().worker}>
                <box {...padded(briefs, lay().worker)}>
                  <BriefsView
                    rewake={props.env.rewake}
                    room={props.env.room}
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
      <Show when={props.env.closing()}>
        <Closing theme={theme} />
      </Show>
    </box>
  );
}
