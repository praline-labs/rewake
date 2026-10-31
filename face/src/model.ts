// The face's copy of a session's mirror (docs/face.md): the snapshot the
// wrapper sends on connect, then its changes. The wrapper holds the model to
// its bounds, so this copy keeps whatever it is sent.
import type { InputKind, Item, Line, PermissionMode, TurnState } from "./mirror.gen.ts";
import { bounds } from "./mirror.gen.ts";

export class FaceModel {
  items: Item[] = [];
  turn: TurnState = "unknown";
  // stopped says the last turn line ended the turn by an interrupt.
  stopped = false;
  // since is when the working turn began and worked how the last one ended,
  // both as the wrapper's model timed them (Unix ms), so a face that came
  // after the turn draws what one that watched it does.
  since: number | undefined;
  worked: { took: number; at: number; stopped: boolean } | undefined;
  dropped = 0;
  // mode is the session's permission mode, unknown until it says it.
  mode: PermissionMode | undefined;
  inputs: InputKind[] = [];
  // ready is a snapshot whole: until its end the face shows what it had.
  ready = false;
  private byId = new Map<string, Item>();
  private next: Item[] | undefined;

  // apply takes one line of the stream; it says whether what is drawn
  // changed. A line of another version ends the stream as unreadable.
  apply(line: Line): boolean {
    switch (line.op) {
      case "hello":
        if (line.v !== bounds.version) throw new Error(`the mirror speaks version ${line.v}`);
        this.inputs = line.inputs ?? [];
        return false;
      case "begin":
        this.next = [];
        this.turn = line.turn ?? "unknown";
        this.dropped = line.dropped ?? 0;
        this.stopped = line.stopped === true;
        this.mode = line.mode;
        this.timed(line);
        return false;
      case "item":
        if (line.item) this.next?.push(line.item);
        return false;
      case "end":
        this.items = this.next ?? [];
        this.next = undefined;
        this.byId = new Map(this.items.map((item) => [item.id, item]));
        this.ready = true;
        return true;
      case "add":
      case "set":
        return line.item ? this.put(line.item) : false;
      case "append": {
        const item = line.id === undefined ? undefined : this.byId.get(line.id);
        if (!item || line.text === undefined) return false;
        item.text += line.text;
        return true;
      }
      case "turn":
        this.turn = line.turn ?? "unknown";
        this.stopped = line.stopped === true;
        this.timed(line);
        return true;
      case "mode":
        this.mode = line.mode;
        return true;
      case "drop":
        if (line.id !== undefined && this.byId.delete(line.id))
          this.items = this.items.filter((item) => item.id !== line.id);
        this.dropped = line.dropped ?? this.dropped;
        return true;
      default:
        return false;
    }
  }

  // timed takes the turn's times off a begin or a turn line. A turn that
  // begins forgets how the last ended; the end of one that did not work is
  // not a Done, and leaves the last as it was.
  private timed(line: Line) {
    this.since = this.turn === "working" ? line.since : undefined;
    if (this.turn === "working") this.worked = undefined;
    else if (line.took !== undefined && line.at !== undefined)
      this.worked = { took: line.took, at: line.at, stopped: line.stopped === true };
    else if (line.op === "begin") this.worked = undefined;
  }

  private put(item: Item): boolean {
    const at = this.byId.get(item.id);
    if (at) Object.assign(at, { output: undefined, cut: undefined, permit: undefined }, item);
    else {
      this.items.push(item);
      this.byId.set(item.id, item);
    }
    return true;
  }
}
