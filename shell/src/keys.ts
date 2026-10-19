// The prefix, Ctrl+\, and the command after it (docs/v2/shell.md, "The
// prefix and the room"). Read in the renderer's prependInputHandlers, on raw
// sequences before OpenTUI parses them: the terminal sends Ctrl+\ as the byte
// 0x1c, in the kitty protocol's CSI u form (with event types and lock bits),
// or in xterm's modifyOtherKeys form.

export type Command =
  | { kind: "next" | "prev" | "main" | "left" | "right" | "zoom" | "prefix" }
  | { kind: "index"; index: number }

// A press of the prefix: 92 is the backslash, 5 is Ctrl; lock bits (Caps
// Lock 64, Num Lock 128) add to the modifier.
const prefixPress = /^\x1b\[92;(5|69|133|197)(:[12])?u$/
const release = /^\x1b\[\d+(;\d+)?:3[u~A-D]$/
const left = /^(\x1b\[(1;1(:1)?)?D|\x1bOD)$/
const right = /^(\x1b\[(1;1(:1)?)?C|\x1bOC)$/
// A letter or digit under the kitty protocol's report of all keys.
const csiKey = /^\x1b\[(\d+)(;1(:1)?)?u$/

function isPrefix(seq: string): boolean {
  return seq === "\x1c" || seq === "\x1b[27;5;92~" || prefixPress.test(seq)
}

function commandOf(seq: string): Command | undefined {
  if (left.test(seq)) return { kind: "left" }
  if (right.test(seq)) return { kind: "right" }
  if (isPrefix(seq)) return { kind: "prefix" }
  const code = csiKey.exec(seq)?.[1]
  const key = code === undefined ? seq : String.fromCodePoint(Number(code))
  if (key === "n") return { kind: "next" }
  if (key === "p") return { kind: "prev" }
  if (key === "m" || key === "0") return { kind: "main" }
  if (key === "z") return { kind: "zoom" }
  if (/^[1-9]$/.test(key)) return { kind: "index", index: Number(key) }
  return undefined
}

// prefixHandler is the input handler: true takes the sequence from OpenTUI.
// The prefix waits for one key; any other key after it is dropped, as in
// the host mode, and the release of a key taken is taken too.
export function prefixHandler(run: (command: Command) => void): (seq: string) => boolean {
  let pending = false
  return seq => {
    if (release.test(seq)) return pending || seq.startsWith("\x1b[92;")
    if (!pending) {
      pending = isPrefix(seq)
      return pending
    }
    pending = false
    const command = commandOf(seq)
    if (command) run(command)
    return true
  }
}
