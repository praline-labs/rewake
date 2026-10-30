// rewake's face over a pane (docs/face.md, "The face"): a layer over the
// pane's rectangle, the rows its worker drew, the input between two rules and
// a row for what went wrong. The input is the shell's own, so the person can
// always switch back with the prefix's v, whatever the face did.

import type { InputRenderable } from "@opentui/core";
import { RGBA } from "@opentui/core";
import { createEffect, For, Show } from "solid-js";
import type { Theme } from "../theme.ts";
import { draftOf, faceOf, promptFace, scrollFace, setDraft, sizeFace } from "./faces.ts";
import type { FaceLine, Span, Tone } from "./messages.ts";

const colour = (c: string) => RGBA.fromHex(c);

// indexed is a colour of the person's palette: as the terminal said it, else
// by its index, which the terminal draws in its own.
const indexed = (i: number, theme: Theme) => {
  const said = theme.palette[i];
  return said ? colour(said) : RGBA.fromIndex(i);
};

// toneOf is a tone in the person's colours: the theme's, or their palette's.
const toneOf = (tone: Tone | undefined, theme: Theme): RGBA => {
  switch (tone) {
    case "faint":
      return colour(theme.faint);
    case "code":
      return indexed(12, theme);
    case "accent":
      return indexed(1, theme);
    case "done":
      return colour(theme.worker);
    case "error":
      return indexed(1, theme);
    case "gap":
      return indexed(3, theme);
    default:
      return RGBA.defaultForeground();
  }
};

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
              fg: toneOf(span.tone, props.theme),
              bg: props.line.band ? band : bg,
              bold: span.bold,
              italic: span.italic,
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

export function FaceView(props: {
  id: string;
  width: number;
  height: number;
  theme: Theme;
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
  // A layer over the native face: every cell painted, the terminal's own
  // background where the shell does not know its colour, or a space would
  // let the native face show through.
  const bg = backgroundOf(props.theme);
  const rule = () => "─".repeat(props.width);
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
    >
      <box height={body()} flexDirection="column">
        <For each={face()?.lines ?? []}>
          {(line) => <FaceRow line={line} theme={props.theme} />}
        </For>
      </box>
      <text height={1} wrapMode="none" fg={faint} bg={bg} content={rule()} />
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
      <text height={1} wrapMode="none" fg={faint} bg={bg} content={rule()} />
      <text height={1} wrapMode="none" fg={status().fg} bg={bg} content={row(status().text)} />
    </box>
  );
}
