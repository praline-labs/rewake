// The shell's side of one face (docs/face.md, "Hot reload"): it starts the
// face's worker, forwards sizes, prompts and pings, and keeps what the face
// drew. A reload starts a worker of the current sources beside the running
// one and ends the old whole once the new speaks this shell's contract;
// nothing here touches a pane's pty or the shell's process, so a face that
// fails to load, throws or hangs costs its own drawing alone.
import {
  contract,
  type FromFace,
  type Pointing,
  refusedBy,
  type ToFace,
} from "../../../face/src/messages.ts";
import { taken } from "./check.ts";
import type { FaceOptions, FaceState } from "./facestate.ts";

export type { FaceState } from "./facestate.ts";

let ids = 0;

// A prompt the face sent waits for its answer only while it can still come:
// the wrapper answers within its own 10 s, on the link the prompt went by, to
// the worker that sent it. Its text is kept only as far as the refusal shows
// it, and only the latest few wait.
const maxPrompts = 16;
const shownChars = 40;
const expireAfter = 30_000;
// A copy is said as long as the terminal program says one sent by OSC 52.
const copiedFor = 4000;
const noticeFor = 10_000;

// Candidate is a reload's face until its first answer says whether it speaks
// this shell's contract, with when, the size and the unfolding it was
// started with.
type Candidate = {
  worker: Worker;
  timer: ReturnType<typeof setTimeout>;
  at: number;
  width: number;
  height: number;
  expanded: boolean;
};

export class FaceHost {
  state: FaceState = { lines: [], turn: "unknown", link: "connecting", prompts: false };
  private worker: Worker | undefined;
  private width = 80;
  private height = 24;
  private pinged = 0;
  private ponged = 0;
  private pingedAt = 0;
  private timer: ReturnType<typeof setInterval> | undefined;
  private sent = new Map<string, { text: string; at: number; verb?: string }>();
  private copiedTimer: ReturnType<typeof setTimeout> | undefined;
  private noticeTimer: ReturnType<typeof setTimeout> | undefined;
  private candidate: Candidate | undefined;
  // told is the contract of the last face a reload kept out: said once.
  private told: number | undefined;

  constructor(private readonly options: FaceOptions) {}

  private set(change: Partial<FaceState>) {
    this.state = { ...this.state, ...change };
    this.options.onState(this.state);
  }

  // start runs the face, ended first if it runs.
  start() {
    this.end();
    const worker = this.spawn();
    if (worker) this.adopt(worker);
  }

  // reload starts a face of the current sources beside the running one, which
  // draws on until the new one answers. A face of another contract would
  // draw nothing until a restart (docs/traps.md), so it is ended and the
  // running one kept; any other answer, a failure included, swaps them as a
  // start does. A stopped face is started.
  reload() {
    if (!this.worker) return this.start();
    this.drop();
    const at = Date.now();
    const worker = this.spawn();
    if (!worker) return;
    // One that never answers is taken in, and its missed ping stops it.
    const timer = setTimeout(() => {
      if (this.candidate?.worker === worker) this.adopt(worker);
    }, this.options.pongWithin ?? 2000);
    const { width, height } = this;
    this.candidate = { worker, timer, at, width, height, expanded: this.state.expanded ?? false };
  }

  // spawn starts a worker and tells it to start, ping 1 sent with it; what it
  // posts is heard once it is the face.
  private spawn(): Worker | undefined {
    let worker: Worker;
    try {
      worker = new Worker(this.options.url);
    } catch (err) {
      this.failed(`the face does not start: ${String(err)}`);
      return;
    }
    // A face that does not load, or throws outside its own catch, ends here:
    // its last lines stay, with the error over them.
    const failed = (text: string) => {
      if (this.candidate?.worker === worker) this.adopt(worker);
      if (this.worker === worker) this.failed(text);
    };
    worker.addEventListener("error", (event) => {
      event.preventDefault();
      failed(`the face failed: ${event.message || String(event)}`);
    });
    worker.addEventListener("messageerror", () => failed("the face sent what does not read"));
    // What the face posts is held to the contract, and only the shell's own
    // copy of it is read.
    worker.addEventListener("message", (event: MessageEvent<unknown>) => {
      const got = taken(event.data);
      if (this.candidate?.worker === worker && !this.agreed(worker, got)) return;
      if (this.worker !== worker) return;
      if ("fault" in got)
        return this.failed(`the face sent what the contract does not allow: ${got.fault}`);
      this.heard(got.message);
    });
    this.send(
      { kind: "start", contract, path: this.options.path, width: this.width, height: this.height },
      worker,
    );
    if (this.state.expanded) this.send({ kind: "unfold", all: true }, worker);
    this.send({ kind: "ping", n: 1 }, worker);
    return worker;
  }

