import { expect, test } from "bun:test";
import { TextAttributes } from "@opentui/core";
import { testRender } from "@opentui/solid";
import "../pane.ts";
import type { PaneTerminal } from "../pane.ts";
import { themeOf } from "../theme.ts";
import { ruleOf } from "./rule.ts";
import { FaceRow, FaceView } from "./view.tsx";

const theme = themeOf(undefined);

// The worker's rows are data the shell's thread paints: a row must come out
// as its text and its style, not as the object it was sent as (seen while
// this was written: a styled text as content drew [object Object], and a
// span's attributes in its style drew no bold).
test("a row the face drew is painted as its spans' text", async () => {
  const line = {
    spans: [{ text: "● " }, { text: "Bash", bold: true }, { text: "(ls)", tone: "faint" as const }],
  };
  const { renderOnce, captureCharFrame, captureSpans } = await testRender(
    () => <FaceRow line={line} theme={theme} />,
    { width: 12, height: 1 },
  );
  await renderOnce();
  expect(captureCharFrame().split("\n")[0]).toBe("● Bash(ls)  ");
  const bash = captureSpans().lines[0]?.spans.find((s) => s.text === "Bash");
  expect(bash?.attributes).toBe(TextAttributes.BOLD);
});

// Seen live on 2.11.0: the native face showed through the pane's last column.
test("the face covers every cell of the pane it is laid over", async () => {
  const { renderOnce, captureCharFrame } = await testRender(
    () => (
      <box width={10} height={6} flexDirection="column">
        {Array.from({ length: 6 }, () => (
          <text content="NNNNNNNNNN" />
        ))}
        <FaceView id="none" width={10} height={6} theme={theme} live={false} active={() => false} />
      </box>
    ),
    { width: 12, height: 6 },
  );
  await renderOnce();
  expect(captureCharFrame()).not.toContain("N");
});

// A diff's mark is data the worker sends and the shell's thread paints: an
// old line on the look's red, a heading underlined.
test("a marked span is painted on its mark's colour", async () => {
  const line = {
    spans: [
      { text: "-a", mark: "removed" as const },
      { text: "H", underline: true },
    ],
  };
  const { renderOnce, captureSpans } = await testRender(
    () => <FaceRow line={line} theme={theme} />,
    { width: 4, height: 1 },
  );
  await renderOnce();
  const spans = captureSpans().lines[0]?.spans ?? [];
  expect(
    spans
      .find((s) => s.text === "-a")
      ?.bg.toInts()
      .slice(0, 3),
  ).toEqual([122, 41, 54]);
  expect(spans.find((s) => s.text.startsWith("H"))?.attributes).toBe(TextAttributes.UNDERLINE);
});

// The session's name sits at the right end of the input's top rule, as the
// terminal program draws its own, and fits any pane.
test("the input's rule carries the name, cut to a narrow pane", () => {
  expect(ruleOf(20, "w-claude (code)")).toBe("── w-claude (code) ─");
  expect(ruleOf(12, "w-claude (code)")).toBe("── w-clau… ─");
  expect(ruleOf(30)).toBe("─".repeat(30));
  for (let width = 0; width <= 30; width++) {
    const rule = ruleOf(width, "名前-claude (code)");
    expect(Bun.stringWidth(rule)).toBe(width);
    expect(rule.endsWith(" ─") || rule === "─".repeat(width)).toBe(true);
  }
});

// The mode sits at the rule's left end, the context after the name; a
// narrow pane drops the mode, then the context, then cuts the name.
test("the input's rule carries the mode and the context, dropped in turn as the pane narrows", () => {
  const rule = (width: number) => ruleOf(width, "w", "46k/272k", "auto");
  expect(rule(40)).toBe(`── ⏵⏵ auto mode ${"─".repeat(9)} w · 46k/272k ─`);
  expect(rule(30)).toBe("───────────────── w · 46k/272k ─".slice(-30));
  expect(rule(12)).toBe("────────── w ─".slice(-12));
  expect(ruleOf(40, "w", "46k/272k", "default")).toBe(`${"─".repeat(25)} w · 46k/272k ─`);
  for (let width = 0; width <= 50; width++)
    expect(Bun.stringWidth(ruleOf(width, "名前", "46k/272k", "bypass"))).toBe(width);
});

