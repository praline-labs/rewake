// The ends of the run's turns not yet answered for good, and their one sender
// (docs/v2/stage4-turns.md#the-mods-side). An end is frozen when it is made:
// written to the harness's store under the run's key before anything is sent,
// and every attempt sends the same body until a receipt or a refusal that
// says it can never be taken.
import { bounds, paths } from '../protocol.gen.ts'
import type { EndedAnswer, Receipt, TurnEndRequest } from '../protocol.gen.ts'

// Host is what the outbox does through the harness, as closures a hook makes
// over its own `$`: the engine refuses a module that passes `$` itself, so the
// timer's callbacks reach the harness through these.
export interface Host {
  post(path: string, request: unknown): Promise<{ status: number; ok: boolean; text: string }>
  get(key: string): Promise<unknown>
  set(key: string, value: unknown): Promise<void>
  remove(key: string): Promise<void>
  after(ms: number, fn: () => void): void
  log(text: string): void
}

// The store is shared by every session on the machine, so each run's keys
// carry its epoch, and an index names the epochs that have any.
export const outboxKey = (epoch: string) => 'rewake/outbox/' + epoch
export const loadKey = (epoch: string) => 'rewake/load/' + epoch
export const indexKey = 'rewake/outboxes'

// The most epochs one mod/ended asks of.
const maxEndedEpochs = 32

// A failed attempt is sent again after 1 s, doubling up to 10 s.
export const firstRetryMs = 1000
export const lastRetryMs = 10000

export function nextDelay(delay: number): number {
  return delay === 0 ? firstRetryMs : Math.min(delay * 2, lastRetryMs)
}

function asList(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : []
}

// listed puts the run's epoch in the index once its key is written, so a
// later load can ask whether this run ended.
export async function listed(host: Host, epoch: string) {
  const index = asList(await host.get(indexKey))
  if (!index.includes(epoch)) {
    await host.set(indexKey, [...index, epoch])
  }
}

// utf8Length counts what the text takes on the wire, as the endpoint's bound
// counts it.
export function utf8Length(text: string): number {
  let bytes = 0
  for (let i = 0; i < text.length; i++) {
    const unit = text.charCodeAt(i)
    if (unit < 0x80) {
      bytes += 1
    } else if (unit < 0x800) {
      bytes += 2
    } else if (unit >= 0xd800 && unit <= 0xdbff && (text.charCodeAt(i + 1) & 0xfc00) === 0xdc00) {
      bytes += 4
      i++
    } else {
      bytes += 3
    }
  }
  return bytes
}

function marker(left: number): string {
  return `\n[rewake: ${left} characters of this answer were left out to keep the report within its bound]`
}

// cut keeps an end's whole body within the endpoint's bound: an answer that
// would pass it is cut at the last whole character that fits, with a line
// saying how many were left out. The report is bounded and says so; it is
// never refused for its size.
export function cut(end: TurnEndRequest): TurnEndRequest {
  const bound = bounds.maxEndBodyBytes
  if (end.answer === undefined || utf8Length(JSON.stringify(end)) <= bound) {
    return end
  }
  const chars = Array.from(end.answer)
  const kept = (count: number): TurnEndRequest => ({
    ...end,
    answer: chars.slice(0, count).join('') + marker(chars.length - count),
  })
  const fits = (count: number) => utf8Length(JSON.stringify(kept(count))) <= bound
  let low = 0
  let high = chars.length - 1
  while (low < high) {
    const middle = Math.ceil((low + high) / 2)
    if (fits(middle)) {
      low = middle
    } else {
      high = middle - 1
    }
  }
  // The marker's count loses a digit as more is kept, so the search may stop
  // a character short of the last that fits.
  while (low + 1 < chars.length && fits(low + 1)) {
    low++
  }
  return kept(low)
}

function isReceipt(text: string, end: TurnEndRequest): boolean {
  try {
    const receipt = JSON.parse(text) as Receipt
    return receipt.turnId === end.turnId && receipt.endId === end.endId
  } catch {
    return false
  }
}

function refusal(text: string): string {
  try {
    return String((JSON.parse(text) as { error?: unknown }).error ?? text)
  } catch {
    return text
  }
}

export class Outbox {
  private ends: TurnEndRequest[] = []
  // busy is an attempt in flight or one scheduled: the outbox's one sender.
  private busy = false
  private delay = 0
  private admitted = false
  private warned = false

