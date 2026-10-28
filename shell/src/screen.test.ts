import { expect, test } from "bun:test";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { RGBA } from "@opentui/core";
import { frameOf, listenScreen } from "./screen.ts";

const span = (text: string, width: number, fg: RGBA, bg: RGBA, attributes = 0) => ({
  text,
  width,
  fg,
  bg,
  attributes,
});

test("a captured window becomes runs with colours and the model's attributes", () => {
  const frame = frameOf(
    {
      cols: 6,
      rows: 1,
      cursor: [0, 0],
      lines: [
        {
          spans: [
            span("ab", 2, RGBA.fromInts(255, 0, 0, 255), RGBA.fromInts(0, 0, 0, 0), 1 | 8),
            span("世", 2, RGBA.fromInts(1, 2, 3, 255), RGBA.fromInts(0, 0, 255, 255), 32),
          ],
        },
      ],
    },
    { pane: "main", cursor: { x: 1, y: 0 } },
  );
  expect(frame.lines[0]).toEqual([
    { t: "ab", w: 2, fg: "#ff0000", a: 1 | 8 },
    { t: "世", w: 2, fg: "#010203", bg: "#0000ff", a: 16 },
  ]);
  expect(frame.cursor).toEqual({ x: 1, y: 0 });
});

test("the socket answers a line with a frame, and a bad line with why", async () => {
  const path = join(mkdtempSync(join(tmpdir(), "scr")), "s.sock");
  const stop = listenScreen(path, async (pane) => {
    if (pane === "gone") throw new Error("no window gone to draw");
    // Past what one write of a socket takes.
    if (pane === "big")
      return {
        pane,
        cols: 200,
        rows: 60,
        lines: Array.from({ length: 60 }, () =>
          Array.from({ length: 200 }, () => ({ t: "x", w: 1, fg: "#112233", bg: "#445566" })),
        ),
      };
    // 400x200 cells, each run long in text: inside the cell bounds, over 4 MiB.
    if (pane === "fat")
      return {
        pane,
        cols: 400,
        rows: 200,
        lines: Array.from({ length: 200 }, () =>
          Array.from({ length: 400 }, () => ({
            t: "xxxxxxxx",
            w: 1,
            fg: "#112233",
            bg: "#445566",
            a: 1,
          })),
        ),
      };
    if (pane === "wide") return { pane, cols: 401, rows: 1, lines: [[]] };
    return { pane, cols: 1, rows: 1, lines: [[{ t: "x", w: 1 }]] };
  });
  expect(stop).toBeDefined();
  const ask = async (line: string) => {
    let got = "";
    await new Promise<void>((resolve) => {
      void Bun.connect({
        unix: path,
        socket: {
          open(s) {
            s.write(`${line}\n`);
          },
          data(_s, d) {
            got += Buffer.from(d).toString();
          },
          close() {
            resolve();
          },
        },
      });
    });
    return JSON.parse(got);
  };
  expect((await ask('{"pane":"main"}')).lines[0][0].t).toBe("x");
  const big = await ask('{"pane":"big"}');
  expect(big.lines.length).toBe(60);
  expect(big.lines[59].length).toBe(200);
  expect((await ask('{"pane":"fat"}')).error).toContain("as an answer");
  expect((await ask('{"pane":"wide"}')).error).toContain("over 400x200");
  expect((await ask('{"pane":"gone"}')).error).toContain("no window gone");
  expect((await ask("not json")).error).toContain("JSON");
  expect((await ask("{}")).error).toContain("names no pane");
  stop?.();
});
