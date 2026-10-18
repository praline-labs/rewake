// The workers' pane (docs/v2/workers-panel.md): the workers of the spaces
// main's run opened, the chosen one's screen as a Raster blitted from a
// detached poll of workers/screen, and its keys from the key surface. Module
// state, as the rest of the mod's: a reload starts the pane over, and the
// next poll brings it back.
import { paths } from '../protocol.gen.ts'
import type { KeysRequest, ResizeRequest, ScreenAnswer, ScreenRequest, Worker, WorkersAnswer, WorkersRequest } from '../protocol.gen.ts'
import { nextDelay } from './outbox.ts'
import type { Host } from './outbox.ts'

export const paneId = 'rewake-workers'

// The list is polled this often while the pane is open, and less often
// while it is closed, so /workers opens it on a list already current.
export const listOpenMs = 2000
export const listClosedMs = 5000
// A blit refused because the pane is not drawn waits this long.
const unmountedMs = 1000
// The Enter after a line waits this long behind it: a line and its Enter in
// one write reach a Claude Code worker as a paste, the Enter a newline in it.
export const enterMs = 50
// A worker is never sized below this: a narrower pane shows its screen
// cropped rather than squeeze the program into a few columns.
export const minColumns = 100
export const minRows = 30

// Shown is what a blit, a redraw or a close of the pane needs from the
// harness: the pane's id and the Raster's key are fixed.
export interface Shown {
  blit(cells: string, columns: number, rows: number): Promise<{ deny?: string }>
  redraw(): void
  close(): void
  // size opens the open pane again at its first or its switched size, for
  // a terminal of so many columns.
  size(switched: boolean, columns: number): void
}

// Screen is the chosen worker's screen as the Raster draws it.
export interface Screen {
  space: string
  seq: number
  columns: number
  rows: number
  cells: string
}

export class Workers {
  workers: Worker[] = []
  chosen?: string
  screen?: Screen
  // ended names, by space, the workers seen to end while the pane showed
  // them, so the pane says so rather than show a last screen.
  ended = new Map<string, string>()
  isOpen = false
  // switched is the pane's other size, which the prefix chose; /workers
  // opens it at its first. columns is the terminal's width /workers ran in:
  // a drawing's viewport is the pane's, not the terminal's.
  switched = false
  columns = 80
  // clearLine asks the next drawing of the line mode's field to empty it.
  clearLine = false
  // size is the screen's size the pane's body leaves, once a drawing said.
  size?: { columns: number; rows: number }
  private sized?: { space: string; columns: number; rows: number }
  private listing = false
  private looping?: string
  private typing = Promise.resolve()
  // taken is the last key number typed, by key surface instance.
  private taken = new Map<string, number>()

  constructor(private readonly host: Host, private readonly shown: Shown, private readonly loadId: string) {}

  // begin starts the list's poll, for a run that hosts spaces.
  begin() {
    if (this.listing) return
    this.listing = true
    this.host.after(0, () => { void this.list() })
  }

  private async list() {
    let changed = false
    try {
      const answer = await this.host.post(paths.workersList, { loadId: this.loadId } satisfies WorkersRequest)
      if (answer.ok) {
        const workers = (JSON.parse(answer.text) as WorkersAnswer).workers
        changed = JSON.stringify(workers) !== JSON.stringify(this.workers)
        for (const gone of this.workers.filter((one) => !workers.some((worker) => worker.space === one.space))) {
          this.ended.set(gone.space, gone.name)
        }
        this.workers = workers
      }
    } catch {
      // The next poll asks again.
    }
    if (this.chosen === undefined || !this.workers.some((one) => one.space === this.chosen)) {
      const first = this.workers[0]?.space
      if (first !== undefined || this.chosen === undefined) {
        changed = this.choose(first) || changed
      }
    }
    // Only /workers opens the pane: one the mod opened by itself took the
    // person's screen while they were at the prompt.
    if (changed) this.shown.redraw()
    this.host.after(this.isOpen ? listOpenMs : listClosedMs, () => { void this.list() })
  }

