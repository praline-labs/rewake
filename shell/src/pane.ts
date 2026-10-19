// The pane's terminal widget, registered for Solid as pane_terminal
// (docs/v2/shell.md, "Solid: registering the widget").

import {
  type EmbeddedTerminalOptions,
  EmbeddedTerminalRenderable,
  type RenderContext,
} from "@opentui/core";
import { extend } from "@opentui/solid";
import { keepDefaultForeground } from "./colors.ts";

// Solid makes a renderable with { id } only, so what the widget reads only
// in its constructor is fixed here: assigning onMouseDown later replaces the
// widget's own mouse forwarding. The shell sets the hooks: pressed, read at
// the press since Solid sets the id prop after construction, and standIn, the
// seed's stand-in foreground, before any pane is made.
export const paneHooks: {
  pressed: (id: string) => void;
  standIn?: [number, number, number];
} = { pressed: () => {} };

export class PaneTerminal extends EmbeddedTerminalRenderable {
  constructor(ctx: RenderContext, options: EmbeddedTerminalOptions) {
    const self: { term?: PaneTerminal } = {};
    super(ctx, {
      ...options,
      maxScrollback: 10_000,
      onMouseDown: () => {
        if (self.term) paneHooks.pressed(self.term.id);
      },
    });
    self.term = this;
    // The widget draws every cell in an RGB colour; with no foreground from
    // the terminal its default one goes back to the terminal's own.
    this.renderAfter = (buffer) => {
      if (paneHooks.standIn) keepDefaultForeground(buffer, paneHooks.standIn);
    };
  }
}
extend({ pane_terminal: PaneTerminal });
declare module "@opentui/solid" {
  interface OpenTUIComponents {
    pane_terminal: typeof PaneTerminal;
  }
}
