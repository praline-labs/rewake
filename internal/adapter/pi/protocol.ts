import type { Socket } from "node:net";
import type { State } from "./management.ts";
import type { ModePlan } from "./modes.ts";
import type { Forwarded } from "./stream.ts";
import type { ToolFields } from "./tools.ts";

type Waiter = {
  resolve: (frame: Frame) => void;
  reject: (error: Error) => void;
  timer: ReturnType<typeof setTimeout>;
};
export type Link = {
  socket: Socket;
  thread: string;
  ready: boolean;
  next: number;
  waiting: Map<number, Waiter>;
};
export type Turn = { id: string; link: Link; text: string; reason: string };

export type Frame = {
  op: string;
  id: number;
  protocol?: number;
  thread?: string;
  turn?: string;
  outcome?: string;
  text?: string;
  notice?: string;
  ok?: boolean;
  error?: string;
  steered?: boolean;
  state?: State;
  sequence?: number;
  pending?: boolean;
  action?: string;
  request?: string;
  from?: string;
  focus?: string;
  code?: string;
  native?: unknown;
  levels?: string[];
  mode?: ModePlan;
  mirror?: boolean;
  events?: Forwarded[];
} & ToolFields;
