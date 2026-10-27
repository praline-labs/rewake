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
  changes,
  cut,
  type Detail,
  detailOf,
  foldText,
  headOf,
  infoOf,
  type Listed,
  listOf,
  type NavItem,
  navOf,
  ordered,
  type Pick,
  pickOf,
  type Row,
  rowsOf,
  type Summary,
  stepOf,
  stripOfBriefs,
  timeOf,
  type Unreadable,
  unreadableLine,
} from "./briefs.ts";
import { type RoomView, type Want, wanting } from "./room.ts";
import { ink, type Theme } from "./theme.ts";

// What the view shows outlives its mounting, as a worker's screen outlives
// its pane being hidden: the brief opened, the row chosen, the last answers,
// which the strip reads too.
const [opened, setOpened] = createSignal<number>();
const [list, setList] = createSignal<Summary[]>([]);
const [lost, setLost] = createSignal<Unreadable[]>([]);
const [detail, setDetail] = createSignal<Detail>();
// The line shown instead when a view did not read, or came before main's run
// was known; the next good answer replaces it.
const [problem, setProblem] = createSignal<string>();
const [at, setAt] = createSignal<Pick>();
// The closed briefs unfolded under their row; kept while the shell runs.
const [unfolded, setUnfolded] = createSignal(false);

const listChanged = changes<Listed>();
const detailChanged = changes<Detail>();

// keep shows an answer when it is new: its value, or the line instead.
function keep<T>(answer: Answer<T>, changed: (a: Answer<T>) => boolean, set: (v: T) => void) {
  if (!changed(answer)) return;
  if (answer.ok) {
    setProblem(undefined);
    set(answer.value);
  } else setProblem(answer.line);
}