  // choose makes space the chosen worker; it says whether that changed.
  choose(space: string | undefined): boolean {
    if (space === this.chosen) return false
    this.chosen = space
    this.screen = undefined
    this.looping = undefined
    this.loop()
    this.resize()
    return true
  }

  opens() {
    this.isOpen = true
    this.loop()
  }

  closes() {
    this.isOpen = false
    this.looping = undefined
  }

  // drawn takes the size the pane's body leaves the screen, from a drawing.
  drawn(columns: number, rows: number) {
    if (this.size?.columns === columns && this.size.rows === rows) return
    this.size = { columns, rows }
    this.host.after(0, () => { this.resize() })
  }

  private resize() {
    const space = this.chosen
    const size = this.size
    if (space === undefined || size === undefined || this.ended.has(space)) return
    const wanted = { columns: Math.max(minColumns, size.columns), rows: Math.max(minRows, size.rows) }
    if (this.sized?.space === space && this.sized.columns === wanted.columns && this.sized.rows === wanted.rows) return
    this.sized = { space, ...wanted }
    const request: ResizeRequest = { loadId: this.loadId, space, ...wanted }
    void this.host.post(paths.workersResize, request).catch(() => { /* The next size sends again. */ })
  }

  // loop runs one detached poll of the chosen worker's screen while the pane
  // is open; a new choice or a close ends it at its next answer.
  private loop() {
    const space = this.chosen
    if (!this.isOpen || space === undefined || this.looping === space || this.ended.has(space)) return
    this.looping = space
    this.host.after(0, () => { void this.poll(space, 0) })
  }

  private async poll(space: string, delay: number) {
    if (this.looping !== space) return
    const request: ScreenRequest = { loadId: this.loadId, space, after: this.screen?.space === space ? this.screen.seq : 0 }
    let status = 0
    let text = ''
    try {
      const answer = await this.host.post(paths.workersScreen, request)
      status = answer.status
      text = answer.text
    } catch {
      // No answer: asked again, below.
    }
    if (this.looping !== space) return
    if (status === 404) {
      this.gone(space)
      return
    }
    if (status !== 200) {
      const next = nextDelay(delay)
      this.host.after(next, () => { void this.poll(space, next) })
      return
    }
    let wait = 0
    try {
      wait = await this.show(space, JSON.parse(text) as ScreenAnswer)
    } catch {
      this.host.log('rewake: a malformed screen of a worker')
    }
    if (this.looping === space) this.host.after(wait, () => { void this.poll(space, 0) })
  }

  // show draws a screen: a blit at the mounted size, a redraw at another;
  // it answers how long the next poll waits.
  private async show(space: string, answer: ScreenAnswer): Promise<number> {
    if (answer.ended) {
      this.gone(space)
      return 0
    }
    const old = this.screen
    if (old?.space === space && old.seq === answer.seq) return 0
    this.screen = { space, seq: answer.seq, columns: answer.columns, rows: answer.rows, cells: answer.cells }
    if (old?.space !== space || old.columns !== answer.columns || old.rows !== answer.rows) {
      this.shown.redraw()
      return 0
    }
    const view = this.view(this.screen)
    const blitted = await this.shown.blit(view.cells, view.columns, view.rows)
    if (blitted.deny !== undefined) {
      // Not drawn now — another pane in front, a view switched: the next
      // drawing takes the screen from here.
      return unmountedMs
    }
    return 0
  }

  private gone(space: string) {
    const name = this.workers.find((one) => one.space === space)?.name ?? space
    this.ended.set(space, name)
    if (this.looping === space) this.looping = undefined
    this.shown.redraw()
  }

