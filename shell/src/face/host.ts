// The shell's side of one face (docs/face.md, "Hot reload"): it starts the
// face's worker, forwards sizes, prompts and pings, and keeps what the face
// drew. A reload ends the worker whole and starts it again from the current
// sources; nothing here touches a pane's pty or the shell's process, so a face
// that fails to load, throws or hangs costs its own drawing alone.
import type { FaceLine, FromFace, Link, ToFace } from "./messages.ts";

export type FaceState = {
  lines: FaceLine[];
  turn: string;
  link: Link | "stopped";
  prompts: boolean;
  // error is the face's last failure, shown in its header until it draws again.
  error?: string;
  // said is the answer to the last prompt.
  said?: string;
};

export type FaceOptions = {
  path: string;
  url: string;
  onState: (state: FaceState) => void;
  pingEvery?: number;
  pongWithin?: number;
  promptsExpireAfter?: number;
};

let ids = 0;

// A prompt the face sent waits for its answer only while it can still come:
// the wrapper answers within its own 10 s, on the link the prompt went by, to
// the worker that sent it. Its text is kept only as far as the refusal shows
// it, and only the latest few wait.
const maxPrompts = 16;
const shownChars = 40;
const expireAfter = 30_000;

export class FaceHost {
  state: FaceState = { lines: [], turn: "unknown", link: "connecting", prompts: false };
  private worker: Worker | undefined;
  private width = 80;
  private height = 24;
  private pinged = 0;
  private ponged = 0;
  private pingedAt = 0;
  private timer: ReturnType<typeof setInterval> | undefined;
  private sent = new Map<string, { text: string; at: number }>();

  constructor(private readonly options: FaceOptions) {}

  private set(change: Partial<FaceState>) {
    this.state = { ...this.state, ...change };
    this.options.onState(this.state);
  }

  // start runs the face, ended first if it runs.
  start() {
    this.end();
    let worker: Worker;
    try {
      worker = new Worker(this.options.url);
    } catch (err) {
      return this.failed(`the face does not start: ${String(err)}`);
    }
    this.worker = worker;
    // A face that does not load, or throws outside its own catch, ends here:
    // its last lines stay, with the error over them.
    worker.addEventListener("error", (event) => {
      event.preventDefault();
      if (this.worker === worker) this.failed(`the face failed: ${event.message || String(event)}`);
    });
    worker.addEventListener("messageerror", () => {
      if (this.worker === worker) this.failed("the face sent what does not read");
    });
    worker.addEventListener("message", (event: MessageEvent<FromFace>) => {
      if (this.worker === worker) this.heard(event.data);
    });
    this.pinged = this.ponged = 0;
    this.pingedAt = Date.now();
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
    this.send({ kind: "start", path: this.options.path, width: this.width, height: this.height });
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
          error: undefined,
        });
      case "error":
        return this.set({ error: message.text });
      case "answer": {
        const text = this.sent.get(message.id)?.text;
        this.sent.delete(message.id);
        return this.set({
          said: message.ok
            ? undefined
            : `not sent: ${message.detail ?? "refused"}${text ? ` (${text})` : ""}`,
        });
      }
      case "pong":
        this.ponged = Math.max(this.ponged, message.n);
        return;
    }
  }

  private failed(text: string) {
    this.end();
    this.set({ link: "stopped", error: text });
  }

  private send(message: ToFace) {
    try {
      this.worker?.postMessage(message);
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

  // prompt says false when no face runs to take it.
  prompt(text: string): boolean {
    if (!this.worker) return false;
    const id = `p${++ids}`;
    this.expire();
    this.sent.set(id, { text: text.slice(0, shownChars), at: Date.now() });
    for (const old of this.sent.keys()) {
      if (this.sent.size <= maxPrompts) break;
      this.sent.delete(old);
    }
    this.set({ said: undefined });
    this.send({ kind: "prompt", id, text });
    return true;
  }

  // expire forgets the prompts whose answer can no longer come.
  private expire() {
    const before = Date.now() - (this.options.promptsExpireAfter ?? expireAfter);
    for (const [id, prompt] of this.sent) if (prompt.at < before) this.sent.delete(id);
  }

  get running() {
    return this.worker !== undefined;
  }

  // end stops the worker, keeping what it drew; the answers it owed go with
  // it.
  end() {
    this.sent.clear();
    clearInterval(this.timer);
    this.timer = undefined;
    const worker = this.worker;
    this.worker = undefined;
    worker?.terminate();
  }
}
