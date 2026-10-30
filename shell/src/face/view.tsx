// rewake's face over a pane (docs/face.md, "The face"): a layer over the
// pane's rectangle, the rows its worker drew, the input between two rules and
// a row for what went wrong. The input is the shell's own, so the person can
// always switch back with the prefix's v, whatever the face did.

import type { InputRenderable } from "@opentui/core";
import { RGBA } from "@opentui/core";
import { createEffect, For, Index, Show } from "solid-js";
import { mix, type Theme } from "../theme.ts";
import { draftOf, faceOf, promptFace, scrollFace, setDraft, sizeFace, wheelFace } from "./faces.ts";
import type { FaceLine, Span, Tone } from "./messages.ts";

const colour = (c: string) => RGBA.fromHex(c);

// indexed is a colour of the person's palette: as the terminal said it, else
// by its index, which the terminal draws in its own.
const indexed = (i: number, theme: Theme) => {
  const said = theme.palette[i];
  return said ? colour(said) : RGBA.fromIndex(i);
};

// toneOf is a tone's colour: the faint and default ones the person's, the
// others the fixed ones of the look rewake's face repeats.
const toneOf = (tone: Tone | undefined, theme: Theme): RGBA => {
  switch (tone) {
    case "faint":
      return colour(theme.faint);
    case "code":
      return colour(theme.look.code);
    case "accent":
      return colour(theme.look.spinner);
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

// fgOf is a span's colour: its tone's, the working verb's moved towards its
// lighter shade by its glow.
const fgOf = (span: Span, theme: Theme): RGBA =>
  span.glow && span.tone === "accent"
    ? colour(mix(theme.look.spinner, theme.look.shimmer, span.glow))
    : toneOf(span.tone, theme);

const backgroundOf = (theme: Theme) =>
  theme.background ? colour(theme.background) : RGBA.defaultBackground();

// FaceRow is one row the face's worker drew, its spans in the person's
// colours, each cell on the row's own background.
export function FaceRow(props: { line: FaceLine; theme: Theme }) {
  const bg = backgroundOf(props.theme);
  const band = props.theme.band ? colour(props.theme.band) : bg;
  return (
    <text height={1} wrapMode="none" bg={bg}>
      <For each={props.line.spans}>
        {(span) => (
          <span
            style={{
              fg: fgOf(span, props.theme),
              bg: span.mark ? colour(props.theme.look[span.mark]) : props.line.band ? band : bg,
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

const graphemes = new Intl.Segmenter();

// ruleOf is the input's top rule with the session's name at its right end, as
// the terminal program draws its own, `──── name ─`: cut with … where the
// pane is too narrow for it, left out where not even a letter of it fits.
export function ruleOf(width: number, title?: string): string {
  const bare = "─".repeat(Math.max(width, 0));
  // Two cells of the rule before it, a space, the name, a space and a cell
  // of the rule after it.
  const room = width - 5;
  if (!title || room < 2) return bare;
  let name = title;
  if (Bun.stringWidth(name) > room) {
    name = "";
    for (const { segment } of graphemes.segment(title)) {
      if (Bun.stringWidth(name + segment) > room - 1) break;
      name += segment;
    }
    name += "…";
  }
  const tail = ` ${name} ─`;
  return "─".repeat(width - Bun.stringWidth(tail)) + tail;
}

export function FaceView(props: {
  id: string;
  width: number;
  height: number;
  theme: Theme;
  // title is the session's name, drawn on the input's top rule.
  title?: string;
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
    const failed = f?.error ?? f?.said;
    if (failed) return { text: failed, fg: indexed(1, props.theme) };
    const link = f?.link ?? "connecting";
    if (link === "live") return { text: "", fg: faint };
    return { text: link === "connecting" ? "connecting…" : `the face is ${link}`, fg: faint };
  };
  const row = (text: string) => text + " ".repeat(Math.max(props.width - Bun.stringWidth(text), 0));
  let input: InputRenderable | undefined;
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
    >
      {/* Rows are updated in place: the face posts a frame on every tick
          that changes it. */}
      <box height={body()} flexDirection="column">
        <Index each={face()?.lines ?? []}>
          {(line) => <FaceRow line={line()} theme={props.theme} />}
        </Index>
      </box>
      <text
        height={1}
        wrapMode="none"
        fg={ruled}
        bg={bg}
        content={ruleOf(props.width, props.title)}
      />
      <box height={1} flexDirection="row">
        <text
          width={2}
          height={1}
          wrapMode="none"
          fg={RGBA.defaultForeground()}
          bg={bg}
          content="❯ "
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
              if (key.name === "pageup") scrollFace(props.id, body() - 1);
              if (key.name === "pagedown") scrollFace(props.id, -(body() - 1));
            }}
          />
        </Show>
      </box>
      <text height={1} wrapMode="none" fg={ruled} bg={bg} content={ruleOf(props.width)} />
      <text height={1} wrapMode="none" fg={status().fg} bg={bg} content={row(status().text)} />
    </box>
  );
}