  // take types what a key surface posted that it did not type before: the
  // surface's instance and, in order, its keys and the pane's commands, each
  // with its number. It answers the last number taken, for the surface to
  // drop what it holds.
  take(data: unknown): number | undefined {
    const batch = data as { from?: unknown; keys?: unknown } | undefined
    if (typeof batch?.from !== 'string' || !Array.isArray(batch.keys)) return undefined
    let last = this.taken.get(batch.from) ?? 0
    for (const item of batch.keys as { n?: unknown; keys?: unknown; choose?: unknown; close?: unknown; size?: unknown }[]) {
      if (typeof item?.n !== 'number' || item.n <= last) continue
      if (typeof item.keys === 'string') this.type(item.keys)
      else if (typeof item.choose === 'number') this.chooseNth(item.choose)
      else if (item.close === true) this.close()
      else if (item.size === true) this.switchSize()
      else continue
      last = item.n
    }
    this.taken.set(batch.from, last)
    if (this.taken.size > 16) this.taken.delete(this.taken.keys().next().value!)
    return last
  }

  // view is a screen as the pane's body shows it: cropped to the body, its
  // bottom rows kept, where a program keeps its prompt.
  view(screen: { columns: number; rows: number; cells: string }) {
    return this.size === undefined ? screen : crop(screen, this.size.columns, this.size.rows)
  }

  // switchSize switches the pane between its first and its other size.
  switchSize() {
    if (!this.isOpen) return
    this.switched = !this.switched
    this.shown.size(this.switched, this.columns)
  }

  // chooseNth chooses the worker of a digit, as its button's hotkey does.
  private chooseNth(digit: number) {
    const space = this.workers.slice(0, 9)[digit - 1]?.space
    if (space !== undefined && this.choose(space)) this.shown.redraw()
  }

  // close closes the pane from its prefix or its button; marking it closed
  // here does not lean on the ui.close hook hearing a close raised inside
  // another hook.
  close() {
    if (!this.isOpen) return
    this.closes()
    this.shown.close()
  }

  chosenName(): string {
    return this.workers.find((one) => one.space === this.chosen)?.name ?? ''
  }

  // typeLine sends a line, then its Enter on its own.
  typeLine(line: string) {
    this.type(line)
    if (line === '') this.type('\r')
    else this.host.after(enterMs, () => { this.type('\r') })
  }

  // type sends keys to the chosen worker, one post after another, so they
  // reach it in the order typed.
  type(keys: string) {
    const space = this.chosen
    if (space === undefined || keys === '' || this.ended.has(space)) return
    const request: KeysRequest = { loadId: this.loadId, space, keys }
    this.typing = this.typing
      .then(() => this.host.post(paths.workersKeys, request))
      .then(() => undefined, () => { this.host.log('rewake: keys did not reach the worker') })
  }
}

// crop cuts a screen of Raster cells, 12 bytes each, to its left columns
// and its bottom rows.
export function crop(screen: { columns: number; rows: number; cells: string }, columns: number, rows: number) {
  columns = Math.min(columns, screen.columns)
  rows = Math.min(rows, screen.rows)
  if (columns === screen.columns && rows === screen.rows) return screen
  const from = Uint8Array.from(atob(screen.cells), (ch) => ch.charCodeAt(0))
  const out = new Uint8Array(columns * rows * 12)
  const top = screen.rows - rows
  for (let row = 0; row < rows; row++) {
    const at = ((top + row) * screen.columns) * 12
    out.set(from.subarray(at, at + columns * 12), row * columns * 12)
  }
  let binary = ''
  for (const byte of out) binary += String.fromCharCode(byte)
  return { columns, rows, cells: btoa(binary) }
}

// line is one worker as the pane lists it.
export function line(worker: Worker): string {
  const parts = [worker.name, worker.harness]
  if (worker.model) parts.push(worker.model)
  if (worker.effort) parts.push(worker.effort)
  if (worker.percent !== undefined) parts.push(`ctx ${worker.percent}%`)
  parts.push(worker.state === 'open' ? (worker.activity ?? 'open') : worker.state)
  return parts.join('  ')
}
