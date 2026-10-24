// The closing screen (docs/shell.md, "Who owns which pty"): from the moment main's
// end begins until the shell exits, the whole screen is a spinner and one
// line, a layer over the work as the modal will be, and nothing of the work
// shows under it.
import { createSignal, onCleanup } from "solid-js";
import type { Theme } from "./theme.ts";

const frames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏";

export function Closing(props: { theme: Theme }) {
  const [frame, setFrame] = createSignal(0);
  const timer = setInterval(() => setFrame((f) => (f + 1) % frames.length), 80);
  onCleanup(() => clearInterval(timer));
  return (
    <box
      position="absolute"
      left={0}
      top={0}
      width="100%"
      height="100%"
      zIndex={100}
      justifyContent="center"
      alignItems="center"
      {...(props.theme.background ? { backgroundColor: props.theme.background } : {})}
    >
      <text fg={props.theme.text} content={`${frames[frame()]} closing rewake…`} />
    </box>
  );
}
