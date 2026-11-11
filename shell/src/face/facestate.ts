// What the shell keeps of one face and how a host of it is set up
// (host.ts).
import type { FaceLine, Link } from "../../../face/src/messages.ts";
import type { PermissionMode, ThinkingLevel } from "../../../face/src/mirror.gen.ts";

export type FaceState = {
  lines: FaceLine[];
  turn: string;
  link: Link | "stopped";
  prompts: boolean;
  // mode is the session's permission mode, and level the thinking level
  // Alt+T moves, as the face heard them from the mirror.
  mode?: PermissionMode;
  level?: ThinkingLevel;
  workMode?: string;
  // error is the face's last failure, shown in its header until it draws again.
  error?: string;
  // said is the answer to the last prompt.
  said?: string;
  // copied says, for a few seconds, what the last copy sent to the clipboard.
  copied?: { text: string; ok: boolean };
  // notice says, for a while, why a reload kept the running face.
  notice?: string;
  // expanded is the person's ctrl+o: the conversation shown whole. The
  // shell keeps it, so a reloaded face opens as the person left it.
  expanded?: boolean;
};

export type FaceOptions = {
  path: string;
  url: string;
  onState: (state: FaceState) => void;
  pingEvery?: number;
  pongWithin?: number;
  promptsExpireAfter?: number;
  // copy puts a text on the person's clipboard through the terminal, and
  // says whether it could.
  copy?: (text: string) => boolean;
  copiedFor?: number;
  noticeFor?: number;
};