// takeBriefs is what each read of the room view says of the briefs, as far as
// that call asked for them: the list, and the brief opened. A view that did
// not read is a line, never a crash.
export function takeBriefs(view: RoomView | undefined, asked: Want) {
  if (!asked.briefs) return;
  if (!view) return setProblem("the room did not read");
  keep(listOf(view), listChanged, (v) => {
    setList(ordered(v.briefs));
    setLost(v.unreadable);
  });
  if (asked.brief !== undefined && asked.brief === opened())
    keep(detailOf(view, asked.brief), detailChanged, setDetail);
}

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
  let scroll: ScrollBoxRenderable | undefined;
  let scrollbox: ScrollBoxRenderable | undefined;

  const rows = createMemo(() => rowsOf(list(), unfolded()));
  const selected = createMemo(() => {
    const i = rows().findIndex((row) => pickOf(row) === at());
    return i >= 0 ? i : 0;
  });

  // The room view carries the list while this view shows, an opened brief's
  // navigation row being made of it, and the brief while it is open.
  createEffect(() => {
    wanting({ briefs: true, brief: opened() });
    onCleanup(() => wanting({ briefs: false }));
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
    const all = rows();
    const next = all[Math.max(0, Math.min(all.length - 1, selected() + by))];
    if (!next) return;
    setAt(pickOf(next));
    scrollbox?.scrollChildIntoView(`brief-${pickOf(next)}`);
  };
  const toggle = () => {
    setAt("fold");
    setUnfolded((v) => !v);
  };
  const act = (row: Row) => ("fold" in row ? toggle() : open(row.brief.id));

  // The text keeps a column clear of the scrollbar, as the panes keep one
  // from the screen's edge: the bar lies over the viewport's last column and
  // the content leaves two, so the text is as wide whether it shows or not.
  const inset = { paddingRight: 2 };
  const bar = { position: "absolute" as const, right: 0, top: 0, height: "100%" as const };
  const textWidth = () => Math.max(props.width - inset.paddingRight, 1);
  const nav = () => navOf(list(), opened(), props.width);
  const step = (by: number) => {
    const id = stepOf(nav(), opened(), by);
    if (id !== undefined) open(id);
  };

  useKeyboard((key) => {
    if (!props.active()) return;
    if (opened() === undefined) {
      if (key.name === "up") move(-1);
      else if (key.name === "down") move(1);
      else if (key.name === "return") {
        const row = rows()[selected()];
        if (row) act(row);
      }
      return;
    }
    if (key.name === "escape" || key.name === "backspace") back();
    else if (key.name === "left") step(-1);
    else if (key.name === "right") step(1);
    else if (key.name === "up") scroll?.scrollBy(-1);
    else if (key.name === "down") scroll?.scrollBy(1);
    else if (key.name === "pageup") scroll?.scrollBy(-1, "viewport");
    else if (key.name === "pagedown") scroll?.scrollBy(1, "viewport");
    else if (key.name === "home") scroll?.scrollTo(0);
    else if (key.name === "end") scroll?.scrollTo(scroll.scrollHeight);
  });

  const faint = theme.faint;
  // The shown brief in the row is filled as the briefs' button on the bar is
  // while it has the keys; the rest sit on the buttons' grey.
  const navStyle = (item: NavItem) =>
    item.id !== undefined && item.id === opened()
      ? { bg: theme.text, fg: ink(theme.text), attributes: 1 }
      : { bg: theme.idle, fg: RGBA.defaultForeground(), attributes: 0 };
  // A list entry's title is a button: the selected one in a pane's selection
  // colours, the others on the buttons' grey, a closed one's words faint.
  const headStyle = (faintText: boolean, chosen: boolean) =>
    chosen
      ? { bg: theme.selection.bg, fg: theme.selection.fg }
      : { bg: theme.idle, fg: faintText ? theme.faintOnIdle : RGBA.defaultForeground() };
  // The info line starts under the title, past the button's padding and its
  // number.
  const indentOf = (brief: Summary) => ` ${brief.id} · `.length;
  return (
    <box width={props.width} height={props.height} flexDirection="column">
      <Show
        when={opened() === undefined}
        fallback={
          <>
            <box flexDirection="row" height={1} flexShrink={0} overflow="hidden">
              <For each={nav()}>
                {(item: NavItem) => (
                  <text
                    height={1}
                    marginRight={1}
                    flexShrink={0}
                    wrapMode="none"
                    {...navStyle(item)}
                    {...selecting(theme)}
                    content={item.text}
                    {...{
                      onMouseDown: () => {
                        props.choose();
                        if (item.id === undefined) back();
                        else open(item.id);
                      },
                    }}
                  />
                )}
              </For>
            </box>
            <Show when={detail()} fallback={<text fg={faint} content="reading the brief…" />}>
              {(d: Accessor<Detail>) => (
                <scrollbox
                  ref={(s: ScrollBoxRenderable) => (scroll = s)}
                  flexGrow={1}
                  focusable={false}
                  contentOptions={inset}
                  verticalScrollbarOptions={bar}
                >
                  <markdown content={d().text} syntaxStyle={style} fg={theme.text} />
                  <text fg={faint} content={"─".repeat(textWidth())} height={1} />
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
              )}
            </Show>
          </>
        }
      >
        <scrollbox
          ref={(s: ScrollBoxRenderable) => (scrollbox = s)}
          flexGrow={1}
          focusable={false}
          contentOptions={inset}
          verticalScrollbarOptions={bar}
        >
          <Show
            when={rows().length > 0 || lost().length > 0}
            fallback={
              <text fg={faint} content="no briefs; main makes them with rewake brief new" />
            }
          >
            <For each={rows()}>
              {(row, i) => (
                // A press on either line acts on the row; the selection and
                // the keys sit on the first.
                <box
                  id={`brief-${pickOf(row)}`}
                  flexDirection="column"
                  {...{
                    onMouseDown: () => {
                      props.choose();
                      act(row);
                    },
                  }}
                >
                  <box flexDirection="row" height={1}>
                    <text
                      height={1}
                      flexShrink={0}
                      wrapMode="none"
                      {...headStyle("fold" in row || row.brief.closed, i() === selected())}
                      {...selecting(theme)}
                      content={` ${cut("fold" in row ? foldText(row.fold) : headOf(row.brief), textWidth() - 2)} `}
                    />
                  </box>
                  <Show when={"brief" in row && row.brief}>
                    {(brief: Accessor<Summary>) => (
                      <text
                        height={1}
                        wrapMode="none"
                        fg={faint}
                        content={cut(
                          `${" ".repeat(indentOf(brief()))}${infoOf(brief())}`,
                          textWidth(),
                        )}
                      />
                    )}
                  </Show>
                </box>
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
