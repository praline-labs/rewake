// The face's input as pi's editor (docs/face.md, "The input"): it grows with
// its text to pi's bound and then scrolls inside, its keys are pi's, and a
// paste comes whole, a large one behind pi's marker.
import { expect, test } from "bun:test";
import { testRender } from "@opentui/solid";
import { createSignal } from "solid-js";
import { themeOf } from "../theme.ts";
import { draftOf, inputOf, inputRowsOf, setDraft } from "./faces.ts";
import { FaceInput } from "./input.tsx";
import { cleaned, moreRule, Pastes, rowsFor } from "./paste.ts";
import { ruleOf, ruleParts, withMore } from "./rule.ts";
import { FaceView } from "./view.tsx";

const theme = themeOf(undefined);

const frame = (capture: () => string) => capture().split("\n");

// The owner, October 31, 2026: the input ran on sideways in one row. It
// grows as the text wraps or holds a new line, the conversation above giving
// the room, to 30% of the pane as pi's editor, and then says what it hides.
test("the input grows with its text to pi's bound, then scrolls inside it", async () => {
  const { renderOnce, captureCharFrame, mockInput, renderer } = await testRender(
    () => (
      <FaceView id="grow" width={24} height={20} theme={theme} live={true} active={() => true} />
    ),
    { width: 24, height: 20, kittyKeyboard: true },
  );
  await renderOnce();
  expect(frame(captureCharFrame)[16]).toBe("─".repeat(24));
  await mockInput.typeText("one two three four five six seven");
  await renderOnce();
  await renderOnce();
  let rows = frame(captureCharFrame);
  expect(inputRowsOf("grow")).toBe(2);
  expect(rows.slice(15, 19).map((r) => r.trimEnd())).toEqual([
    "─".repeat(24),
    "❯ one two three four",
    "  five six seven",
    "─".repeat(24),
  ]);
  for (const line of ["a", "b", "c", "d", "e"]) {
    mockInput.pressEnter({ shift: true });
    await mockInput.typeText(line);
  }
  await renderOnce();
  await renderOnce();
  rows = frame(captureCharFrame);
  expect(inputRowsOf("grow")).toBe(rowsFor(20));
  expect(rows.slice(11, 19).map((r) => r.trimEnd())).toEqual([
    "─────── ↑ 1 more ───────",
    "❯ five six seven",
    "  a",
    "  b",
    "  c",
    "  d",
    "  e",
    "─".repeat(24),
  ]);
  expect(draftOf("grow")).toBe("one two three four five six seven\na\nb\nc\nd\ne");
  // Up to the first row: the text it hides is under it.
  for (let i = 0; i < 6; i++) mockInput.pressArrow("up");
  await renderOnce();
  await renderOnce();
  const top = frame(captureCharFrame).slice(11, 19);
  expect(top.map((r) => r.trimEnd())).toEqual([
    "─".repeat(24),
    "❯ one two three four",
    "  five six seven",
    "  a",
    "  b",
    "  c",
    "  d",
    "─────── ↓ 1 more ───────",
  ]);
  // Another window of the pane, the shadow `rewake screen` takes, draws it
  // the same (seen live: it drew the draft from its top, with no more).
  const shadowed = async (rows: string[]) => {
    const shadow = await testRender(
      () => (
        <FaceView
          id="grow"
          width={24}
          height={20}
          theme={theme}
          live={false}
          active={() => false}
        />
      ),
      { width: 24, height: 20 },
    );
    for (let i = 0; i < 3; i++) await shadow.renderOnce();
    expect(frame(shadow.captureCharFrame).slice(11, 19)).toEqual(rows);
    shadow.renderer.destroy();
  };
  await shadowed(top);
  for (let i = 0; i < 6; i++) mockInput.pressArrow("down");
  await renderOnce();
  await renderOnce();
  await shadowed(frame(captureCharFrame).slice(11, 19));
  expect(frame(captureCharFrame)[11]).toBe("─────── ↑ 1 more ───────");
  renderer.destroy();
});

// Seen live while this was written: a draft put back as the face is shown
// was measured before the textarea had its width, and showed its last two
// rows of four. Its rows count once a frame drew it, in every window.
test("a draft put back as the face is shown takes all its rows", async () => {
  setDraft(
    "back",
    "This is a long prompt typed into the face to see whether its input grows to several rows",
  );
  for (const live of [true, false]) {
    const { renderOnce, captureCharFrame, renderer } = await testRender(
      () => (
        <FaceView id="back" width={30} height={12} theme={theme} live={live} active={() => live} />
      ),
      { width: 30, height: 12 },
    );
    for (let i = 0; i < 3; i++) await renderOnce();
    expect(
      frame(captureCharFrame)
        .slice(5, 11)
        .map((r) => r.trimEnd()),
    ).toEqual([
      "─".repeat(30),
      "❯ This is a long prompt typed",
      "  into the face to see whether",
      "  its input grows to several",
      "  rows",
      "─".repeat(30),
    ]);
    renderer.destroy();
  }
});

