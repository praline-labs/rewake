// The face's link to the mirror and the person's inputs over it, in process
// (link.ts, face.ts): a prompt, Esc and shift+tab reach the session only
// where it takes them, each answer goes to the shell, and a link that fails
// says why and connects again a second later on a fresh snapshot.
import { expect, test } from "bun:test";
import { Face } from "./face.ts";
import { faced, Hand, replies } from "./facekit.ts";
import { contract, type FromFace } from "./messages.ts";
import { bounds } from "./mirror.gen.ts";

const answers = (posts: FromFace[]) => posts.filter((p) => p.kind === "answer");
const errors = (posts: FromFace[]) => posts.flatMap((p) => (p.kind === "error" ? [p.text] : []));

test("a prompt goes to the session, its answer to the shell, and the view back to the bottom", () => {
  const f = faced({ items: replies(Array.from({ length: 30 }, (_, i) => `r ${i}`)) });
  f.tick();
  const bottom = f.shown();
  f.take({ kind: "scroll", by: 10 });
  f.take({ kind: "prompt", id: "p1", text: "next" });
  expect(f.heard).toEqual([{ id: "p1", kind: "prompt", text: "next" }]);
  expect(answers(f.posts)).toEqual([{ kind: "answer", id: "p1", ok: true, detail: undefined }]);
  f.tick();
  expect(f.shown()).toEqual(bottom);
});

test("Esc interrupts a working session that takes it, and passes on a refusal", () => {
  const f = faced({ inputs: ["prompt", "interrupt"], begin: { turn: "working" } });
  f.tick();
  f.take({ kind: "escape", id: "e1" });
  expect(f.heard).toEqual([{ id: "e1", kind: "interrupt", text: "" }]);
  expect(answers(f.posts)).toMatchObject([{ id: "e1", ok: true }]);

  const refused = faced({
    inputs: ["interrupt"],
    begin: { turn: "working" },
    refuse: "pi is not working",
  });
  refused.take({ kind: "escape", id: "e2" });
  expect(answers(refused.posts)).toEqual([
    { kind: "answer", id: "e2", ok: false, detail: "pi is not working" },
  ]);
});

test("Esc while idle, or to a session that takes no interrupt, sends nothing", () => {
  for (const [turn, inputs] of [
    ["idle", ["prompt", "interrupt"]],
    ["working", ["prompt"]],
  ] as const) {
    const f = faced({ inputs: [...inputs], begin: { turn } });
    f.tick();
    f.take({ kind: "escape", id: "e" });
    expect(f.heard).toEqual([]);
    expect(answers(f.posts)).toEqual([]);
  }
});

test("Esc drops a selection first, and only the next one interrupts", () => {
  const f = faced({
    inputs: ["interrupt"],
    begin: { turn: "working" },
    items: replies(["a reply to select"]),
  });
  f.tick();
  const row = f.rowOf("● a reply");
  const selected = () => f.frame().lines[row]?.spans.some((s) => s.selected);
  f.mouse("down", 2, row);
  f.mouse("drag", 9, row);
  expect(selected()).toBe(true);
  f.take({ kind: "escape", id: "e1" });
  expect(selected()).toBe(false);
  expect(f.heard).toEqual([]);
  f.take({ kind: "escape", id: "e2" });
  expect(f.heard.map((h) => h.id)).toEqual(["e2"]);
});

test("shift+tab reaches a session that takes it, whose level the frame carries", () => {
  const f = faced({ inputs: ["prompt", "thinking"], begin: { level: "high" } });
  f.tick();
  expect(f.frame().level).toBe("high");
  f.take({ kind: "thinking", id: "t1" });
  expect(f.heard).toEqual([{ id: "t1", kind: "thinking", text: "" }]);
  f.send({ op: "level", level: "xhigh" });
  f.tick();
  expect(f.frame().level).toBe("xhigh");

  const other = faced({ inputs: ["prompt"] });
  other.tick();
  expect(other.frame().level).toBeUndefined();
  other.take({ kind: "thinking", id: "t2" });
  expect(other.heard).toEqual([]);
});

test("a link the mirror closes shows lost, and connects again a second later afresh", () => {
  const f = faced({ items: replies(["kept"]) });
  f.tick();
  f.mouse("down", 2, 0);
  f.mouse("drag", 5, 0);
  f.cut();
  f.tick();
  expect(f.frame().link).toBe("lost");
  expect(errors(f.posts)).toEqual([]);
  f.take({ kind: "prompt", id: "p", text: "lost" });
  expect(answers(f.posts)).toEqual([
    { kind: "answer", id: "p", ok: false, detail: "the mirror is not connected" },
  ]);
  f.hand.advance(1000);
  expect(f.connects()).toBe(2);
  f.tick();
  expect(f.frame().link).toBe("live");
  expect(f.shown()[0]).toBe("● kept");
  expect(f.frame().lines[0]?.spans.some((s) => s.selected)).toBe(false);
});

test("a stream that does not read, or runs past the bound, ends the link with why", () => {
  const f = faced();
  f.raw(new TextEncoder().encode("not json\n"));
  expect(errors(f.posts)[0]).toStartWith("the mirror's stream does not read: ");
  f.hand.advance(1000);
  expect(f.connects()).toBe(2);
  f.raw(new Uint8Array(bounds.maxLineBytes * 2 + 1).fill(0x61));
  expect(errors(f.posts)[1]).toContain("a line past the protocol's bound");
});

test("a character split between two reads is read whole", () => {
  const f = faced();
  const line = `${JSON.stringify({ op: "add", item: { id: "c", kind: "text", status: "settled", text: "café" } })}\n`;
  const bytes = new TextEncoder().encode(line);
  const cut = bytes.indexOf(0xc3) + 1;
  f.raw(bytes.slice(0, cut));
  f.raw(bytes.slice(cut));
  f.tick();
  expect(f.shown()[0]).toBe("● café");
});

test("a socket that does not open says why and is tried again each second", () => {
  const f = faced({ closed: true });
  return Promise.resolve().then(() => {
    expect(errors(f.posts)).toEqual(["the mirror's socket does not open: Error: no such socket"]);
    f.hand.advance(2000);
    expect(f.connects()).toBe(2);
  });
});

// Review of 288cd7f: a face refused for its contract stays refused, so the
// shell keeps its last frame and the error.
test("a face of another contract says so, then answers pings alone", () => {
  const posts: FromFace[] = [];
  let connects = 0;
  const face = new Face(
    (m) => posts.push(m),
    () => Promise.resolve(connects++),
    new Hand(),
  );
  face.take({ kind: "start", contract: contract + 1, path: "/m.sock", width: 40, height: 30 });
  face.take({ kind: "size", width: 41, height: 30 });
  face.take({ kind: "scroll", by: 1 });
  face.take({ kind: "unfold", all: true });
  face.take({ kind: "ping", n: 7 });
  expect(posts).toEqual([
    { kind: "error", text: `this face speaks contract ${contract}, the shell ${contract + 1}` },
    { kind: "pong", n: 7 },
  ]);
  expect(connects).toBe(0);
});
