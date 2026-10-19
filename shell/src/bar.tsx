// The two bottom lines (docs/v2/shell.md, "The bar"): a button for main and
// for each worker, the focused one filled in its colour and main's colour
// apart from the workers'; under them the hints for the current focus.
import { TextAttributes } from "@opentui/core";
import { For, Show } from "solid-js";
import { type Context, hintsFor } from "./hints.ts";
import { ink, inkOr, type Theme } from "./theme.ts";

export type Tab = { id: string; label: string; main: boolean };

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
    <box flexDirection="row" height={1} width="100%" overflow="hidden">
      <For each={props.tabs}>
        {(t) => (
          <text
            height={1}
            marginRight={1}
            flexShrink={0}
            {...style(t)}
            content={` ${t.label} `}
            onMouseDown={() => props.pick(t.id)}
          />
        )}
      </For>
      <box flexGrow={1} />
      <Show when={props.zoomed}>
        <text height={1} flexShrink={0} fg={props.theme.text} content=" zoomed " />
      </Show>
    </box>
  );
}

export function Hints(props: { context: Context; theme: Theme; accent: string }) {
  return (
    <text height={1} width="100%" wrapMode="none" fg={props.theme.text}>
      <For each={hintsFor(props.context)}>
        {(h, i) => (
          <>
            {i() > 0 ? "   " : " "}
            <b style={{ fg: props.accent }}>{h.keys}</b>
            {` ${h.does}`}
          </>
        )}
      </For>
    </text>
  );
}