// pi's keys, read in its editor: Enter sends, shift+Enter, alt+Enter and
// ctrl+j start a line, and so does Enter after a `\`; what is sent is the
// text trimmed, and the input is empty again.
test("Enter sends the text, and pi's new-line keys start a line", async () => {
  const sent: string[] = [];
  const { renderOnce, mockInput, renderer } = await testRender(
    () => (
      <box width={40} height={10}>
        <FaceInput
          id="keys"
          width={40}
          height={30}
          theme={theme}
          live={true}
          active={() => true}
          page={() => 10}
          prompt={(_, text) => sent.push(text) > 0}
        />
      </box>
    ),
    { width: 40, height: 10, kittyKeyboard: true },
  );
  await renderOnce();
  await mockInput.typeText("one");
  mockInput.pressEnter({ shift: true });
  await mockInput.typeText("two");
  mockInput.pressEnter({ meta: true });
  await mockInput.typeText("three");
  mockInput.pressKey("j", { ctrl: true });
  await mockInput.typeText("four\\");
  mockInput.pressEnter();
  await mockInput.typeText("five  ");
  await renderOnce();
  expect(draftOf("keys")).toBe("one\ntwo\nthree\nfour\nfive  ");
  expect(inputRowsOf("keys")).toBe(5);
  expect(inputOf("keys")).toEqual({ rows: 5, above: 0, under: 0 });
  mockInput.pressEnter();
  await renderOnce();
  expect(sent).toEqual(["one\ntwo\nthree\nfour\nfive"]);
  expect(draftOf("keys")).toBe("");
  expect(inputRowsOf("keys")).toBe(1);
  renderer.destroy();
});

// A bracketed paste arrives whole, its newlines and blank lines kept; one
// past pi's size is a marker in the input and the paste itself when sent.
test("a paste keeps its lines, a large one shown as pi's marker and sent whole", async () => {
  const sent: string[] = [];
  const { renderOnce, captureCharFrame, mockInput, renderer } = await testRender(
    () => (
      <box width={40} height={10}>
        <FaceInput
          id="paste"
          width={40}
          height={30}
          theme={theme}
          live={true}
          active={() => true}
          page={() => 10}
          prompt={(_, text) => sent.push(text) > 0}
        />
      </box>
    ),
    { width: 40, height: 10 },
  );
  await renderOnce();
  await mockInput.pasteBracketedText("a\r\n\r\n\tb");
  await renderOnce();
  expect(draftOf("paste")).toBe("a\n\n    b");
  const long = Array.from({ length: 40 }, (_, i) => `line ${i}`).join("\n");
  await mockInput.typeText(" ");
  await mockInput.pasteBracketedText(long);
  await renderOnce();
  expect(draftOf("paste")).toBe("a\n\n    b [paste #1 +40 lines]");
  expect(frame(captureCharFrame)[2]?.trimEnd()).toBe("    b [paste #1 +40 lines]");
  mockInput.pressEnter();
  await renderOnce();
  expect(sent).toEqual([`a\n\n    b ${long}`]);
  renderer.destroy();
});

