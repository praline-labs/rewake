// Native state and control stay outside the heartbeat's awaited chain: a
// compaction may take minutes while the load must keep renewing its lease.

import type { AskKind, ControlCommand, ControlReply, StateRequest } from "../protocol.gen.ts";
import { paths } from "../protocol.gen.ts";
import type { Host } from "./outbox.ts";

export interface Native {
  id: () => Promise<string>;
  model: () => Promise<string>;
  // cwd is the directory the session runs in now: a worktree entry moves it.
  cwd: () => Promise<string>;
  // usage is $.session.usage() as Claude Code answers it; the host reads it.
  usage: () => Promise<unknown>;
  compact: (
    focus: string,
  ) => Promise<{ skip?: string; tokensBefore?: number; tokensAfter?: number }>;
  abort: (turnId: string) => Promise<void>;
  trace: (text: string) => Promise<unknown>;
  // submit sends a prompt of rewake's face as the person's (mirror.ts).
  submit: (text: string) => Promise<unknown>;
}

type Result = Pick<ControlReply, "outcome"> & Partial<ControlReply>;

export class Management {
  private generation = 0;
  private sequence = 0;
  private compactions = 0;
  private last = { request: "", from: "" };
  private turn?: string;
  // step is the last main-loop step as read: the effort its request asked
  // for and the usage its response reported, the cache's only source.
  private step: { effort?: unknown; usage?: unknown } = {};
  private compacting = false;
  // dialog is what the engine said of a dialog the session waits at that no
  // request carries to main (dialogs.ts), until the next tool result or step.
  private dialog?: string;
  // held are what the dialogs a request carries to main ask, while it holds
  // them (dialogs.ts).
  private held: AskKind[] = [];
  private remote?: ControlCommand;
  private interrupt?: { command: ControlCommand; turn: string; generation: number };
  private sending = Promise.resolve();
  private seen = new Set<string>();

  constructor(
    private readonly host: Host,
    private readonly native: Native,
    private readonly loadId: string,
  ) {}

  admit(generation: number) {
    this.generation = generation;
    this.changed();
  }

  started(turnId: string) {
    this.turn = turnId;
    this.changed();
  }

  // A main-loop request names the effort the session runs at now; the person
  // may change it between turns, and no other source of the engine says it.
  stepStarted(effort: unknown) {
    if (effort === this.step.effort) return;
    this.step = { ...this.step, effort };
    this.changed();
  }

  // stepEnded keeps the usage a main-loop response reported, as reported;
  // the next sample carries it.
  stepEnded(usage: unknown) {
    this.step = { ...this.step, usage };
  }

  // waits says the session waits at a dialog nobody else is told of, or with
  // no message that it no longer does.
  waits(message?: string) {
    if (message === this.dialog) return;
    this.dialog = message;
    this.changed();
  }

  // holds says the session waits at a dialog asking kind while a request
  // carries it, until the returned release.
  holds(kind: AskKind): () => void {
    this.held.push(kind);
    this.changed();
    let released = false;
    return () => {
      if (released) return;
      released = true;
      this.held.splice(this.held.indexOf(kind), 1);
      this.changed();
    };
  }

  // Return the actor for the durable end, and leave the same trace in native
  // history without starting a model turn. Other turns cannot settle it.
  async ended(turnId: string, aborted: boolean): Promise<string | undefined> {
    if (this.turn === turnId) this.turn = undefined;
    this.dialog = undefined;
    const pending = this.interrupt;
    let actor: string | undefined;
    if (pending?.turn === turnId) {
      this.interrupt = undefined;
      if (aborted) {
        actor = `${pending.command.from} interrupted this turn with rewake interrupt`;
        try {
          const result = await this.native.trace(actor);
          if (result && typeof result === "object" && "deny" in result)
            throw new Error("trace refused");
          await this.reply(pending.command, pending.generation, { outcome: "done" });
        } catch {
          await this.reply(pending.command, pending.generation, {
            outcome: "failed",
            detail: "the turn stopped, but its interruption trace could not be recorded",
          });
        }
      } else {
        await this.reply(pending.command, pending.generation, {
          outcome: "refused",
          reason: "no turn running",
        });
      }
    }
    this.changed();
    return actor;
  }

