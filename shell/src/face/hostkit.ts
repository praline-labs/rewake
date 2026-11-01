// What the face host's tests share: a face's module written to a directory
// of its own, the host that runs it, and a wrapper's mirror socket.
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { FaceHost, type FaceState } from "./host.ts";

export const worker = new URL("../../../face/src/worker.ts", import.meta.url).pathname;
const dirs: string[] = [];
export const ends: (() => void)[] = [];

// cleanup ends what a test started and removes its directories.
export const cleanup = () => {
  for (const end of ends.splice(0)) end();
  for (const dir of dirs.splice(0)) rmSync(dir, { recursive: true, force: true });
};

export function scratch() {
  const dir = mkdtempSync(join(tmpdir(), "face-"));
  dirs.push(dir);
  return dir;
}

// A face's module in a directory of its own, rewritten as a reload would meet
// a change of the sources.
export function faceFile(dir: string, source: string) {
  const path = join(dir, "face.ts");
  writeFileSync(path, source);
  return path;
}

export function hosted(url: string, path = "/nowhere.sock", timing = {}) {
  const states: FaceState[] = [];
  const host = new FaceHost({ path, url, onState: (s) => states.push(s), ...timing });
  ends.push(() => host.end());
  const until = async (ok: (s: FaceState) => boolean, within = 3000) => {
    const deadline = Date.now() + within;
    while (Date.now() < deadline) {
      if (ok(host.state)) return host.state;
      await Bun.sleep(10);
    }
    throw new Error(`no such state: ${JSON.stringify(host.state)}`);
  };
  return { host, states, until };
}

// A wrapper's mirror socket serving one snapshot of a prompt, of replies or
// of the items given, taking the inputs given; it keeps what the face writes
// and answers each input, with the refusal given if any. send writes a change
// to the face connected last.
export type Served = {
  prompt?: string;
  replies?: string[];
  items?: object[];
  begin?: object;
  inputs?: string[];
  refuse?: string;
};

export function mirror(dir: string, served: Served = {}) {
  const path = join(dir, "m.sock");
  const items = served.items ??
    served.replies?.map((text, i) => ({ id: `r${i}`, kind: "text", status: "settled", text })) ?? [
      { id: "p", kind: "prompt", status: "settled", text: served.prompt ?? "" },
    ];
  const lines = [
    { op: "hello", v: 1, inputs: served.inputs ?? ["prompt"] },
    { op: "begin", items: items.length, dropped: 0, turn: "idle", ...served.begin },
    ...items.map((item) => ({ op: "item", item })),
    { op: "end" },
  ];
  const heard: { id: string; kind: string; text: string }[] = [];
  let last: { write(data: string): number } | undefined;
  const server = Bun.listen({
    unix: path,
    socket: {
      open(s) {
        last = s;
        s.write(lines.map((l) => `${JSON.stringify(l)}\n`).join(""));
      },
      data(s, chunk) {
        for (const raw of chunk.toString().split("\n").filter(Boolean)) {
          const input = JSON.parse(raw);
          heard.push(input);
          const answer = served.refuse
            ? { op: "answer", id: input.id, ok: false, detail: served.refuse }
            : { op: "answer", id: input.id, ok: true };
          s.write(`${JSON.stringify(answer)}\n`);
        }
      },
    },
  });
  ends.push(() => server.stop(true));
  const send = (line: object) => last?.write(`${JSON.stringify(line)}\n`);
  return { path, send, heard };
}
