// The briefs modal (docs/shell.md, "The briefs"): a list of the room's
// briefs, and one opened as its markdown with the comments under it. The
// person only reads; the panes keep drawing under it.
import { type CliRenderer, type ScrollBoxRenderable, SyntaxStyle } from "@opentui/core";
import { useKeyboard, useRenderer } from "@opentui/solid";
import {
  type Accessor,
  createEffect,
  createMemo,
  createSignal,
  For,
  onCleanup,
  Show,
} from "solid-js";
import {
  type Answer,
  type Detail,
  ended,
  poll,
  readBrief,
  readList,
  rowOf,
  type Summary,
  timeOf,
} from "./briefs.ts";
import type { Theme } from "./theme.ts";

const fallbackBackground = "#1c1c1c";

// markdownStyle is the markdown's look in the shell's own colours.
function markdownStyle(renderer: CliRenderer, theme: Theme) {
  const bold = { fg: theme.main, bold: true };
  return SyntaxStyle.fromStyles(
    {
      default: { fg: theme.text },
      "markup.heading": bold,
      "markup.heading.1": bold,
      "markup.heading.2": bold,
      "markup.heading.3": bold,
      "markup.strong": { bold: true },
      "markup.italic": { italic: true },
      "markup.strikethrough": { dim: true },
      "markup.raw": { fg: theme.worker },
      "markup.list": { fg: theme.main },
      "markup.quote": { fg: theme.faint, italic: true },
      "markup.link": { fg: theme.main, underline: true },
      "markup.link.label": { fg: theme.main, underline: true },
      "markup.link.url": { fg: theme.faint },
    },
    renderer.nativeScene,
  );
}

export function BriefsModal(props: {
  rewake: string;
  room: string;
  theme: Theme;
  left: number;
  top: number;
  width: number;
  height: number;
  close: () => void;
}) {
  const renderer = useRenderer();
  const theme = props.theme;
  const style = markdownStyle(renderer, theme);
  onCleanup(() => style.destroy());
  const [opened, setOpened] = createSignal<number>();
  const [list, setList] = createSignal<Summary[]>([]);
  const [detail, setDetail] = createSignal<Detail>();
  const [problem, setProblem] = createSignal<string>();
  const [at, setAt] = createSignal<number>();
  let scroll: ScrollBoxRenderable | undefined;
  let rows: ScrollBoxRenderable | undefined;

  const selected = createMemo(() => {
    const briefs = list();
    const i = briefs.findIndex((b) => b.id === at());
    return i >= 0 ? i : 0;
  });

  // Polling lasts as long as what shows: the list, or the opened brief.
  createEffect(() => {
    const id = opened();
    const keep = <T,>(answer: Answer<T>, set: (v: T) => void) => {
      if (answer.ok) {
        setProblem(undefined);
        set(answer.value);
      } else setProblem(answer.line);
    };
    const stop =
      id === undefined
        ? poll(
            () => readList(props.rewake, props.room),
            (a) => keep(a, setList),
          )
        : poll(
            () => readBrief(props.rewake, props.room, id),
            (a) => keep(a, setDetail),
          );
    onCleanup(stop);
  });

  const open = (id: number) => {
    setDetail(undefined);
    setProblem(undefined);
    setOpened(id);
  };
  const back = () => {
    setProblem(undefined);
    setOpened(undefined);
  };
  const move = (by: number) => {
    const briefs = list();
    const next = briefs[Math.max(0, Math.min(briefs.length - 1, selected() + by))];
    if (!next) return;
    setAt(next.id);
    rows?.scrollChildIntoView(`brief-${next.id}`);
  };

  useKeyboard((key) => {
    if (opened() === undefined) {
      if (key.name === "escape") props.close();
      else if (key.name === "up") move(-1);
      else if (key.name === "down") move(1);
      else if (key.name === "return") {
        const brief = list()[selected()];
        if (brief) open(brief.id);
      }
      return;
    }
    if (key.name === "escape" || key.name === "backspace") back();
    else if (key.name === "up") scroll?.scrollBy(-1);
    else if (key.name === "down") scroll?.scrollBy(1);
    else if (key.name === "pageup") scroll?.scrollBy(-1, "viewport");
    else if (key.name === "pagedown") scroll?.scrollBy(1, "viewport");
    else if (key.name === "home") scroll?.scrollTo(0);
    else if (key.name === "end") scroll?.scrollTo(scroll.scrollHeight);
  });

  const faint = theme.faint;
  const inner = () => Math.max(props.width - 4, 1);
  return (
    <box
      position="absolute"
      left={props.left}
      top={props.top}
      zIndex={50}
      width={props.width}
      height={props.height}
      flexDirection="column"
      border
      borderStyle="single"
      borderColor={faint}
      title=" briefs "
      titleAlignment="left"
      paddingLeft={1}
      paddingRight={1}
      backgroundColor={theme.background ?? fallbackBackground}
    >
      <Show
        when={opened() === undefined}
        fallback={
          <Show when={detail()} fallback={<text fg={faint} content="reading the brief…" />}>
            {(d: Accessor<Detail>) => (
              <>
                <text fg={theme.text} height={1} wrapMode="none" attributes={1}>
                  {`${d().id} · ${d().title}`}
                  <span style={{ fg: faint }}>
                    {` · ${d().done}/${d().total}${d().closed ? " · closed" : ""}${d()
                      .bound.map(
                        (b) => ` · ${b.name}${ended(d(), b) && !d().closed ? " (ended)" : ""}`,
                      )
                      .join("")}`}
                  </span>
                </text>
                <scrollbox ref={(s: ScrollBoxRenderable) => (scroll = s)} flexGrow={1}>
                  <markdown content={d().text} syntaxStyle={style} fg={theme.text} />
                  <text fg={faint} content={"─".repeat(inner())} height={1} />
                  <Show
                    when={d().comments.length > 0}
                    fallback={<text fg={faint} content="no comments" />}
                  >
                    <For each={d().comments}>
                      {(c) => (
                        <box flexDirection="column" marginBottom={1}>
                          <text fg={theme.text} attributes={1} wrapMode="none">
                            {c.from}
                            <span style={{ fg: faint }}>{` · ${timeOf(c.at)}`}</span>
                          </text>
                          <text fg={theme.text} wrapMode="word" content={c.text} />
                        </box>
                      )}
                    </For>
                  </Show>
                </scrollbox>
              </>
            )}
          </Show>
        }
      >
        <scrollbox ref={(s: ScrollBoxRenderable) => (rows = s)} flexGrow={1}>
          <Show
            when={list().length > 0}
            fallback={
              <text fg={faint} content="no briefs; main makes them with rewake brief new" />
            }
          >
            <For each={list()}>
              {(brief, i) => (
                <text
                  id={`brief-${brief.id}`}
                  height={1}
                  wrapMode="none"
                  fg={i() === selected() ? theme.selection.fg : theme.text}
                  bg={i() === selected() ? theme.selection.bg : undefined}
                  {...{ onMouseDown: () => open(brief.id) }}
                >
                  <For each={rowOf(brief)}>
                    {(p) => (
                      <span style={{ fg: p.faint && i() !== selected() ? faint : undefined }}>
                        {p.text}
                      </span>
                    )}
                  </For>
                </text>
              )}
            </For>
          </Show>
        </scrollbox>
      </Show>
      <text
        height={1}
        wrapMode="none"
        fg={problem() ? theme.main : faint}
        content={
          problem() ??
          (opened() === undefined
            ? "↑↓ choose · Enter open · Esc close"
            : "↑↓ PgUp PgDn Home End scroll · Esc back")
        }
      />
    </box>
  );
}
