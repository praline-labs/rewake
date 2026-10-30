// How each call of the main loop was let run or refused, for the line the
// face draws under it (docs/face.md, "The model"): the check's own verdict,
// then who settled an ask — rewake's permission request, the person at the
// engine's dialog, or auto mode's classifier. The hooks only observe: nothing
// here changes a verdict, and nothing throws into them.
import type { MirrorPermit } from "../protocol.gen.ts";
import { type CheckEvent, callKey, type DialogEvent } from "./calls.ts";

// Verdict is what the check's next(e) came to, as far as the permit reads it.
export interface Verdict {
  decision: string;
  rule?: unknown;
  hook?: unknown;
  plugin?: unknown;
  setting?: unknown;
}

interface Seen {
  verdict: Verdict;
  // by is who answered an ask, where the mod saw it answered, and allowed
  // what they answered, where the mod saw it.
  by?: "rewake" | "person";
  allowed?: boolean;
  // denied is the reason auto mode's classifier refused the call with.
  denied?: string;
  // edit says auto mode takes the call on its fast path, never asking its
  // classifier: an edit inside the session's directory (2.1.310).
  edit?: boolean;
}

// What the engine tells the model of a refusal by auto mode's classifier,
// the reason after the first (2.1.310): the terminal program draws its own
// line for either.
const classifierRefused = [
  "Permission for this action was denied by the Claude Code auto mode classifier. Reason: ",
  "Permission for this action has been denied. Reason: ",
];
const reasonEnd = ". If you have other tasks";

// What the engine tells the model of a call the person or a hook refused at
// its dialog (2.1.310): the call never ran, though it ends as a failure.
const dialogRefused = [
  "The user doesn't want to proceed with this tool use",
  "Permission for this tool use was denied",
];

// reasonOf is the classifier's reason in a refusal's text, as the terminal
// program cuts it out; undefined when the text is no such refusal.
export function reasonOf(text: string): { reason?: string } | undefined {
  const [first, second] = classifierRefused;
  if (first && text.startsWith(first)) {
    const rest = text.slice(first.length);
    const end = rest.indexOf(reasonEnd);
    return end > 0 ? { reason: rest.slice(0, end) } : {};
  }
  return second && text.startsWith(second) ? {} : undefined;
}

// The tools whose edits inside the session's directory auto mode allows
// without its classifier.
const editTools = ["Edit", "MultiEdit", "Write", "NotebookEdit"];

// A call's record lives until its result, and a bounded number of them:
// a check whose call never came is dropped with the oldest.
const maxSeen = 256;

export class Permits {
  private calls = new Map<string, Seen>();
  // asked are the asks the check left to the mode's decider, by callKey: the
  // engine's dialog carries no call id.
  private asked = new Map<string, string>();

  // checked records a main-loop call's verdict; cwd is the session's
  // directory, where auto mode's fast path applies.
  checked(e: CheckEvent, verdict: Verdict, cwd?: string) {
    if (e.tool_use_id === undefined || e.agentId !== undefined) return;
    const path = (e.input as { file_path?: unknown; notebook_path?: unknown } | undefined) ?? {};
    const file = typeof path.file_path === "string" ? path.file_path : path.notebook_path;
    const edit =
      cwd !== undefined &&
      editTools.includes(e.tool) &&
      typeof file === "string" &&
      (file === cwd || file.startsWith(cwd.endsWith("/") ? cwd : cwd + "/"));
    this.calls.set(e.tool_use_id, { verdict, edit });
    if (verdict.decision === "ask") this.asked.set(callKey(e.tool, e.input), e.tool_use_id);
    for (const [id] of this.calls) {
      if (this.calls.size <= maxSeen) break;
      this.calls.delete(id);
    }
    for (const [key] of this.asked) {
      if (this.asked.size <= maxSeen) break;
      this.asked.delete(key);
    }
  }

  // answered says who settled an ask of a call, and how where it is known.
  answered(id: string | undefined, by: "rewake" | "person", allowed?: boolean) {
    const seen = id === undefined ? undefined : this.calls.get(id);
    if (!seen) return;
    seen.by = by;
    seen.allowed = allowed;
  }

  // dialog says who settled the engine's dialog of a call, known by its key.
  dialog(e: DialogEvent, by: "rewake" | "person", allowed?: boolean) {
    const key = callKey(e.tool_name, e.tool_input);
    this.answered(this.asked.get(key), by, allowed);
    this.asked.delete(key);
  }

  // denied records auto mode's classifier refusing a call.
  denied(id: string | undefined, reason: string | undefined) {
    const seen = id === undefined ? undefined : this.calls.get(id);
    if (seen) seen.denied = reason ?? "";
  }

  // take is the call's permit once it ended, its record forgotten: refused
  // is the text of a refusal, failed a failure's, where the call ended so.
  take(
    id: string,
    ended: { refused?: string; failed?: string },
    mode: string | undefined,
  ): MirrorPermit | undefined {
    const seen = this.calls.get(id);
    this.calls.delete(id);
    if (!seen) return undefined;
    const { verdict } = seen;
    // The engine may tell a refusal as a failure.
    const text = ended.refused ?? ended.failed ?? "";
    const classifier = reasonOf(text);
    const rejected = dialogRefused.some((prefix) => text.startsWith(prefix));
    const allowed =
      verdict.decision !== "deny" &&
      seen.allowed !== false &&
      seen.denied === undefined &&
      ended.refused === undefined &&
      !classifier &&
      !rejected;
    if (verdict.decision !== "ask") {
      const by =
        verdict.hook || verdict.plugin ? "hook" : verdict.rule || verdict.setting ? "rule" : "mode";
      return { by, allowed };
    }
    if (seen.by) return { by: seen.by, allowed };
    if (rejected) return { by: "person", allowed };
    if (classifier || seen.denied !== undefined) {
      const reason = classifier?.reason ?? (seen.denied || undefined);
      return { by: "classifier", allowed: false, ...(reason ? { reason } : {}) };
    }
    if (mode === "auto" && allowed && !seen.edit) return { by: "classifier", allowed };
    return { by: "mode", allowed };
  }
}
