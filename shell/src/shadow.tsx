// The shadow window: the same drawing in a tree of its own that is never put
// on the person's screen, with a widget beside every pane's that is fed the
// same bytes. rewake screen asks it for a window with another session in
// its place, so nothing on the person's screen changes (docs/screen.md).
import type { CliRenderer, TerminalColors } from "@opentui/core";
import { testRender } from "@opentui/solid";
import { createEffect, createSignal } from "solid-js";
import { faceShown } from "./face/faces.ts";
import type { PaneTerminal } from "./pane.ts";
import { chosen, mainPane, paneOf, workers, zoomed } from "./panes.ts";
import { type Frame, frameOf } from "./screen.ts";
import { type Env, type View, Window } from "./window.tsx";

type Base = Pick<Env, "seed" | "theme" | "rewake" | "room">;

// shadowOf builds the shadow window beside renderer's and returns what takes
// a picture of it, one at a time: the shadow window has one state.
export async function shadowOf(
  renderer: CliRenderer,
  base: Base,
  colors: TerminalColors | undefined,
): Promise<(id: string) => Promise<Frame>> {
  const [shadowView, setShadowView] = createSignal<View>({
    focused: "main",
    chosen: undefined,
    zoomed: false,
    width: renderer.width,
    height: renderer.height,
    prefix: false,
  });
  const seedShadow = (t: PaneTerminal) => {
    if (base.seed.widget) t.write(base.seed.widget);
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

  let drawing: Promise<unknown> = Promise.resolve();
  return (id) => {
    const next = drawing.then(() => draw(id));
    drawing = next.catch(() => {});
    return next;
  };
}
