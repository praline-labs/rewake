// The navigation's two lines (docs/v2/shell.md, "The bar"): a button for
// main and for each worker, the focused one filled in its colour and main's
// colour apart from the workers'; and the hints for the current focus. Both,
// and the strip, are the shell's own rows, drawn alike by ShellRow.
import { RGBA, TextAttributes } from "@opentui/core";
import { For, type JSX, Show } from "solid-js";
import { type Context, hintsFor } from "./hints.ts";
import { ink, inkOr, type Theme } from "./theme.ts";

export type Tab = { id: string; label: string; main: boolean };

// separator parts the hints as it parts the strip.
export const separator = " · ";

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
  pick: (id: string) => void;
}) {
  const colour = (t: Tab) => (t.main ? props.theme.main : props.theme.worker);
  const style = (t: Tab) => {
    if (t.id === props.focused) {
      const bg = colour(t);
      return { bg, fg: ink(bg), attributes: TextAttributes.BOLD };
    }
    const bg = props.theme.idle;
    const marked = t.id === props.chosen ? TextAttributes.UNDERLINE : 0;
    return { bg, fg: inkOr(colour(t), bg), attributes: marked };
  };
  return (
    <ShellRow theme={props.theme}>
      <For each={props.tabs}>
        {(t) => (
          <text
            height={1}
            marginRight={1}
            flexShrink={0}
            {...style(t)}
            {...selecting(props.theme)}
            content={` ${t.label} `}
            onMouseDown={() => props.pick(t.id)}
          />
        )}
      </For>
      <box flexGrow={1} />
      <Show when={props.zoomed}>
        <text
          height={1}
          flexShrink={0}
          fg={RGBA.defaultForeground()}
          {...selecting(props.theme)}
          content="zoomed"
        />
      </Show>
    </ShellRow>
  );
}

export function Hints(props: { context: Context; theme: Theme; accent: string }) {
  const hints = () => hintsFor(props.context);
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
