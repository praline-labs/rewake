// The prefix, Ctrl+\, and the command after it (docs/v2/shell.md, "The
// prefix and the room"). Read in the renderer's prependInputHandlers, on raw
// sequences before OpenTUI parses them: the terminal sends Ctrl+\ as the byte
// 0x1c, in the kitty protocol's CSI u form (with event types and lock bits),
// or in xterm's modifyOtherKeys form.

export type Command =
  | { kind: "next" | "prev" | "main" | "left" | "right" | "zoom" | "prefix" }
  | { kind: "index"; index: number };

// A press of the prefix: 92 is the backslash, 5 is Ctrl; lock bits (Caps
// Lock 64, Num Lock 128) add to the modifier.
const prefixPress = /^\x1b\[92;(5|69|133|197)(:[12])?u$/;
const release = /^\x1b\[\d+(;\d+)?:3[u~A-D]$/;
const left = /^(\x1b\[(1;1(:1)?)?D|\x1bOD)$/;
const right = /^(\x1b\[(1;1(:1)?)?C|\x1bOC)$/;
// A letter or digit under the kitty protocol's report of all keys.
const csiKey = /^\x1b\[(\d+)(;1(:1)?)?u$/;

function isPrefix(seq: string): boolean {
  return seq === "\x1c" || seq === "\x1b[27;5;92~" || prefixPress.test(seq);
}

function commandOf(seq: string): Command | undefined {
  if (left.test(seq)) return { kind: "left" };
  if (right.test(seq)) return { kind: "right" };
  if (isPrefix(seq)) return { kind: "prefix" };
  const code = csiKey.exec(seq)?.[1];
  const key = code === undefined ? seq : String.fromCodePoint(Number(code));
  if (key === "n") return { kind: "next" };
  if (key === "p") return { kind: "prev" };
  if (key === "m" || key === "0") return { kind: "main" };
  if (key === "z") return { kind: "zoom" };
  if (/^[1-9]$/.test(key)) return { kind: "index", index: Number(key) };
  return undefined;
}

// A typed prefix waits this long for its command.
const prefixLapse = 2000;
// The key code of a CSI sequence, for matching a release to its press.
const codeOf = (seq: string) => /^\x1b\[(\d+)[;u]/.exec(seq)?.[1];

// prefixHandler is the input handler: true takes the sequence from OpenTUI.
// The prefix waits for one key; any other key after it is dropped, and the
// releases of the prefix and of the key taken are taken too. waiting hears
// when a prefix starts and stops waiting, for the hint line.
export function prefixHandler(
  run: (command: Command) => void,
  waiting: (on: boolean) => void = () => {},
  now = Date.now,
): (seq: string) => boolean {
  let pendingAt: number | undefined;
  let taken: string | undefined;
  let lapse: ReturnType<typeof setTimeout> | undefined;
  const settle = () => {
    clearTimeout(lapse);
    pendingAt = undefined;
    waiting(false);
  };
  return (seq) => {
    const released = release.test(seq);
    if (pendingAt !== undefined && now() - pendingAt > prefixLapse) settle();
    if (pendingAt !== undefined) {
      if (released) return true;
      settle();
      taken = codeOf(seq);
      const command = commandOf(seq);
      if (command) run(command);
      return true;
    }
    if (isPrefix(seq)) {
      pendingAt = now();
      waiting(true);
      clearTimeout(lapse);
      lapse = setTimeout(() => {
        if (pendingAt !== undefined) settle();
      }, prefixLapse);
      return true;
    }
    if (released) {
      const code = codeOf(seq);
      if (code !== "92" && (taken === undefined || code !== taken)) return false;
      taken = undefined;
      return true;
    }
    // Another key ends the wait for that release: a terminal reporting no
    // releases never sends it.
    taken = undefined;
    return false;
  };
}
