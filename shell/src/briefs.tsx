// The briefs view (docs/shell.md, "The briefs"): a list of the room's
// briefs, and one opened as its markdown with the comments under it, shown in
// the worker's place, and its strip. The person only reads.
import { type CliRenderer, RGBA, type ScrollBoxRenderable, SyntaxStyle } from "@opentui/core";
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
import { ShellRow, selecting, separator } from "./bar.tsx";
import {
  type Answer,
  type Detail,
  ended,
  poll,
  readBrief,
  readList,
  rowOf,
  type Summary,
  stripOfBriefs,
  timeOf,
  type Unreadable,
  unreadableLine,
} from "./briefs.ts";
import type { Theme } from "./theme.ts";

// What the view shows outlives its mounting, as a worker's screen outlives
// its pane being hidden: the brief opened, the row chosen, the last answers,
// which the strip reads too.
const [opened, setOpened] = createSignal<number>();
const [list, setList] = createSignal<Summary[]>([]);
const [lost, setLost] = createSignal<Unreadable[]>([]);
const [detail, setDetail] = createSignal<Detail>();
const [at, setAt] = createSignal<number>();

// briefOpen is one brief showing, not the list, for the hints.
export const briefOpen = () => opened() !== undefined;

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

// BriefsView is the briefs in a pane width by height; the keys reach it only
// while active, as a worker's reach its terminal only while it has the focus.
// choose gives it the focus: a press on a row calls it before the row opens,
// since the opened brief unmounts the row and the press bubbles no further.
// Its scrollboxes take no focus, so a press never gives them the keys too.
export function BriefsView(props: {
  rewake: string;
  room: string;
  theme: Theme;
  width: number;
  height: number;
  active: () => boolean;
  choose: () => void;
}) {
  const renderer = useRenderer();
  const theme = props.theme;
  const style = markdownStyle(renderer, theme);
  onCleanup(() => style.destroy());
  const [problem, setProblem] = createSignal<string>();
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
            (a) =>
              keep(a, (v) => {
                setList(v.briefs);
                setLost(v.unreadable);
              }),
          )
        : poll(
            () => readBrief(props.rewake, props.room, id),
            (a) => keep(a, setDetail),
          );
    onCleanup(stop);
  });

  const open = (id: number) => {
    if (detail()?.id !== id) setDetail(undefined);
    setProblem(undefined);
    setAt(id);
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
    if (!props.active()) return;
    if (opened() === undefined) {
      if (key.name === "up") move(-1);
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
  return (
    <box width={props.width} height={props.height} flexDirection="column">
      <Show
        when={opened() === undefined}
        fallback={
          <Show when={detail()} fallback={<text fg={faint} content="reading the brief…" />}>
            {(d: Accessor<Detail>) => (
              <>
                <text fg={theme.text} height={1} wrapMode="none" attributes={1}>
                  {d().title}
                  <span style={{ fg: faint }}>
                    {`${d().closed ? " · closed" : ""}${d()
                      .bound.map(
                        (b) => ` · ${b.name}${ended(d(), b) && !d().closed ? " (ended)" : ""}`,
                      )
                      .join("")}`}
                  </span>
                </text>
                <scrollbox
                  ref={(s: ScrollBoxRenderable) => (scroll = s)}
                  flexGrow={1}
                  focusable={false}
                >
                  <markdown content={d().text} syntaxStyle={style} fg={theme.text} />
                  <text fg={faint} content={"─".repeat(props.width)} height={1} />
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
        <scrollbox ref={(s: ScrollBoxRenderable) => (rows = s)} flexGrow={1} focusable={false}>
          <Show
            when={list().length > 0 || lost().length > 0}
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
                  {...{
                    onMouseDown: () => {
                      props.choose();
                      open(brief.id);
                    },
                  }}
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
            <For each={lost()}>
              {(u) => <text height={1} wrapMode="none" fg={faint} content={unreadableLine(u)} />}
            </For>
          </Show>
        </scrollbox>
      </Show>
      <Show when={problem()}>
        {(line: Accessor<string>) => (
          <text height={1} wrapMode="none" fg={theme.main} content={line()} />
        )}
      </Show>
    </box>
  );
}

// BriefsStrip is the strip under the briefs, a shell's row as a worker's
// strip is: what shows, the list or one brief, in place of a session.
export function BriefsStrip(props: { width: number; theme: Theme; pick: () => void }) {
  const plain = RGBA.defaultForeground();
  const parts = () => {
    const id = opened();
    const width = Math.max(props.width - 2, 1);
    return stripOfBriefs(id === undefined ? { list: list() } : { id, detail: detail() }, width);
  };
  return (
    <ShellRow theme={props.theme} onMouseDown={props.pick}>
      <text height={1} wrapMode="none" flexGrow={1} fg={plain} {...selecting(props.theme)}>
        <For each={parts()}>
          {(part, i) =>
            i() === 0 ? (
              <b>{part}</b>
            ) : (
              <>
                <span style={{ fg: props.theme.faint }}>{separator}</span>
                {part}
              </>
            )
          }
        </For>
      </text>
    </ShellRow>
  );
}
