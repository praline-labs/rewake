import { CustomEditor, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { matchesKey } from "@earendil-works/pi-tui";
import type { Modes } from "./modes.ts";

// Input is intercepted only on the native editor's focus path, never on
// dialogs. Shift+Tab is left to pi's own app.thinking.cycle; only Alt+T is
// taken beside it. All editing, paste and rendering stay native. Another
// extension's replacement editor is outside this bare-pi integration's
// contract.
export function modeEditor(pi: ExtensionAPI, modes: Modes) {
  pi.on("session_start", (_event, ctx) => {
    if (ctx.mode !== "tui") return;
    ctx.ui.setEditorComponent((tui, theme, keys) => {
      class Editor extends CustomEditor {
        private pasting = false;

        handleInput(data: string): void {
          const paste = this.pasting || data.includes("\x1b[200~");
          if (data.includes("\x1b[200~")) this.pasting = true;
          if (data.includes("\x1b[201~")) this.pasting = false;
          if (!paste && matchesKey(data, "alt+t")) {
            void modes.thinking();
            return;
          }
          super.handleInput(data);
        }
      }
      return new Editor(tui, theme, keys);
    });
  });
  pi.on("session_shutdown", (_event, ctx) => {
    if (ctx.mode === "tui") ctx.ui.setEditorComponent(undefined);
  });
}
