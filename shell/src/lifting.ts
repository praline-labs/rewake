import { createSignal } from "solid-js";
import { type Clock, realClock } from "./clock.ts";

// The closing layer's state (docs/shell.md). closing is set when main's end
// begins: from then until the shell exits the screen is the closing layer
// alone, and once main's end is sure the keys reach no program. "left" is a
// layer begun by main leaving its screen alone, under which main keeps its
// focus and keys: a leave may be no end.
export type Closing = "" | "left" | "sure";

const closingShown = 600;
// A closing begun by a sign of main's alone — main leaving its screen, the
// earliest, or main's closing mark — is lifted when main comes back to its
// screen, or still runs closingLift after the last sign: a screen that hid a
// main going on would leave the person no way to it.
const closingLift = 5000;

// ending says the shell is already on its way out, which no lift undoes.
export function closingLayer(ending: () => boolean, clock: Clock = realClock) {
  const [closing, setClosing] = createSignal<Closing>("");
  // closingAt is when the layer began, so it is shown at least closingShown
  // before the shell exits.
  let closingAt = 0;
  let closingSign = false;
  let cancelLift: (() => void) | undefined;
  function begin(sign?: "left" | "mark") {
    if (closing() === "sure" && !closingSign) return;
    cancelLift?.();
    closingSign = sign !== undefined;
    if (closingSign) cancelLift = clock.after(closingLift, lift);
    if (!closing()) closingAt = clock.now();
    if (sign !== "left") setClosing("sure");
    else if (!closing()) setClosing("left");
  }
  function lift() {
    if (!closingSign || ending()) return;
    cancelLift?.();
    closingSign = false;
    setClosing("");
  }
  // remaining is how much of the minimum time the layer still has to show.
  const remaining = () => Math.max(0, closingAt + closingShown - clock.now());
  return { closing, begin, lift, remaining };
}
