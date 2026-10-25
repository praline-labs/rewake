// A worker's permission requests and questions, carried to main
// (docs/permission-requests.md). A permission the engine would ask the person
// about becomes a question of the engine's own dialog with three choices;
// every question dialog — that one, and the model's own AskUserQuestion — is
// opened as a request of the run's wrapper, which tells main, and held by
// successive polls while the person at the screen may answer it too. The
// first answer wins: the person's through the dialog, main's through a poll,
// or the deadline's refusal. A permission dialog the engine draws itself — in
// auto mode, where the check carries nothing — is opened the same way when it
// is drawn and answered with main's decision. Nothing is kept past this load.

import type {
  AskKind,
  AskOpenAnswer,
  AskOpenRequest,
  AskPollAnswer,
  AskQuestion,
  AskWinner,
} from "../protocol.gen.ts";
import { paths } from "../protocol.gen.ts";
import type { CheckEvent, DialogDecision, DialogEvent, QuestionEvent } from "./calls.ts";
import { callKey, canonical, summary } from "./calls.ts";
import type { Host } from "./outbox.ts";

// The choices of a permission, in the order the wrapper reads main's once,
// session and refuse against.
export const allowOnce = "Allow once";
export const allowSession = "Allow for this session";
export const refuseCall = "Refuse";

// The roles whose requests go to main: the sessions that take work.
const workerRoles = ["general", "write"];

// The modes in which an ask is the person's to answer: an ask in any other
// mode is the mode's to settle — the classifier's, a plan's — or never asked.
const askingModes = ["default", "manual", "acceptEdits"];

// The tools whose asks stay at the screen: the question tool is the bridge
// itself, and a plan's approval is the person's.
const screenOnly = ["AskUserQuestion", "ExitPlanMode"];

// maxInput bounds a call's arguments a request carries, within a body.
const maxInput = 16_000;

// The refusal of a request nobody answered in time, which the model reads.
export const unanswered =
  "No answer came from main or the person at the screen within the deadline, so this was refused. " +
  'Do not retry it blindly: if the work needs it, end the turn with rewake pending "needs <what>".';

// atScreen is the result of a dialog the person answered at the screen.
const atScreen = Symbol("answered at the screen");

interface Permission {
  tool: string;
  input: string;
  cwd: string;
  key: string;
  // readOnly says the check called the call read-only.
  readOnly: boolean;
  // reason is main's reason for a refusal; expired says the deadline passed.
  reason?: string;
  expired?: boolean;
}

export class Asks {
  // role is the run's role, from the wrapper's answer to the hello; mode is
  // the permission mode the harness last reported.
  role?: string;
  mode?: string;
  // allowed are the exact calls main or the person allowed for this load.
  private allowed = new Set<string>();
  // permissions are the open permission questions, by their text.
  private permissions = new Map<string, Permission>();
  private serial = 0;
  // checks are the asks the check left to the engine, by callKey: the call's
  // id, whether an administrator's ceiling made it the person's, and whether
  // the check called it read-only.
  private checks = new Map<string, { id: string; ceiling: boolean; readOnly: boolean }>();
  // waiting are the dialogs carried to main, each ended when its call is over.
  private waiting: { id?: string; end: () => void }[] = [];

  constructor(
    private host: Host,
    private loadId: string,
  ) {}

  worker(): boolean {
    return this.role !== undefined && workerRoles.includes(this.role);
  }

  // asking says whether an ask of the engine's is carried to main: a real
  // call of a worker in a mode where the person would be asked, of a tool
  // whose ask is not the screen's alone, with no administrator's ceiling.
  asking(e: CheckEvent): boolean {
    return (
      this.worker() &&
      e.tool_use_id !== undefined &&
      e.ceiling === undefined &&
      this.mode !== undefined &&
      askingModes.includes(this.mode) &&
      !screenOnly.includes(e.tool)
    );
  }

  key(cwd: string, e: CheckEvent): string {
    return cwd + "\u0000" + e.tool + "\u0000" + canonical(e.input);
  }

  allows(key: string): boolean {
    return this.allowed.has(key);
  }