// Review of 8e94ba1: a Backspace after the marker sent `[paste #1 +40 lines`
// as typed. pi's editor holds a marker as one piece: the cursor steps over
// it, Backspace takes it whole, and so does a word's delete that reaches it.
test("a paste's marker moves and deletes whole, as in pi's editor", async () => {
  const sent: string[] = [];
  const shown = (live: boolean) => (
    <box width={40} height={10}>
      <FaceInput
        id="whole"
        width={40}
        height={30}
        theme={theme}
        live={live}
        active={() => live}
        page={() => 10}
        prompt={(_, text) => sent.push(text) > 0}
      />
    </box>
  );
  const { renderOnce, mockInput, renderer } = await testRender(() => shown(true), {
    width: 40,
    height: 10,
  });
  await renderOnce();
  const long = Array.from({ length: 40 }, (_, i) => `line ${i}`).join("\n");
  await mockInput.pasteBracketedText(long);
  mockInput.pressBackspace();
  await renderOnce();
  expect(draftOf("whole")).toBe("");
  mockInput.pressEnter();
  await renderOnce();
  expect(sent).toEqual([]);
  // After a line of its own: Left steps over it, a key there types before it.
  await mockInput.typeText("a");
  mockInput.pressEnter({ meta: true });
  await mockInput.pasteBracketedText(long);
  await mockInput.typeText("b");
  mockInput.pressArrow("left");
  mockInput.pressArrow("left");
  await mockInput.typeText("c");
  await renderOnce();
  expect(draftOf("whole")).toBe("a\nc[paste #2 +40 lines]b");
  // A word's delete from after it takes it whole.
  mockInput.pressArrow("right");
  mockInput.pressKey("w", { ctrl: true });
  await renderOnce();
  expect(draftOf("whole")).toBe("a\ncb");
  mockInput.pressArrow("right");
  await mockInput.typeText(" ");
  await mockInput.pasteBracketedText(long);
  await renderOnce();
  renderer.destroy();
  // A draft put back as the face is shown again keeps its marker whole.
  const again = await testRender(() => shown(true), { width: 40, height: 10 });
  await again.renderOnce();
  again.mockInput.pressBackspace();
  await again.renderOnce();
  expect(draftOf("whole")).toBe("a\ncb ");
  await again.mockInput.pasteBracketedText(long);
  await again.renderOnce();
  again.renderer.destroy();
  // Shown once more, the marker put back is sent as its paste.
  const last = await testRender(() => shown(true), { width: 40, height: 10 });
  await last.renderOnce();
  last.mockInput.pressEnter();
  await last.renderOnce();
  expect(sent).toEqual([`a\ncb ${long}`]);
  last.renderer.destroy();
});

// Review of 8e94ba1: a pane made lower kept the input at the old pane's
// bound, 12 rows where 6 were the new.
test("a pane made lower lowers the input's bound", async () => {
  const [height, setHeight] = createSignal(40);
  setDraft("lower", Array.from({ length: 15 }, (_, i) => `line ${i}`).join("\n"));
  const t = await testRender(
    () => (
      <FaceView
        id="lower"
        width={40}
        height={height()}
        theme={theme}
        live={true}
        active={() => true}
      />
    ),
    { width: 40, height: 40 },
  );
  for (let i = 0; i < 4; i++) await t.renderOnce();
  expect(inputRowsOf("lower")).toBe(12);
  setHeight(20);
  t.resize(40, 20);
  for (let i = 0; i < 4; i++) await t.renderOnce();
  expect(inputRowsOf("lower")).toBe(6);
  expect(inputOf("lower").above + inputOf("lower").under).toBe(9);
  t.renderer.destroy();
});

test("pi's paste rules: line ends, tabs, control characters and the large paste's marker", () => {
  expect(cleaned("a\r\nb\rc\td\x07\x1b")).toBe("a\nb\nc    d");
  const p = new Pastes();
  expect(p.take("short\ntext")).toEqual({ text: "short\ntext" });
  expect(p.take("x".repeat(1001))).toEqual({ text: "[paste #1 1001 chars]", id: 1 });
  expect(p.take("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11")).toEqual({
    text: "[paste #2 +11 lines]",
    id: 2,
  });
  // A marker the person typed for a paste never made, or one of a paste
  // whose marker was deleted, stays as typed.
  const text = "[paste #2 +11 lines] and [paste #9 +3 lines] [paste #1 1001 chars]";
  expect(p.markers(text).map((m) => [m.id, m.start, m.end])).toEqual([
    [2, 0, 20],
    [1, 45, 66],
  ]);
  expect(p.expand(text, new Set([2]))).toBe(
    "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11 and [paste #9 +3 lines] [paste #1 1001 chars]",
  );
  p.clear();
  expect(p.take("y".repeat(1001)).text).toBe("[paste #1 1001 chars]");
  expect(rowsFor(10)).toBe(5);
  expect(rowsFor(40)).toBe(12);
});

// pi's editor names the rows its text hides in the middle of its rules.
test("the input's rules say how many rows it hides above and under", () => {
  expect(moreRule(20, "↓", 3)).toBe("───── ↓ 3 more ─────");
  expect(Bun.stringWidth(moreRule(20, "↓", 3))).toBe(20);
  expect(moreRule(8, "↓", 3)).toBe("─".repeat(8));
  // A rule with no run of the rule long enough for it says nothing.
  const rule = withMore(ruleParts(40, "w", "46k/272k", "auto"), 2)
    .map((p) => p.text)
    .join("");
  expect(rule).toBe("── ⏵⏵ auto mode ───────── w · 46k/272k ─");
  expect(
    withMore(ruleParts(40, "w"), 2)
      .map((p) => p.text)
      .join(""),
  ).toBe(`${"─".repeat(13)} ↑ 2 more ${"─".repeat(13)} w ─`);
  expect(ruleOf(40, "w")).toBe(`${"─".repeat(36)} w ─`);
});
