import { expect, test } from "bun:test";
import { TextAttributes } from "@opentui/core";
import { testRender } from "@opentui/solid";
import { themeOf } from "../theme.ts";
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
