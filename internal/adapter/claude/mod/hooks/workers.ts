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
// while it is closed, so the pane opens by itself once a worker is there.
export const listOpenMs = 2000
export const listClosedMs = 5000
// A blit refused because the pane is not drawn waits this long.
const unmountedMs = 1000

// Shown is what a blit or a redraw of the pane needs from the harness: the
// pane's id and the Raster's key are fixed.
export interface Shown {
  blit(cells: string, columns: number, rows: number): Promise<{ deny?: string }>
  redraw(): void
  open(): Promise<void>
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
  // size is the screen's size the pane's body leaves, once a drawing said.
  size?: { columns: number; rows: number }
  private sized?: { space: string; columns: number; rows: number }
  private listing = false
  private looping?: string
  private typing = Promise.resolve()
  private opened = false
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
    if (changed) this.shown.redraw()
    if (!this.opened && this.workers.length > 0) {
      // Opened by the mod itself, without the keyboard: Claude Code shows it
      // from 144 columns, and /workers opens it at any width.
      this.opened = true
      void this.shown.open().catch(() => { /* The person opens it with /workers. */ })
    }
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
    if (this.sized?.space === space && this.sized.columns === size.columns && this.sized.rows === size.rows) return
    this.sized = { space, ...size }
    const request: ResizeRequest = { loadId: this.loadId, space, ...size }
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
    const blitted = await this.shown.blit(answer.cells, answer.columns, answer.rows)
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
  // surface's instance and, in order, its keys, each with its number. It
  // answers the last number taken, for the surface to drop what it holds.
  take(data: unknown): number | undefined {
    const batch = data as { from?: unknown; keys?: unknown } | undefined
    if (typeof batch?.from !== 'string' || !Array.isArray(batch.keys)) return undefined
    let last = this.taken.get(batch.from) ?? 0
    for (const item of batch.keys as { n?: unknown; keys?: unknown }[]) {
      if (typeof item?.n === 'number' && item.n > last && typeof item.keys === 'string') {
        this.type(item.keys)
        last = item.n
      }
    }
    this.taken.set(batch.from, last)
    if (this.taken.size > 16) this.taken.delete(this.taken.keys().next().value!)
    return last
  }

  chosenName(): string {
    return this.workers.find((one) => one.space === this.chosen)?.name ?? ''
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

// line is one worker as the pane lists it.
export function line(worker: Worker): string {
  const parts = [worker.name, worker.harness]
  if (worker.model) parts.push(worker.model)
  if (worker.effort) parts.push(worker.effort)
  if (worker.percent !== undefined) parts.push(`ctx ${worker.percent}%`)
  parts.push(worker.state === 'open' ? (worker.activity ?? 'open') : worker.state)
  return parts.join('  ')
}
