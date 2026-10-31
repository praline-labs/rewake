// The outer terminal's title is main's alone: a worker's OSC title is kept on its
// pane for the shell's own use, and never forwarded, whichever pane has the focus
// and however the worker is drawn (docs/shell.md, "What the widget lacks").
export function outerTitle(isMain: boolean, title: string | undefined): string | undefined {
  return isMain && title ? title : undefined;
}