  // agreed takes a reload's face in on its first answer, unless that is the
  // refusal of another contract: then it is ended, the running face draws on,
  // and the person is told once, in the shell's words, what would load it.
  private agreed(worker: Worker, got: ReturnType<typeof taken>) {
    const theirs =
      "message" in got && got.message.kind === "error" ? refusedBy(got.message.text) : undefined;
    if (theirs === undefined) {
      this.adopt(worker);
      return true;
    }
    this.drop();
    if (theirs !== this.told) {
      this.told = theirs;
      this.noticed(
        `the face on disk speaks contract ${theirs}, this shell ${contract}: rewake restart shell loads it`,
      );
    }
    return false;
  }

  // adopt makes a started worker the face, the one it replaces ended; ping 1
  // went with its start.
  private adopt(worker: Worker) {
    const was = this.candidate?.worker === worker ? this.candidate : undefined;
    if (was) {
      clearTimeout(was.timer);
      this.candidate = undefined;
    }
    this.retire();
    this.worker = worker;
    this.told = undefined;
    this.pinged = 1;
    this.ponged = 0;
    this.pingedAt = was?.at ?? Date.now();
    const every = this.options.pingEvery ?? 1000;
    const within = this.options.pongWithin ?? 2000;
    this.timer = setInterval(() => {
      this.expire();
      if (this.ponged < this.pinged && Date.now() - this.pingedAt > within)
        return this.failed(`the face did not answer for ${within / 1000} s and was stopped`);
      if (this.ponged === this.pinged) {
        this.pingedAt = Date.now();
        this.send({ kind: "ping", n: ++this.pinged });
      }
    }, every);
    this.set({ link: "connecting", error: undefined });
    // What changed while it was a candidate.
    if (was && (was.width !== this.width || was.height !== this.height))
      this.send({ kind: "size", width: this.width, height: this.height });
    if (was && was.expanded !== (this.state.expanded ?? false))
      this.send({ kind: "unfold", all: this.state.expanded ?? false });
  }

  private heard(message: FromFace) {
    switch (message.kind) {
      case "lines":
        // A lost link takes the answers of what went by it.
        if (message.link === "lost" && this.state.link !== "lost") this.sent.clear();
        return this.set({
          lines: message.lines,
          turn: message.turn,
          link: message.link,
          prompts: message.prompts,
          mode: message.mode,
          level: message.level,
          workMode: message.workMode,
          error: undefined,
        });
      case "error":
        return this.set({ error: message.text });
      case "copy":
        return this.copied(message.text, message.what);
      case "answer": {
        const sent = this.sent.get(message.id);
        this.sent.delete(message.id);
        const text = sent?.text;
        return this.set({
          said: message.ok
            ? undefined
            : `${sent?.verb ?? "not sent"}: ${message.detail ?? "refused"}${text ? ` (${text})` : ""}`,
        });
      }
      case "pong":
        this.ponged = Math.max(this.ponged, message.n);
        return;
    }
  }

  // copied sends a copy to the clipboard, the shell's only path there, and
  // says so as the terminal program says it.
  private copied(text: string, what: string) {
    const ok = this.options.copy?.(text) ?? false;
    const n = text.length;
    const said = ok
      ? `sent ${what === "selection" ? `${n} chars` : `${what} (${n} chars)`} via OSC 52 · check terminal clipboard settings if paste fails`
      : "not copied: the terminal takes no OSC 52";
    const copied = { text: said, ok };
    clearTimeout(this.copiedTimer);
    this.copiedTimer = setTimeout(() => {
      if (this.state.copied === copied) this.set({ copied: undefined });
    }, this.options.copiedFor ?? copiedFor);
    this.set({ copied });
  }

