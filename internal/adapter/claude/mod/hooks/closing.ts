// The end of the harness's process, told to its run's wrapper as it begins,
// so rewake's shell starts its closing screen before the harness's own screen
// goes (docs/v2/shell.md). Measured on 2.1.302: /exit's command.run fires
// before the harness leaves its screen, and Ctrl+C or Ctrl+D twice reach the
// mod first at session.end, some 60 ms after.
import type { ClosingRequest } from "../protocol.gen.ts";
import { paths } from "../protocol.gen.ts";
import type { Host } from "./outbox.ts";

// closingWait bounds how long the end waits for the wrapper's answer: one that
// does not answer lets the end go on, and the shell still sees main exit.
export const closingWait = 300;

// The reasons of a session's end whose process exits: clear and resume go on
// in the same process, and logout is left to the shell's other signs.
export const exitReasons = new Set(["prompt_input_exit", "other"]);

// tellClosing sends session/closing once and waits for its answer within
// closingWait; any failure is silent, being only an earlier sign.
export async function tellClosing(host: Host, loadId: string, reason: string) {
  const request: ClosingRequest = { loadId, reason };
  await Promise.race([
    host.post(paths.sessionClosing, request).catch(() => undefined),
    new Promise<void>((resolve) => host.after(closingWait, resolve)),
  ]);
}
