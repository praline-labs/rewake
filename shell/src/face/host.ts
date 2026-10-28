// The shell's side of one face (docs/face.md, "Hot reload"): it starts the
// face's worker, forwards sizes, prompts and pings, and keeps what the face
// drew. A reload ends the worker whole and starts it again from the current
// sources; nothing here touches a pane's pty or the shell's process, so a face
// that fails to load, throws or hangs costs its own drawing alone.
import type { FromFace, Link, ToFace } from "./messages.ts";
import type { FaceLine } from "./render.ts";

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
};

let ids = 0;

export class FaceHost {
  state: FaceState = { lines: [], turn: "unknown", link: "connecting", prompts: false };
  private worker: Worker | undefined;
  private width = 80;
  private height = 24;
  private pinged = 0;
  private ponged = 0;
  private pingedAt = 0;
  private timer: ReturnType<typeof setInterval> | undefined;
  private sent = new Map<string, string>();

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
        const text = this.sent.get(message.id);
        this.sent.delete(message.id);
        return this.set({
          said: message.ok
            ? undefined
            : `not sent: ${message.detail ?? "refused"}${text ? ` (${text.slice(0, 40)})` : ""}`,
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
    this.sent.set(id, text);
    this.set({ said: undefined });
    this.send({ kind: "prompt", id, text });
    return true;
  }

  get running() {
    return this.worker !== undefined;
  }

  // end stops the worker, keeping what it drew.
  end() {
    clearInterval(this.timer);
    this.timer = undefined;
    const worker = this.worker;
    this.worker = undefined;
    worker?.terminate();
  }
}