  // Samples are serialized and complete: what the session read of itself
  // goes as read, and a read that failed is absent, so an unknown usage
  // clears an older one instead of keeping pre-compaction tokens as current.
  changed() {
    if (!this.generation) return;
    const state = {
      generation: this.generation,
      activity: this.turn ? "working" : "idle",
      compacting: this.compacting,
      compactions: this.compactions,
      dialog: this.dialog,
      // A permission stops the call whatever else is asked beside it.
      waiting: this.held.includes("permission") ? ("permission" as const) : this.held[0],
      ...this.last,
    };
    const step = this.step;
    this.sending = this.sending
      .then(async () => {
        if (state.generation !== this.generation) return;
        const native: Record<string, unknown> = { step };
        const reads = { usage: this.native.usage, model: this.native.model, cwd: this.native.cwd };
        for (const [key, read] of Object.entries(reads)) {
          try {
            native[key] = await read();
          } catch {
            /* unknown */
          }
        }
        const request: StateRequest = {
          loadId: this.loadId,
          sequence: ++this.sequence,
          conversation: await this.native.id(),
          native,
          ...state,
        };
        await this.host.post(paths.modState, request);
      })
      .catch(() => {
        /* A later native event or heartbeat samples again. */
      });
  }

  compactStarted() {
    this.compacting = true;
    this.changed();
  }

  compactEnded(completed: boolean) {
    this.compacting = false;
    if (completed) {
      this.compactions++;
      this.last = { request: this.remote?.id ?? "", from: this.remote?.from ?? "" };
    }
    this.changed();
  }

  // A plugin-origin compaction is observed here through its returned result;
  // its event skips this plugin's hook, so it is counted once.
  async control(command: ControlCommand, generation: number) {
    if (generation !== this.generation || this.seen.has(command.id)) return;
    this.seen.add(command.id);
    if (this.seen.size > 64) this.seen.delete(this.seen.values().next().value!);
    if (command.conversation !== (await this.native.id())) {
      await this.reply(command, generation, {
        outcome: "refused",
        reason: "not the selected conversation",
      });
      return;
    }
    if (this.compacting || this.remote || this.interrupt) {
      await this.reply(command, generation, {
        outcome: "refused",
        reason: "another request in flight",
      });
      return;
    }
    if (command.action === "compact") {
      if (this.turn) {
        await this.reply(command, generation, { outcome: "refused", reason: "in a turn" });
        return;
      }
      this.remote = command;
      this.compactStarted();
      await this.reply(command, generation, { outcome: "requested" });
      try {
        const result = await this.native.compact(command.focus);
        this.compactEnded(result.skip === undefined);
        await this.reply(
          command,
          generation,
          result.skip === undefined
            ? {
                outcome: "done",
                tokensBefore: result.tokensBefore,
                tokensAfter: result.tokensAfter,
              }
            : { outcome: "refused", detail: result.skip.slice(0, 300) },
        );
      } catch {
        this.compactEnded(false);
        await this.reply(command, generation, {
          outcome: "failed",
          reason: "cut short",
          detail: "native compaction failed or was interrupted",
        });
      } finally {
        this.remote = undefined;
      }
      return;
    }
    if (command.action === "interrupt") {
      if (!this.turn) {
        await this.reply(command, generation, { outcome: "refused", reason: "no turn running" });
        return;
      }
      const pending = { command, generation, turn: this.turn };
      this.interrupt = pending;
      this.host.after(3000, () => {
        if (this.interrupt === pending) {
          void this.reply(command, generation, {
            outcome: "failed",
            open: true,
            reason: "cut short",
            detail: "the turn has not settled after interrupt",
          });
        }
      });
      try {
        await this.native.abort(pending.turn);
      } catch {
        if (this.interrupt === pending) {
          this.interrupt = undefined;
          await this.reply(command, generation, {
            outcome: "failed",
            detail: "native interrupt could not be requested",
          });
        }
      }
      return;
    }
    if (command.action === "prompt" && command.text) {
      // Submitted from the clock, outside the heartbeat's dispatch: a
      // plugin's prompt runs once the session is idle, and a turn may be
      // going on now.
      const text = command.text;
      this.host.after(0, () => {
        void this.native.submit(text).catch(() => undefined);
      });
      await this.reply(command, generation, { outcome: "done" });
      return;
    }
    await this.reply(command, generation, { outcome: "refused", detail: "unknown control action" });
  }

  private async reply(command: ControlCommand, generation: number, result: Result) {
    try {
      await this.host.post(paths.modControl, {
        loadId: this.loadId,
        generation,
        id: command.id,
        reason: "",
        detail: "",
        open: false,
        ...result,
      } satisfies ControlReply);
    } catch {
      /* The host reports a missing acknowledgement as unknown. */
    }
  }
}
