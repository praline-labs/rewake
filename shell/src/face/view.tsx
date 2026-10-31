// rewake's face over a pane (docs/face.md, "The face"): a layer over the
// pane's rectangle, the rows its worker drew, the input between two rules and
// a row for what went wrong. The input is the shell's own, so the person can
// always switch back with the prefix's v, whatever the face did.

import type { BoxRenderable, InputRenderable } from "@opentui/core";
import { RGBA } from "@opentui/core";
import { createEffect, For, Index, Show } from "solid-js";
import type { FaceLine, Pointing, Span, Tone } from "../../../face/src/messages.ts";
import type { PermissionMode } from "../../../face/src/mirror.gen.ts";
import type { Theme } from "../theme.ts";
import {
  draftOf,
  escapeFace,
  faceOf,
  pointFace,
  promptFace,
  scrollFace,
  setDraft,
  sizeFace,
  unselectFace,
  wheelFace,
} from "./faces.ts";
import { ruleOf, ruleParts } from "./rule.ts";

const colour = (c: string) => RGBA.fromHex(c);

// indexed is a colour of the person's palette: as the terminal said it, else
// by its index, which the terminal draws in its own.
const indexed = (i: number, theme: Theme) => {
  const said = theme.palette[i];
  return said ? colour(said) : RGBA.fromIndex(i);
};

// brightGreen is the palette's bright green, the working line's colour.
const brightGreen = 10;

// toneOf is a tone's colour: the faint, accent and default ones the person's,
// the others the fixed ones of the look rewake's face repeats.
const toneOf = (tone: Tone | undefined, theme: Theme): RGBA => {
  switch (tone) {
    case "faint":
      return colour(theme.faint);
    case "code":
      return colour(theme.look.code);
    case "accent":
      return indexed(brightGreen, theme);
    case "done":
      return colour(theme.look.success);
    case "error":
      return colour(theme.look.error);
    case "gap":
      return colour(theme.look.warning);
    default:
      return RGBA.defaultForeground();
  }
};

const backgroundOf = (theme: Theme) =>
  theme.background ? colour(theme.background) : RGBA.defaultBackground();

// FaceRow is one row the face's worker drew, its spans in the person's
// colours, each cell on the row's own background: the band for a prompt and
// for the part a click just copied, the selection's for a selected cell.
// OpenTUI's own selection is off: the face's worker selects its text.
export function FaceRow(props: { line: FaceLine; theme: Theme }) {
  const bg = backgroundOf(props.theme);
  const band = props.theme.band ? colour(props.theme.band) : bg;
  const lit = colour(props.theme.band ?? props.theme.idle);
  const selected = colour(props.theme.look.selection);
  const under = (span: Span) =>
    span.selected
      ? selected
      : span.mark
        ? colour(props.theme.look[span.mark])
        : props.line.lit
          ? lit
          : props.line.band
            ? band
            : bg;
  return (
    <text height={1} wrapMode="none" bg={bg} selectable={false}>
      <For each={props.line.spans}>
        {(span) => (
          <span
            style={{
              fg: toneOf(span.tone, props.theme),
              bg: under(span),
              bold: span.bold,
              italic: span.italic,
              underline: span.underline,
            }}
          >
            {span.text}
          </span>
        )}
      </For>
    </text>
  );
}

// The rows below the conversation: a rule, the input, a rule, and the row for
// the face's error or its link.
const below = 4;

// modeColour is a permission mode's colour as the terminal program draws
// its name under the input.
const modeColour = (mode: PermissionMode | undefined, theme: Theme): RGBA => {
  switch (mode) {
    case "plan":
      return colour(theme.look.plan);
    case "edits":
      return colour(theme.look.edits);
    case "bypass":
    case "unasked":
      return colour(theme.look.error);
    default:
      return colour(theme.look.warning);
  }
};

