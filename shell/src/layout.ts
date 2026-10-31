// The screen's measures (docs/shell.md, "The bar"): wide, main's column and
// the worker's side by side, each a pane between its top row and its status
// and ending with the hints or the bar, so both end on the bottom row;
// narrow, the shown pane's column is its top row, the pane and its status,
// and the hints and, while workers exist, the bar take the bottom lines
// across the whole width.
export type Layout = {
  wide: boolean;
  // main and worker are the widths of the columns.
  main: number;
  worker: number;
  rowHeight: number;
  // height is a pane's, its top row and the strip's row under it taken off.
  height: number;
};

export function layoutOf(
  width: number,
  height: number,
  state: { chosen: boolean; zoomed: boolean; workers: number },
): Layout {
  const wide = width >= 120 && state.chosen && !state.zoomed;
  const right = Math.floor((width - 1) / 2);
  const below = wide ? 0 : state.workers > 0 ? 2 : 1;
  const rowHeight = Math.max(height - below, 2);
  return {
    wide,
    main: wide ? width - 1 - right : width,
    worker: wide ? right : width,
    rowHeight,
    height: Math.max(rowHeight - (wide ? 3 : 2), 1),
  };
}
