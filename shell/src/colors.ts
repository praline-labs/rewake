// The person's colours, asked of their terminal once at start
// (docs/v2/shell.md): a theme changed while the shell runs is not followed.
import type { TerminalColors } from "@opentui/core"

export type Seed = {
  // widget is written into every pane's widget before its program's bytes,
  // so indexed colours resolve to the person's own and the default
  // foreground and background are theirs.
  widget: string
  // host is REWAKE_HOST_COLORS, "fg;bg", for the space hosts, whose
  // emulators answer the workers' colour queries.
  host: string
}

export function seedOf(colors: TerminalColors | undefined): Seed {
  if (!colors) return { widget: "", host: "" }
  let widget = ""
  colors.palette?.forEach((c, i) => { if (c) widget += `\x1b]4;${i};${c}\x1b\\` })
  const fg = colors.defaultForeground ?? ""
  const bg = colors.defaultBackground ?? ""
  if (fg) widget += `\x1b]10;${fg}\x1b\\`
  if (bg) widget += `\x1b]11;${bg}\x1b\\`
  return { widget, host: fg || bg ? `${fg};${bg}` : "" }
}