export function FaceView(props: {
  id: string;
  width: number;
  height: number;
  theme: Theme;
  // title is the session's name and context its window, `46k/272k`, drawn
  // on the input's top rule with the mode the face was told.
  title?: string;
  context?: string;
  // live is the person's window: only it takes keys and sizes the face.
  live: boolean;
  active: () => boolean;
}) {
  const face = () => faceOf(props.id);
  const body = () => Math.max(props.height - below, 1);
  createEffect(() => {
    if (props.live) sizeFace(props.id, props.width, body());
  });
  const faint = colour(props.theme.faint);
  const ruled = colour(props.theme.look.rule);
  // A layer over the native face: every cell painted, the terminal's own
  // background where the shell does not know its colour, or a space would
  // let the native face show through.
  const bg = backgroundOf(props.theme);
  const status = () => {
    const f = face();
    if (f?.copied)
      return {
        text: f.copied.text,
        fg: f.copied.ok ? colour(props.theme.look.suggestion) : indexed(1, props.theme),
      };
    const failed = f?.error ?? f?.said;
    if (failed) return { text: failed, fg: indexed(1, props.theme) };
    const link = f?.link ?? "connecting";
    if (link === "live") return { text: "", fg: faint };
    return { text: link === "connecting" ? "connecting…" : `the face is ${link}`, fg: faint };
  };
  const row = (text: string) => text + " ".repeat(Math.max(props.width - Bun.stringWidth(text), 0));
  let input: InputRenderable | undefined;
  let rows: BoxRenderable | undefined;
  // pointed passes the left button over the conversation to the face's
  // worker, in the conversation's cells: a press on the rules or the input is
  // theirs, while a drag goes on past any edge. The bare pointer is not
  // passed: nothing lights as it moves. Nothing here reaches the pane's pty:
  // the layer is what the pointer hits.
  const pointed = (type: Pointing) => (event: { x: number; y: number; button: number }) => {
    if (!props.live || !rows || event.button !== 0) return;
    const x = event.x - rows.x;
    const y = event.y - rows.y;
    if (type === "down" && y >= body()) return;
    pointFace(props.id, type, x, y, props.active());
  };
  return (
    <box
      position="absolute"
      top={0}
      left={0}
      width={props.width}
      height={props.height}
      zIndex={50}
      flexDirection="column"
      backgroundColor={bg}
      onMouseScroll={(event: {
        scroll?: { direction: string };
        preventDefault(): void;
        stopPropagation(): void;
      }) => {
        // The wheel scrolls the conversation and never reaches the pty.
        event.preventDefault();
        event.stopPropagation();
        const direction = event.scroll?.direction;
        if (!props.live || (direction !== "up" && direction !== "down")) return;
        wheelFace(props.id, direction === "up");
      }}
      onMouseDown={pointed("down")}
      onMouseDrag={pointed("drag")}
      onMouseUp={pointed("up")}
    >
      {/* Rows are updated in place: the face posts a frame on every tick
          that changes it. */}
      <box
        height={body()}
        flexDirection="column"
        ref={(r: BoxRenderable) => {
          rows = r;
        }}
      >
        <Index each={face()?.lines ?? []}>
          {(line) => <FaceRow line={line()} theme={props.theme} />}
        </Index>
      </box>
      <text height={1} wrapMode="none" fg={ruled} bg={bg} selectable={false}>
        <For each={ruleParts(props.width, props.title, props.context, face()?.mode)}>
          {(part) => (
            <span style={{ fg: part.mode ? modeColour(part.mode, props.theme) : ruled, bg }}>
              {part.text}
            </span>
          )}
        </For>
      </text>
      <box height={1} flexDirection="row">
        <text
          width={2}
          height={1}
          wrapMode="none"
          fg={RGBA.defaultForeground()}
          bg={bg}
          content="❯ "
          selectable={false}
        />
        <Show
          when={props.live}
          fallback={
            <text
              height={1}
              flexGrow={1}
              wrapMode="none"
              fg={RGBA.defaultForeground()}
              bg={bg}
              content={row(draftOf(props.id)).slice(0, Math.max(props.width - 2, 0))}
            />
          }
        >
          <input
            width={Math.max(props.width - 2, 1)}
            backgroundColor={bg}
            focusedBackgroundColor={bg}
            textColor={RGBA.defaultForeground()}
            focusedTextColor={RGBA.defaultForeground()}
            placeholderColor={faint}
            focused={props.active()}
            ref={(r: InputRenderable) => {
              input = r;
              r.value = draftOf(props.id);
            }}
            placeholder={face()?.prompts ? "" : "this session takes no prompts here"}
            onInput={(text: string) => {
              // A key drops the selection, as the terminal program's does.
              unselectFace(props.id);
              setDraft(props.id, text);
            }}
            onSubmit={
              (() => {
                const text = input?.value ?? "";
                if (!text.trim() || !promptFace(props.id, text)) return;
                setDraft(props.id, "");
                if (input) input.value = "";
              }) as never
            }
            onKeyDown={(key: { name: string }) => {
              if (key.name === "escape") escapeFace(props.id);
              if (key.name === "pageup") scrollFace(props.id, body() - 1);
              if (key.name === "pagedown") scrollFace(props.id, -(body() - 1));
            }}
          />
        </Show>
      </box>
      <text
        height={1}
        wrapMode="none"
        fg={ruled}
        bg={bg}
        content={ruleOf(props.width)}
        selectable={false}
      />
      <text
        height={1}
        wrapMode="none"
        fg={status().fg}
        bg={bg}
        content={row(status().text)}
        selectable={false}
      />
    </box>
  );
}
