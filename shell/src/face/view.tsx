// rewake's face over a pane (docs/face.md, "The face"): a layer over the
// pane's rectangle, its header, the lines its worker drew and the input line.
// The input is the shell's own, so the person can always switch back with the
// prefix's v, whatever the face did.

import type { InputRenderable } from "@opentui/core";
import { RGBA } from "@opentui/core";
import { createEffect, For, Show } from "solid-js";
import type { Theme } from "../theme.ts";
import { draftOf, faceOf, promptFace, scrollFace, setDraft, sizeFace } from "./faces.ts";
import type { Tone } from "./render.ts";

const toneOf = (tone: Tone, theme: Theme): string | RGBA => {
  switch (tone) {
    case "prompt":
      return theme.worker;
    case "thinking":
    case "notice":
    case "output":
      return theme.faint;
    case "tool":
      return RGBA.fromIndex(6);
    case "gap":
      return RGBA.fromIndex(3);
    default:
      return RGBA.defaultForeground();
  }
};

const headerOf = (link: string, turn: string) =>
  link === "live" ? `rewake face · ${turn}` : `rewake face · ${link}`;

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
  const body = () => Math.max(props.height - 2, 1);
  createEffect(() => {
    if (props.live) sizeFace(props.id, props.width, body());
  });
  const faint = props.theme.faint;
  // A layer over the native face: every cell painted, the terminal's own
  // background where the shell does not know its colour, or a space would
  // let the native face show through.
  const bg = props.theme.background ?? RGBA.defaultBackground();
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
      <text height={1} wrapMode="none" fg={faint} bg={bg}>
        {headerOf(face()?.link ?? "connecting", face()?.turn ?? "unknown")}
        <Show when={face()?.error ?? face()?.said}>
          <span style={{ fg: RGBA.fromIndex(1) }}>{` · ${face()?.error ?? face()?.said}`}</span>
        </Show>
      </text>
      <box height={body()} flexDirection="column">
        <For each={face()?.lines ?? []}>
          {(line) => (
            <text
              height={1}
              wrapMode="none"
              fg={toneOf(line.tone, props.theme)}
              bg={bg}
              content={line.text}
            />
          )}
        </For>
      </box>
      <Show
        when={props.live}
        fallback={
          <text
            height={1}
            wrapMode="none"
            fg={props.theme.text}
            bg={bg}
            content={`❯ ${draftOf(props.id)}`}
          />
        }
      >
        <input
          width={props.width}
          focused={props.active()}
          ref={(r: InputRenderable) => {
            input = r;
            r.value = draftOf(props.id);
          }}
          placeholder={
            face()?.prompts ? "a prompt for this session · ctrl+\\ v: native" : "ctrl+\\ v: native"
          }
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
  );
}
