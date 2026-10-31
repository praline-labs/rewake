// The face's input (docs/face.md, "The input"): the shell's own, so the
// person can always switch back with the prefix's v. It is pi's editor in
// its keys and its growth, as read in the bundled pi-tui 1.4.0: Enter sends,
// shift+Enter, alt+Enter and ctrl+j start a new line, as does Enter after a
// `\`; it grows with its text to 30% of the pane, at least 5 rows, then
// scrolls inside with `↑ N more` and `↓ N more` on its rules; a paste comes
// whole with its newlines, a large one as pi's marker. Its keys never reach a
// pty: the pane's widget is blurred while the face is shown.

import type {
  KeyBinding,
  KeyEvent,
  PasteEvent,
  TextareaRenderable,
  TextRenderable,
} from "@opentui/core";
import { decodePasteBytes, RGBA, stripAnsiSequences } from "@opentui/core";
import { useRenderer } from "@opentui/solid";
import { createEffect, on, Show } from "solid-js";
import type { Theme } from "../theme.ts";
import {
  draftOf,
  escapeFace,
  faceOf,
  inputOf,
  inputRowsOf,
  promptFace,
  scrollFace,
  setDraft,
  setInput,
  unfoldFace,
  unselectFace,
} from "./faces.ts";
import { cleaned, Pastes, rowsFor } from "./paste.ts";

// pi's keys over OpenTUI's own, which already move by words (alt or ctrl
// with an arrow, alt+b, alt+f), delete by words (ctrl+w, alt+Backspace,
// alt+d, alt+Delete), to a line's ends (ctrl+u, ctrl+k) and undo (ctrl+-).
const keys: KeyBinding[] = [
  { name: "return", action: "submit" },
  { name: "kpenter", action: "submit" },
  { name: "return", shift: true, action: "newline" },
  { name: "return", meta: true, action: "newline" },
  { name: "linefeed", action: "newline" },
  { name: "j", ctrl: true, action: "newline" },
  { name: "home", action: "line-home" },
  { name: "end", action: "line-end" },
];

// The pastes each pane's input holds markers for, kept while the face is
// switched away, as its draft is.
const held = new Map<string, Pastes>();
const pastesOf = (id: string) => {
  const p = held.get(id) ?? new Pastes();
  held.set(id, p);
  return p;
};

export function FaceInput(props: {
  id: string;
  width: number;
  height: number;
  theme: Theme;
  live: boolean;
  active: () => boolean;
  // page is a page of the conversation, PgUp and PgDn's.
  page: () => number;
  // prompt is where a sent text goes, the face's worker unless a test says.
  prompt?: (id: string, text: string) => boolean;
}) {
  const bg = () =>
    props.theme.background ? RGBA.fromHex(props.theme.background) : RGBA.defaultBackground();
  const faint = RGBA.fromHex(props.theme.faint);
  const text = RGBA.defaultForeground();
  const bound = () => rowsFor(props.height);
  let area: TextareaRenderable | undefined;
  // measure takes the rows the text wraps to, and where the view inside
  // stands, once the textarea laid them out.
  const measure = () => {
    // A frame may come after the window stopped being the person's.
    if (!area || area.isDestroyed) return;
    const n = Math.max(area.editorView.getTotalVirtualLineCount(), 1);
    const rows = Math.min(n, bound());
    const above = Math.min(area.scrollY, n - rows);
    setInput(props.id, { rows, above, under: Math.max(n - rows - above, 0) });
  };
  // The text wraps at the textarea's width, and scrolls to its cursor, only
  // as a frame draws it: a draft put back as the face is shown, or a pane
  // made narrower, has its rows after the next frame.
  const renderer = useRenderer();
  let waiting = false;
  const afterFrame = () => {
    if (waiting) return;
    waiting = true;
    renderer.once("frame", () => {
      waiting = false;
      measure();
    });
  };
  // A key measures at once and again after the frame, which scrolls the
  // text to the cursor.
  const changed = () => {
    measure();
    afterFrame();
  };
  createEffect(on(() => props.width, afterFrame, { defer: true }));
  // Another window of the pane shows the draft scrolled as the person's,
  // once a frame laid out its rows: a scroll past them is clamped before.
  let shadow: TextRenderable | undefined;
  createEffect(() => {
    const view = inputOf(props.id);
    draftOf(props.id);
    const scroll = () => {
      if (shadow && !shadow.isDestroyed) shadow.scrollY = view.above;
    };
    scroll();
    renderer.once("frame", scroll);
  });
  const send = () => {
    if (!area) return;
    const value = area.plainText;
    const at = area.cursorOffset;
    // Enter after a `\` is a new line, pi's way for a terminal that sends
    // shift+Enter as Enter.
    if (at > 0 && value[at - 1] === "\\") {
      area.deleteCharBackward();
      area.newLine();
      return;
    }
    const pastes = pastesOf(props.id);
    const full = pastes.expand(value).trim();
    if (!full || !(props.prompt ?? promptFace)(props.id, full)) return;
    pastes.clear();
    area.setText("");
    setDraft(props.id, "");
    changed();
  };
  const pasted = (event: PasteEvent) => {
    event.preventDefault();
    const raw = cleaned(stripAnsiSequences(decodePasteBytes(event.bytes)));
    if (raw) area?.insertText(pastesOf(props.id).take(raw));
  };
  return (
    <Show
      when={props.live}
      fallback={
        <text
          width={Math.max(props.width - 2, 1)}
          height={inputRowsOf(props.id)}
          ref={(r: TextRenderable) => {
            shadow = r;
          }}
          wrapMode="word"
          fg={text}
          bg={bg()}
          content={draftOf(props.id)}
        />
      }
    >
      <textarea
        width={Math.max(props.width - 2, 1)}
        height={inputRowsOf(props.id)}
        wrapMode="word"
        backgroundColor={bg()}
        focusedBackgroundColor={bg()}
        textColor={text}
        focusedTextColor={text}
        placeholderColor={faint}
        keyBindings={keys}
        focused={props.active()}
        ref={(r: TextareaRenderable) => {
          area = r;
          r.setText(draftOf(props.id));
          r.gotoBufferEnd();
          afterFrame();
        }}
        placeholder={faceOf(props.id)?.prompts ? "" : "this session takes no prompts here"}
        onContentChange={() => {
          // A key drops the selection, as the terminal program's does.
          unselectFace(props.id);
          setDraft(props.id, area?.plainText ?? "");
          changed();
        }}
        onCursorChange={changed}
        onPaste={pasted}
        onSubmit={send}
        onKeyDown={(key: KeyEvent) => {
          // A move of the cursor may scroll the text and says no change.
          afterFrame();
          if (key.name === "escape") escapeFace(props.id);
          if (key.name === "o" && key.ctrl) unfoldFace(props.id);
          if (key.name === "pageup") scrollFace(props.id, props.page());
          if (key.name === "pagedown") scrollFace(props.id, -props.page());
        }}
      />
    </Show>
  );
}
