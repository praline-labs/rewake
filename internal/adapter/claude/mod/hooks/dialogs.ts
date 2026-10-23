// The hooks of a worker's permission requests and questions
// (docs/permission-requests.md): the mode the harness reports, the check that
// asks main where the person would be asked, the question bridge, and the
// engine's own permission dialog, carried to main where the check carried
// nothing — auto mode, Claude Code's default — and any other dialog the
// session waits at, which main is told of.
import type { Register } from "claude-code";
import { type Asks, allowOnce, allowSession, refuseCall } from "./asks.ts";

// Dialogs is what the hooks read of the current load.
interface Dialogs {
  asks: Asks;
  management: { waits(message?: string): void };
  // run is set once the wrapper admitted the load.
  run?: unknown;
}

// notDialogs are the notifications that name no dialog the session waits at.
const notDialogs = ["idle_prompt", "auth_success"];

// maxDialog bounds what the state carries of a dialog's message.
const maxDialog = 300;

export function serveDialogs(on: Parameters<Register>[0], current: () => Dialogs | undefined) {
  // held counts the dialogs a request carries to main now: the engine's
  // notification of such a dialog is not another dialog.
  let held = 0;
  // The permission mode the harness reports with its classic events, which an
  // ask is carried to main in at the check only where the person would be
  // asked (asks.ts). A call's end also ends its dialog carried to main.
  on("classic.SessionStart", async ($, e, next) => {
    const load = current();
    if (load && e.permission_mode) load.asks.mode = e.permission_mode;
    return next(e);
  });
  on("classic.UserPromptSubmit", async ($, e, next) => {
    const load = current();
    if (load && e.permission_mode) load.asks.mode = e.permission_mode;
    return next(e);
  });
  on("classic.PostToolUse", async ($, e, next) => {
    const load = current();
    if (load && e.permission_mode) load.asks.mode = e.permission_mode;
    load?.asks.ended(e.tool_use_id);
    load?.management.waits();
    return next(e);
  });
  on("classic.PostToolUseFailure", async ($, e, next) => {
    current()?.asks.ended(e.tool_use_id);
    current()?.management.waits();
    return next(e);
  });
  on("classic.PermissionDenied", async ($, e, next) => {
    current()?.asks.ended(e.tool_use_id);
    current()?.management.waits();
    return next(e);
  });

  // A worker's call the engine would ask the person about is asked as a
  // question with three choices, which the question bridge below carries to
  // main too; an exact call allowed for the session passes without asking.
  // An ask the check does not carry is recorded for the dialog that follows.
  // A failure of the bridge refuses the call: nothing has run.
  on("tool.check", async ($, e, next) => {
    const verdict = await next(e);
    const load = current();
    if (verdict.decision !== "ask" || !load) return verdict;
    if (!load.asks.asking(e)) {
      load.asks.checked(e);
      return verdict;
    }
    const cwd = await $.session.cwd();
    const key = load.asks.key(cwd, e);
    if (load.asks.allows(key))
      return { decision: "allow", reason: "allowed for this session through rewake" };
    const question = load.asks.permission(cwd, e, key);
    try {
      const answer = await $.ui.ask(question, {
        header: "Permission",
        options: [allowOnce, allowSession, refuseCall],
      });
      return load.asks.decide(question, answer);
    } catch {
      return load.asks.refused(question);
    }
  }).catch(($, e, next) => ({
    decision: "deny",
    reason:
      "rewake could not carry this permission request (" +
      next.error.message +
      "), so the call was refused.",
  }));

  // Every question dialog of a worker — a permission's and the model's own —
  // is opened for main as well, and the first answer wins (asks.ts). A
  // failure before the dialog opened leaves it to the person.
  on("tool.call", { tool: "AskUserQuestion" }, async ($, e, next) => {
    const load = current();
    if (!load?.run || !load.asks.worker()) return next(e);
    held++;
    try {
      return await load.asks.bridge(e, () => next(e), next.signal);
    } finally {
      held--;
    }
  }).catch(($, e, next) =>
    next.called
      ? { deny: "rewake could not carry this question (" + next.error.message + ")." }
      : next(e),
  );

  // The engine's own permission dialog, drawn where the check carried
  // nothing: it stays on the screen while main is asked, and the first answer
  // wins (docs/research-claude-mod.md#a-workers-dialogs-in-auto-mode). With
  // no decision, or a failure, the dialog is the person's alone.
  on("classic.PermissionRequest", async ($, e, next) => {
    const load = current();
    if (!load?.run) return next(e);
    held++;
    try {
      const decision = await load.asks.dialog(await $.session.cwd(), e, next.signal);
      return decision ? { decision } : next(e);
    } finally {
      held--;
    }
  }).catch(($, e, next) => next(e));

  // Any other dialog a worker waits at — one main cannot answer, or one whose
  // request could not be opened — is known by the notification the engine
  // sends while it waits: the state names it until the next tool result or
  // step, and the wrapper tells main once (docs/permission-requests.md).
  on("classic.Notification", async ($, e, next) => {
    const load = current();
    if (load?.run && load.asks.worker() && held === 0 && !notDialogs.includes(e.notification_type))
      load.management.waits((e.message || e.notification_type).slice(0, maxDialog));
    return next(e);
  });
}
