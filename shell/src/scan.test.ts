import { expect, test } from "bun:test";
import { Scanner, type Seen } from "./scan.ts";

// The preambles main's host writes for a 20x3 screen showing "main"
// (internal/host/terminal/replay.go), on the normal screen and on the
// alternate one.
const normalPreamble =
  "\x1b[0m\x1b[H\x1b[2J\x1b[?69h\x1b[1;1H\x1b[0mmain                \x1b[1;3r\x1b[1;20s\x1b[1;1H\x1b[0m\x1b[1 q\x1b[?25h\x1b7" +
  "\x1b[?1047h\x1b[1;3r\x1b[1;20s\x1b[1;1H\x1b[0m\x1b[1 q\x1b[?25h\x1b7\x1b[?1047l\x1b[?69l\x1b[1;5H\x1b[0m\x1b[1 q\x1b[?25h";
const alternatePreamble =
  "\x1b[0m\x1b[H\x1b[2J\x1b[?69h\x1b[1;3r\x1b[1;20s\x1b[1;1H\x1b[0m\x1b[1 q\x1b[?25h\x1b7\x1b[1;1H\x1b[0m\x1b[1 q\x1b[?25h" +
  "\x1b[?1047h\x1b[1;1H\x1b[0mmain                \x1b[1;3r\x1b[1;20s\x1b[1;1H\x1b[0m\x1b[1 q\x1b[?25h\x1b7\x1b[?69l\x1b[1;5H\x1b[0m\x1b[1 q\x1b[?25h";

const bytes = (s: string) => new Uint8Array(Buffer.from(s, "latin1"));

// seenIn scans text whole, and again cut at every byte, and requires both
// to see the same.
function seenIn(text: string): Seen[] {
  const whole = new Scanner().scan(bytes(text));
  const cut = new Scanner();
  const pieces = [...text].flatMap((c) => cut.scan(bytes(c)));
  expect(pieces).toEqual(whole);
  return whole;
}

// The preamble's hidden visit to the alternate screen covered main with the
// closing layer for 5 s at every attach of a main on its normal screen
// (found by the release gate, November 3, 2026).
test("the host's preamble of a main on its normal screen is no leave", () => {
  expect(seenIn(normalPreamble)).toEqual([]);
});

test("the host's preamble of a main on its alternate screen is main there", () => {
  expect(seenIn(alternatePreamble)).toEqual([{ entered: true }]);
});

test("a main that drew on its alternate screen and leaves it has left", () => {
  expect(seenIn(`${normalPreamble}\x1b[?1049h\x1b[2J\x1b[1;1H> prompt\x1b[?1049l`)).toEqual([
    { entered: true },
    { left: true },
  ]);
  expect(seenIn(`${alternatePreamble}\x1b[?1049lResume this session`)).toEqual([
    { entered: true },
    { left: true },
  ]);
});

test("a leave of a screen nothing was drawn on is no sign", () => {
  expect(seenIn("\x1b[?1049l\x1b[?1049h\x1b[2J\x1b[H\x1b[?25l\x1b[?1049l")).toEqual([]);
});

// Found by the review of the fix above: each case is what the terminal widget
// itself shows for the bytes.
test("setting the screen the program is on again keeps what it drew", () => {
  expect(seenIn("\x1b[?1047hMAIN\x1b[?1047h\x1b[?1047l")).toEqual([
    { entered: true },
    { left: true },
  ]);
});

test("CAN and SUB cancel a string, and what follows is drawn", () => {
  for (const cancel of ["\x18", "\x1a"]) {
    expect(seenIn(`\x1b[?1049hOLD\x1b[?1049l\x1b[?1049h\x1bPz${cancel}MAIN\x1b\\`)).toEqual([
      { entered: true },
      { left: true },
      { entered: true },
    ]);
  }
  expect(seenIn("\x1b[?1049h\x1b]2;t\x18MAIN")).toEqual([{ entered: true }]);
});

test("BEL ends an OSC, not a DCS", () => {
  expect(seenIn("\x1b[?1049h\x1bPz\x07PAYLOAD\x1b\\\x1b[?1049l")).toEqual([]);
  expect(seenIn("\x1b[?1049h\x1b]2;t\x07MAIN")).toEqual([{ title: "t" }, { entered: true }]);
});

// The parser keeps its state and not a long payload, so a string past any
// limit reads the same whole and in reads of 32 KiB.
test("a DCS longer than a mebibyte stays no drawing across reads", () => {
  const text = `\x1b[?1049h\x1bPz${"x".repeat((1 << 20) + 65536)}\x1b\\\x1b[?1049l`;
  const whole = new Scanner().scan(bytes(text));
  const cut = new Scanner();
  const pieces: Seen[] = [];
  for (let at = 0; at < text.length; at += 32768) {
    pieces.push(...cut.scan(bytes(text.slice(at, at + 32768))));
  }
  expect(whole).toEqual([]);
  expect(pieces).toEqual([]);
});

// A completed copy is emitted whole however long, as before the parser.
test("a large copy comes across whole, in one read and in cut ones", () => {
  const original = "a".repeat(786432);
  for (const end of ["\x07", "\x1b\\"]) {
    const text = `\x1b]52;c;${Buffer.from(original).toString("base64")}${end}`;
    for (const size of [text.length, 32768, 65536]) {
      const scanner = new Scanner();
      const got: Seen[] = [];
      for (let at = 0; at < text.length; at += size) {
        got.push(...scanner.scan(bytes(text.slice(at, at + size))));
      }
      expect(got.length).toBe(1);
      expect(got[0]?.copy === original).toBe(true);
    }
  }
});

test("a mode list is read however long its parameters are", () => {
  expect(seenIn(`\x1b[?1002;${"0".repeat(260)}1006hMAIN`)).toEqual([{ mouse: true }]);
  expect(seenIn(`\x1b[?${"0".repeat(5000)};1000h`)).toEqual([{ mouse: true }]);
  expect(seenIn("\x1b[?1002;1006h\x1b[?1002l")).toEqual([{ mouse: true }, { mouse: false }]);
  // Not a private list: nothing in it is a mode.
  expect(seenIn("\x1b[1002h\x1b[?1002x1000h")).toEqual([]);
});

test("a copy, the title and the closing mark still come across reads", () => {
  const copy = Buffer.from("a long selection").toString("base64");
  expect(seenIn(`\x1b]52;c;${copy}\x1b\\\x1b]2;main\x07\x1b]6262;rewake-closing\x07`)).toEqual([
    { copy: "a long selection" },
    { title: "main" },
    { closing: true },
  ]);
});
