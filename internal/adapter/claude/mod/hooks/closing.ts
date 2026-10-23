// The end of the harness's process, told to its run's wrapper at session.end,
// so rewake's shell knows its closing screen is an end and not a pause
// (docs/v2/shell.md). Measured on 2.1.302: /exit, Ctrl+C and Ctrl+D twice reach
// session.end after the harness has left its screen, 60–120 ms after the key.
// /exit's command.run comes earlier but tells nothing: with background work
// running it opens a dialog that may keep the session.
import type { ClosingRequest } from "../protocol.gen.ts";
import { paths } from "../protocol.gen.ts";
import type { Host } from "./outbox.ts";

// closingWait bounds how long the end waits for the wrapper's answer: one that
// does not answer lets the end go on, and the shell still sees main exit.
export const closingWait = 300;

// The reasons of a session's end whose process exits: clear and resume go on
// in the same process. logout (/logout) ends it too; it was left out until the
// closing screen came late after it, the mark being the earliest sign there.
export const exitReasons = new Set(["prompt_input_exit", "other", "logout"]);

// tellClosing sends session/closing once and waits for its answer within
// closingWait; any failure is silent, being only an earlier sign.
export async function tellClosing(host: Host, loadId: string, reason: string) {
  const request: ClosingRequest = { loadId, reason };
  await Promise.race([
    host.post(paths.sessionClosing, request).catch(() => undefined),
    new Promise<void>((resolve) => host.after(closingWait, resolve)),
  ]);
}