  // noticed says a notice under the input for a while.
  private noticed(text: string) {
    clearTimeout(this.noticeTimer);
    this.noticeTimer = setTimeout(
      () => this.set({ notice: undefined }),
      this.options.noticeFor ?? noticeFor,
    );
    this.set({ notice: text });
  }

  private failed(text: string) {
    this.end();
    this.set({ link: "stopped", error: text });
  }

  private send(message: ToFace, worker = this.worker) {
    try {
      worker?.postMessage(message);
    } catch (err) {
      this.failed(`the face does not take messages: ${String(err)}`);
    }
  }

  size(width: number, height: number) {
    if (width === this.width && height === this.height) return;
    this.width = width;
    this.height = height;
    this.send({ kind: "size", width, height });
  }

  scroll(by: number) {
    this.send({ kind: "scroll", by });
  }

  wheel(up: boolean) {
    this.send({ kind: "wheel", up, at: Date.now() });
  }

  mouse(type: Pointing, x: number, y: number, focused: boolean) {
    this.send({ kind: "mouse", type, x, y, focused });
  }

  unselect() {
    this.send({ kind: "unselect" });
  }

  // unfold is the person's ctrl+o: everything opened, or folded again.
  unfold() {
    const all = !this.state.expanded;
    this.set({ expanded: all });
    this.send({ kind: "unfold", all });
  }

  // escape is the person's Esc: the face drops its selection, or interrupts
  // the working turn, whose refusal shows as a prompt's does.
  escape() {
    if (!this.worker) return;
    const id = `e${++ids}`;
    this.keep(id, { text: "", at: Date.now(), verb: "not interrupted" });
    this.send({ kind: "escape", id });
  }

  toggleThinking() {
    this.send({ kind: "thinkingVisible" });
  }

  // thinking is Alt+T or Shift+Tab, separate from the retained workmode
  // wire input no key sends now.
  thinking() {
    if (!this.worker) return;
    const id = `t${++ids}`;
    this.keep(id, { text: "", at: Date.now(), verb: "not changed" });
    this.send({ kind: "thinking", id });
  }

  // prompt says false when no face runs to take it.
  prompt(text: string): boolean {
    if (!this.worker) return false;
    const id = `p${++ids}`;
    this.keep(id, { text: text.slice(0, shownChars), at: Date.now() });
    this.set({ said: undefined });
    this.send({ kind: "prompt", id, text });
    return true;
  }

  // keep waits for an input's answer, the latest few only.
  private keep(id: string, input: { text: string; at: number; verb?: string }) {
    this.expire();
    this.sent.set(id, input);
    for (const old of this.sent.keys()) {
      if (this.sent.size <= maxPrompts) break;
      this.sent.delete(old);
    }
  }

  // expire forgets the prompts whose answer can no longer come.
  private expire() {
    const before = Date.now() - (this.options.promptsExpireAfter ?? expireAfter);
    for (const [id, prompt] of this.sent) if (prompt.at < before) this.sent.delete(id);
  }

  get running() {
    return this.worker !== undefined;
  }

  // end stops the worker and a reload's beside it, keeping what it drew; the
  // answers it owed go with it.
  end() {
    this.drop();
    this.retire();
  }

  // drop ends a reload's face not yet taken in.
  private drop() {
    const was = this.candidate;
    this.candidate = undefined;
    if (!was) return;
    clearTimeout(was.timer);
    was.worker.terminate();
  }

  // retire ends the running face, keeping what it drew.
  private retire() {
    this.sent.clear();
    clearTimeout(this.copiedTimer);
    clearInterval(this.timer);
    this.timer = undefined;
    const worker = this.worker;
    this.worker = undefined;
    worker?.terminate();
  }
}