  // permission names the question a permission is asked as, unique to it.
  permission(cwd: string, e: CheckEvent, key: string): string {
    this.serial++;
    const question = `Allow ${e.tool}: ${summary(e.tool, e.input)} [${this.serial}]?`;
    let input = canonical(e.input);
    if (input.length > maxInput)
      input = input.slice(0, maxInput) + ` ... (${input.length - maxInput} characters more)`;
    this.permissions.set(question, {
      tool: e.tool,
      input,
      cwd,
      key,
      readOnly: e.isReadOnly === true,
    });
    return question;
  }

  // kindOf says what a question dialog asks: a permission this load turned
  // into a question, or the model's own question.
  kindOf(e: QuestionEvent): AskKind {
    return this.permissions.has(e.questions[0]?.question ?? "") ? "permission" : "question";
  }

  // decide is the verdict an answer to a permission's question gives.
  decide(question: string, answer: string): { decision: "allow" | "deny"; reason?: string } {
    const permission = this.permissions.get(question);
    this.permissions.delete(question);
    if (answer === allowOnce || answer === allowSession) {
      if (answer === allowSession && permission) this.allowed.add(permission.key);
      return { decision: "allow", reason: "allowed through rewake: " + answer.toLowerCase() };
    }
    if (answer === refuseCall) {
      return {
        decision: "deny",
        reason:
          "Refused" +
          (permission?.reason ? ": " + permission.reason : " by main or the person at the screen") +
          ". Do not retry this call.",
      };
    }
    return { decision: "deny", reason: "Not allowed; the answer was: " + answer };
  }

  // refused is the verdict of a permission's question that ended with no
  // answer: past its deadline, or closed.
  refused(question: string): { decision: "deny"; reason: string } {
    const permission = this.permissions.get(question);
    this.permissions.delete(question);
    return {
      decision: "deny",
      reason: permission?.expired
        ? unanswered
        : "The permission question was closed with no answer, so the call was refused.",
    };
  }

  // checked records an ask the check left to the engine, so the dialog that
  // follows it, which carries no call id, is known by its call.
  checked(e: CheckEvent) {
    if (!this.worker() || e.tool_use_id === undefined) return;
    this.checks.set(callKey(e.tool, e.input), {
      id: e.tool_use_id,
      ceiling: e.ceiling !== undefined,
      readOnly: e.isReadOnly === true,
    });
  }

  // ended says a carried dialog is over at the screen: its call ran or was
  // refused, or, with no id, the loop went on — a next step or the turn's
  // end comes only once every dialog of the step is answered.
  ended(id?: string) {
    this.waiting = this.waiting.filter((one) => {
      if (id !== undefined && one.id !== id) return true;
      one.end();
      return false;
    });
  }

  // dialog carries a permission dialog the engine drew — in auto mode, or any
  // mode the check did not carry — to main as a permission request, held
  // while the person may answer it at the screen. Its answer is main's
  // decision, the deadline's refusal, or undefined: the person answered, or
  // the dialog is not main's to answer.
  async dialog(
    cwd: string,
    e: DialogEvent,
    signal: AbortSignal,
  ): Promise<DialogDecision | undefined> {
    if (!this.worker() || screenOnly.includes(e.tool_name)) return undefined;
    const call = this.checks.get(callKey(e.tool_name, e.tool_input));
    this.checks.delete(callKey(e.tool_name, e.tool_input));
    if (call?.ceiling) return undefined;
    const check: CheckEvent = {
      tool: e.tool_name,
      input: e.tool_input,
      ...(call?.readOnly ? { isReadOnly: true as const } : {}),
    };
    const key = this.key(cwd, check);
    if (this.allows(key)) return { behavior: "allow" };
    const question = this.permission(cwd, check, key);
    const asked: QuestionEvent = {
      ...(call ? { tool_use_id: call.id } : {}),
      questions: [
        {
          question,
          header: "Permission",
          options: [{ label: allowOnce }, { label: allowSession }, { label: refuseCall }],
        },
      ],
    };
    const id = await this.open(asked);
    if (id === undefined) {
      this.permissions.delete(question);
      return undefined;
    }
    const over = new Promise<unknown>((end) => {
      this.waiting.push({ id: call?.id, end: () => end(atScreen) });
    });
    const result = await this.hold(id, asked, () => over, signal);
    if (result === atScreen) {
      this.permissions.delete(question);
      return undefined;
    }
    const denied = (result as { deny?: string }).deny;
    if (denied !== undefined) {
      this.permissions.delete(question);
      return { behavior: "deny", message: denied };
    }
    const answers = (result as { result?: { answers?: Record<string, string> } }).result?.answers;
    const verdict = this.decide(question, answers?.[question] ?? "");
    return verdict.decision === "allow"
      ? { behavior: "allow" }
      : { behavior: "deny", message: verdict.reason ?? "Refused." };
  }

