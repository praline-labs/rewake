// The person's colours, asked of their terminal once at start
// (docs/v2/shell.md): a theme changed while the shell runs is not followed.
import type { CliRenderer, TerminalColors } from "@opentui/core";

export type Seed = {
  // widget is written into every pane's widget before its program's bytes,
  // so indexed colours resolve to the person's own and the default
  // foreground and background are theirs.
  widget: string;
  // host is REWAKE_HOST_COLORS, "fg;bg", for the space hosts, whose
  // emulators answer the workers' colour queries.
  host: string;
  // transparent is set when the terminal gave no background: the panes then
  // leave their default background to the terminal instead of painting black.
  transparent: boolean;
};

export function seedOf(colors: TerminalColors | undefined): Seed {
  if (!colors) return { widget: "", host: "", transparent: true };
  let widget = "";
  colors.palette?.forEach((c, i) => {
    if (c) widget += `\x1b]4;${i};${c}\x1b\\`;
  });
  const fg = colors.defaultForeground ?? "";
  const bg = colors.defaultBackground ?? "";
  if (fg) widget += `\x1b]10;${fg}\x1b\\`;
  if (bg) widget += `\x1b]11;${bg}\x1b\\`;
  return { widget, host: fg || bg ? `${fg};${bg}` : "", transparent: !bg };
}

const reply = /\](4;\d+|10|11);(?:rgb:([0-9a-f]+)\/([0-9a-f]+)\/([0-9a-f]+)|#([0-9a-f]{6}))/i;

function hexOf(r?: string, g?: string, b?: string, six?: string): string {
  if (six) return `#${six.toLowerCase()}`;
  const part = (c: string) =>
    Math.round((parseInt(c, 16) / (16 ** c.length - 1)) * 255)
      .toString(16)
      .padStart(2, "0");
  return `#${part(r!)}${part(g!)}${part(b!)}`;
}

// askColors asks the terminal for its sixteen colours and its default
// foreground and background (OSC 4, 10, 11) once and waits for the replies
// for budgetMs, not for a fixed silence: the renderer's own query gives up
// after 300 ms before the first reply, whatever timeout it is given, and
// then returns nothing without failing, and a terminal that is slow right
// after it starts (Windows Terminal over WSL) answers later. It returns
// undefined when nothing came, and what came when the replies stopped for
// idleMs or all of them arrived.
export async function askColors(
  renderer: CliRenderer,
  budgetMs = 2500,
  idleMs = 300,
): Promise<TerminalColors | undefined> {
  const palette: (string | null)[] = Array(16).fill(null);
  let fg: string | null = null;
  let bg: string | null = null;
  let heard = 0;
  return new Promise((resolve) => {
    let idle: ReturnType<typeof setTimeout> | undefined;
    const finish = () => {
      clearTimeout(idle);
      clearTimeout(limit);
      off();
      if (!heard) return resolve(undefined);
      resolve({ palette, defaultForeground: fg, defaultBackground: bg } as TerminalColors);
    };
    const off = renderer.subscribeOsc((seq) => {
      const m = reply.exec(seq);
      if (!m) return;
      const hex = hexOf(m[2], m[3], m[4], m[5]);
      if (m[1] === "10") fg = hex;
      else if (m[1] === "11") bg = hex;
      else {
        const i = Number(m[1]?.slice(2));
        if (i >= palette.length) return;
        palette[i] = hex;
      }
      heard++;
      if (heard === palette.length + 2) return finish();
      clearTimeout(idle);
      idle = setTimeout(finish, idleMs);
    });
    const limit = setTimeout(finish, budgetMs);
    let q = "\x1b]10;?\x07\x1b]11;?\x07";
    for (let i = 0; i < palette.length; i++) q += `\x1b]4;${i};?\x07`;
    process.stdout.write(q);
  });
}