// The owner's rule: the mouse over the face never reaches the pane's pty. A
// stand-in for the pane's widget under the layer hears nothing of a press, a
// drag, a release or the wheel over the conversation, and no selection of
// the renderer's own starts there.
test("the mouse over the face never reaches the pane under it", async () => {
  const heard: string[] = [];
  const hear = (what: string) => () => heard.push(what);
  const { renderOnce, mockMouse, renderer } = await testRender(
    () => (
      <box width={20} height={8}>
        <box
          position="absolute"
          width={20}
          height={8}
          onMouseDown={hear("down")}
          onMouseDrag={hear("drag")}
          onMouseUp={hear("up")}
          onMouseMove={hear("move")}
          onMouseScroll={hear("scroll")}
        />
        <FaceView id="none" width={20} height={8} theme={theme} live={true} active={() => true} />
      </box>
    ),
    { width: 20, height: 8 },
  );
  await renderOnce();
  await mockMouse.moveTo(3, 1);
  await mockMouse.drag(2, 1, 9, 3);
  await mockMouse.click(4, 2);
  await mockMouse.scroll(4, 2, "up");
  expect(heard).toEqual([]);
  expect(renderer.getSelection()).toBeNull();
});

// A drag over the face's rows is the face's selection, never the renderer's,
// whose copy would take the drawing, gutters and all.
test("a drag over a row of the face starts no selection of the renderer's", async () => {
  const line = { spans: [{ text: "● ", gutter: true }, { text: "some reply" }] };
  const { renderOnce, mockMouse, renderer } = await testRender(
    () => <FaceRow line={line} theme={theme} />,
    { width: 12, height: 1 },
  );
  await renderOnce();
  await mockMouse.drag(0, 0, 8, 0);
  expect(renderer.getSelection()).toBeNull();
});

// The owner's rule on a real pane: a pi worker's pty draws its normal
// screen with scrollback and takes no mouse, so the pane's widget would
// scroll its own scrollback on the wheel, and a fullscreen program's would
// pass the wheel to its pty. Under the face the widget hears no wheel at all:
// its mouse forwarding is the one path to either.
test("the wheel over the face never reaches the pane's widget, pi's or a fullscreen program's", async () => {
  const programs = [
    Array.from({ length: 40 }, (_, i) => `row ${i}`).join("\r\n"),
    "\x1b[?1049h\x1b[?1002h\x1b[?1006hrow 39",
  ];
  const heard: Uint8Array[] = [];
  const forwarded: string[] = [];
  const pane = (output: string) => (t: PaneTerminal) => {
    t.onData = (data) => heard.push(data);
    const widget = t as unknown as { forwardMouse(e: { type: string }, a: string): void };
    const forward = widget.forwardMouse.bind(t);
    widget.forwardMouse = (event, action) => {
      forwarded.push(event.type);
      forward(event, action);
    };
    t.write(output);
  };
  const { renderOnce, mockMouse, renderer } = await testRender(
    () => (
      <box width={40} height={8} flexDirection="row">
        {programs.map((output, i) => (
          <box width={20} height={8}>
            <pane_terminal id={`w${i}`} ref={pane(output)} width={20} height={8} />
            <FaceView
              id="none"
              width={20}
              height={8}
              theme={theme}
              live={true}
              active={() => true}
            />
          </box>
        ))}
      </box>
    ),
    { width: 40, height: 8 },
  );
  await renderOnce();
  for (const x of [4, 24])
    for (let i = 0; i < 5; i++) await mockMouse.scroll(x, 2, i % 2 ? "down" : "up");
  expect(forwarded).toEqual([]);
  expect(heard).toEqual([]);
  renderer.destroy();
});