  constructor(
    private readonly host: Host,
    private readonly epoch: string,
    private readonly loadId: string,
  ) {}

  // add writes a new end to the outbox. When it is the head and nothing is
  // under way the caller's hook is the sender of its first attempt and
  // awaits it; otherwise it queues behind the older ends and the chain sends
  // it, so a later end never passes an earlier one.
  async add(end: TurnEndRequest) {
    this.ends.push(end)
    await this.persist()
    if (this.admitted && !this.busy && this.ends[0] === end) {
      await this.attempt()
    }
  }

  // admit runs once the load's hello is admitted: the ends the store holds
  // for the run go first, oldest first, under the end ids they were made
  // with, then the ones this load made before; then the keys of the runs
  // rewake proves ended are removed.
  async admit() {
    let stored: TurnEndRequest[] = []
    try {
      const value = await this.host.get(outboxKey(this.epoch))
      stored = Array.isArray(value) ? (value as TurnEndRequest[]) : []
    } catch {
      // An unreadable store leaves this load's own ends in memory.
    }
    const known = new Set(stored.map((end) => end.endId))
    this.ends = [...stored, ...this.ends.filter((end) => !known.has(end.endId))]
    this.admitted = true
    if (this.ends.length > 0 && !this.busy) {
      this.schedule(0)
    }
    await this.sweep()
  }

  private schedule(ms: number) {
    this.busy = true
    this.host.after(ms, () => {
      void this.attempt()
    })
  }

  // attempt sends the head once. A receipt or a final refusal removes it and
  // the chain goes on at once; a failure keeps it and sends it again later.
  private async attempt() {
    const end = this.ends[0]
    if (end === undefined) {
      this.busy = false
      return
    }
    this.busy = true
    let final = ''
    try {
      const answer = await this.host.post(paths.turnEnd, end)
      if (answer.status === 200 ? !isReceipt(answer.text, end) : answer.status === 500 || answer.status === 503) {
        return this.retry()
      }
      if (answer.status !== 200) {
        final = `${answer.status} ${refusal(answer.text)}`
      }
    } catch {
      return this.retry()
    }
    this.delay = 0
    if (final !== '') {
      // 410: the run is proven gone, so the end has no destination; any other
      // refusal says it can never be taken as it is. It did not arrive.
      this.host.log(`rewake: the end of turn ${end.turnId} was not taken and is not sent again: ${final}`)
    }
    this.ends.shift()
    await this.persist()
    if (this.ends.length > 0) {
      this.schedule(0)
    } else {
      this.busy = false
    }
  }

  private retry() {
    this.delay = nextDelay(this.delay)
    this.schedule(this.delay)
  }

  // persist writes the outbox under the run's key; the end leaves the store
  // only when it leaves the outbox.
  private async persist() {
    if (this.epoch === '') {
      return
    }
    try {
      if (this.ends.length > 0) {
        await this.host.set(outboxKey(this.epoch), [...this.ends])
        await listed(this.host, this.epoch)
      } else {
        await this.host.remove(outboxKey(this.epoch))
      }
    } catch (err) {
      if (!this.warned) {
        this.warned = true
        this.host.log(`rewake: the harness's store refused the outbox, so an end not yet taken is kept in memory alone and a reload loses it: ${err}`)
      }
    }
  }

  // sweep asks which other runs of the index rewake proves ended and removes
  // their keys, then their lines. A run alive or that cannot be told keeps
  // them; this run's own key is read by its epoch, never through the index.
  private async sweep() {
    try {
      const others = asList(await this.host.get(indexKey))
        .filter((epoch) => epoch !== this.epoch)
        .slice(0, maxEndedEpochs)
      if (others.length === 0) {
        return
      }
      const answer = await this.host.post(paths.modEnded, { loadId: this.loadId, epochs: others })
      if (!answer.ok) {
        return
      }
      const ended = new Set(asList((JSON.parse(answer.text) as EndedAnswer).ended).filter((epoch) => others.includes(epoch)))
      if (ended.size === 0) {
        return
      }
      for (const epoch of ended) {
        await this.host.remove(outboxKey(epoch))
        await this.host.remove(loadKey(epoch))
      }
      const index = asList(await this.host.get(indexKey))
      await this.host.set(indexKey, index.filter((epoch) => !ended.has(epoch)))
    } catch {
      // A sweep cut short leaves keys a later load removes; it loses no end.
    }
  }
}
