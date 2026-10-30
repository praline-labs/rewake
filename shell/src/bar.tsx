// The navigation's two lines (docs/shell.md, "The bar"): a button for
// each worker, the focused one filled in the workers' colour — main has none,
// the prefix reaches it; and the hints for the current focus. Both, and the
// strips, are the shell's own rows, drawn alike by ShellRow.
import { RGBA, TextAttributes } from "@opentui/core";
import { createEffect, createMemo, createSignal, For, type JSX, onCleanup } from "solid-js";
import { type Context, fitHints, hintsFor, separator } from "./hints.ts";
import type { Mark } from "./strip.tsx";
import { ink, inkOr, type Theme } from "./theme.ts";

export { separator };

export type Tab = { id: string; label: string };

// blinkPhase is how long a blinking edge stays lit, then dark: about once a
// second.
const blinkPhase = 500;

// ShellRow is one of the shell's own rows: the band behind it, the panes'
// column of padding on each side, its content from the left.
export function ShellRow(props: { theme: Theme; children: JSX.Element; onMouseDown?: () => void }) {
  return (
    <box
      flexDirection="row"
      height={1}
      flexShrink={0}
      width="100%"
      overflow="hidden"
      paddingLeft={1}
      paddingRight={1}
      onMouseDown={props.onMouseDown}
      {...(props.theme.band ? { backgroundColor: props.theme.band } : {})}
    >
      {props.children}
    </box>
  );
}

// selecting is a text's selection in the panes' one colour, so a selection
// over the shell's rows does not show each part in its own.
export const selecting = (theme: Theme) => ({
  selectionBg: theme.selection.bg,
  selectionFg: theme.selection.fg,
});

export function Bar(props: {
  tabs: Tab[];
  focused: string;
  // chosen is the worker shown beside main, marked while main has the focus.
  chosen?: string;
  zoomed: boolean;
  theme: Theme;
  // pick chooses a worker by its id, or the briefs by theirs.
  pick: (id: string) => void;
  briefs: string;
  // mark is what a worker's activity paints its button's edge with.
  mark: (id: string) => Mark;
}) {
  // One clock for every blinking edge, running only while one blinks: a
  // redraw per phase, none while nothing blinks.
  const [lit, setLit] = createSignal(true);
  const blinking = createMemo(() => props.tabs.some((t) => props.mark(t.id) === "blink"));
  createEffect(() => {
    if (!blinking()) return;
    setLit(true);
    const timer = setInterval(() => setLit((on) => !on), blinkPhase);
    onCleanup(() => clearInterval(timer));
  });
  // The edge is the button's first cell, its padding, so a mark going on or
  // off moves nothing: lit, or the button's own background.
  const edge = (t: Tab, bg: string) => {
    const mark = props.mark(t.id);
    return mark === "steady" || (mark === "blink" && lit()) ? props.theme.edge : bg;
  };
  const style = (t: Tab) => {
    if (t.id === props.focused) {
      const bg = props.theme.worker;
      return { bg, fg: ink(bg), attributes: TextAttributes.BOLD };
    }
    const bg = props.theme.idle;
    const marked = t.id === props.chosen ? TextAttributes.UNDERLINE : 0;
    return { bg, fg: inkOr(props.theme.worker, bg), attributes: marked };
  };
  // The briefs name no program: filled in the terminal's own foreground
  // while they have the keys, underlined while shown beside main.
  const briefsStyle = () => {
    if (props.focused === props.briefs) {
      const bg = props.theme.text;
      return { bg, fg: ink(bg), attributes: TextAttributes.BOLD };
    }
    const marked = props.chosen === props.briefs ? TextAttributes.UNDERLINE : 0;
    return { bg: props.theme.idle, fg: RGBA.defaultForeground(), attributes: marked };
  };
  // The workers' buttons from the left, the briefs' at the right edge (the
  // owner's word, October 30, 2026): where they do not all fit, the workers'
  // row is cut a cell before the right end, so the briefs button stays whole
  // and no button lies over another; the prefix's digits reach a cut worker.
  return (
    <ShellRow theme={props.theme}>
      <box flexDirection="row" flexGrow={1} flexShrink={1} overflow="hidden">
        <For each={props.tabs}>
          {(t) => (
            <text
              height={1}
              marginRight={1}
              flexShrink={0}
              wrapMode="none"
              {...style(t)}
              {...selecting(props.theme)}
              onMouseDown={() => props.pick(t.id)}
            >
              <span style={{ bg: edge(t, style(t).bg) }}> </span>
              {`${t.label} `}
            </text>
          )}
        </For>
      </box>
      <text
        height={1}
        width={props.zoomed ? 6 : 0}
        marginLeft={props.zoomed ? 1 : 0}
        flexShrink={0}
        fg={RGBA.defaultForeground()}
        {...selecting(props.theme)}
        content={props.zoomed ? "zoomed" : ""}
      />
      <text
        height={1}
        marginLeft={1}
        flexShrink={0}
        {...briefsStyle()}
        {...selecting(props.theme)}
        content=" briefs "
        onMouseDown={() => props.pick(props.briefs)}
      />
    </ShellRow>
  );
}

// Hints is the hints row for a column width wide.
export function Hints(props: { context: Context; width: number; theme: Theme; accent: string }) {
  const hints = () => fitHints(hintsFor(props.context), Math.max(props.width - 2, 1));
  const plain = RGBA.defaultForeground();
  return (
    <ShellRow theme={props.theme}>
      <text height={1} wrapMode="none" flexGrow={1} fg={plain} {...selecting(props.theme)}>
        <For each={hints()}>
          {(h, i) => (
            <>
              {i() > 0 ? <span style={{ fg: props.theme.faint }}>{separator}</span> : ""}
              <b style={{ fg: props.accent }}>{h.keys}</b>
              {` ${h.does}`}
            </>
          )}
        </For>
      </text>
    </ShellRow>
  );
}
