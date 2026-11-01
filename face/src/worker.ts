// rewake's face for one session, run as a Bun Worker the shell starts and may
// end at any moment (docs/face.md, "Hot reload"): the face itself is face.ts,
// handed the worker's post, Bun's unix socket and the real clock here.

import { Face } from "./face.ts";
import type { ToFace } from "./messages.ts";

declare const self: Worker;

const face = new Face(
  (message) => self.postMessage(message),
  (path, wire) => Bun.connect({ unix: path, socket: wire }),
);

self.onmessage = (event: MessageEvent<ToFace>) => face.take(event.data);