  // bridge holds one question dialog: the dialog itself, which native()
  // opens and the person may answer, against main's answer through the
  // wrapper's polls, until the first of them or the deadline. A request the
  // wrapper will not open leaves the dialog the person's alone.
  async bridge(
    e: QuestionEvent,
    native: () => Promise<unknown>,
    signal: AbortSignal,
  ): Promise<unknown> {
    const id = await this.open(e);
    if (id === undefined) return native();
    return this.hold(id, e, native, signal);
  }

  // open asks the wrapper to open a request for the dialog's questions, and
  // is its id, or undefined when the wrapper will not.
  private async open(e: QuestionEvent): Promise<string | undefined> {
    const questions: AskQuestion[] = e.questions.map((one) => {
      const question: AskQuestion = {
        question: one.question,
        options: one.options.map((option) =>
          typeof option === "string"
            ? { label: option }
            : {
                label: option.label,
                ...(option.description ? { description: option.description } : {}),
              },
        ),
      };
      if (one.header) question.header = one.header;
      if (one.multiSelect) question.multiSelect = true;
      return question;
    });
    const permission = this.permissions.get(e.questions[0]?.question ?? "");
    const open: AskOpenRequest = {
      loadId: this.loadId,
      call: e.tool_use_id ?? e.questions[0]?.question ?? "",
      kind: this.kindOf(e),
      questions,
    };
    if (permission) {
      open.tool = permission.tool;
      open.input = permission.input;
      open.cwd = permission.cwd;
      if (permission.readOnly) open.readOnly = true;
    }
    try {
      const opened = await this.host.post(paths.askOpen, open);
      if (!opened.ok) return undefined;
      return (JSON.parse(opened.text) as AskOpenAnswer).id;
    } catch {
      return undefined;
    }
  }

  // hold races the open request id: native(), the person's answer at the
  // screen, against main's answer through the wrapper's polls and the
  // deadline; the first wins, and the wrapper is told which.
  private async hold(
    id: string,
    e: QuestionEvent,
    native: () => Promise<unknown>,
    signal: AbortSignal,
  ): Promise<unknown> {
    const permission = this.permissions.get(e.questions[0]?.question ?? "");
    let winner: { by: AskWinner; result: unknown } | undefined;
    const local = native().then(
      (result) => result,
      (error) => ({ deny: String(error) }),
    );
    void local.then((result) => {
      if (winner === undefined) {
        winner = { by: "local", result };
        void this.settle(id, "local");
      }
    });
    while (winner === undefined && !signal.aborted) {
      let reply: AskPollAnswer;
      try {
        const polled = await this.host.post(paths.askPoll, { loadId: this.loadId, id });
        if (!polled.ok) break;
        reply = JSON.parse(polled.text) as AskPollAnswer;
      } catch {
        break;
      }
      if (winner !== undefined) break;
      if (reply.state === "answered") {
        const answers: Record<string, string> = {};
        for (const one of reply.answers ?? []) answers[one.question] = one.answer;
        if (permission && reply.reason) permission.reason = reply.reason;
        winner = { by: "main", result: { result: { questions: e.questions, answers } } };
      } else if (reply.state === "expired") {
        if (permission) permission.expired = true;
        winner = { by: "expired", result: { deny: unanswered } };
      } else if (reply.state === "settled") {
        break;
      }
    }
    if (winner === undefined && signal.aborted) {
      winner = { by: "aborted", result: { deny: "The call was interrupted." } };
    }
    if (winner === undefined) {
      // The wrapper stopped answering: the dialog is the person's alone.
      return local;
    }
    if (winner.by !== "local") void this.settle(id, winner.by);
    return winner.result;
  }

  private async settle(id: string, winner: AskWinner): Promise<void> {
    try {
      await this.host.post(paths.askSettle, { loadId: this.loadId, id, winner });
    } catch {
      // A settling lost leaves the request to its deadline on the wrapper.
    }
  }
}
