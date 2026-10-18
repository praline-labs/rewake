// The drawing of the workers' pane (docs/v2/workers-panel.md): one line per
// worker, a Button its digit presses; the chosen worker's screen as a Raster,
// or a line saying it ended; the key surface over it. Elsewhere than the
// terminal a Raster does not draw, so the pane lists the workers alone.
import { line } from './workers.ts'
import type { Workers } from './workers.ts'

const blank = 0x01000000

// blankCells is a screen of blanks in the default colors, drawn until the
// worker's first screen comes.
function blankCells(columns: number, rows: number): string {
  const data = new Uint8Array(columns * rows * 12)
  const view = new DataView(data.buffer)
  for (let i = 0; i < columns * rows; i++) {
    view.setUint32(i * 12, 0x20, true)
    view.setUint32(i * 12 + 4, blank, true)
    view.setUint32(i * 12 + 8, blank, true)
  }
  let binary = ''
  for (const byte of data) binary += String.fromCharCode(byte)
  return btoa(binary)
}

export async function drawPane($: any, e: any, workers: Workers | undefined) {
  const { Box, Text, Button, Raster, Client } = $.ui.resolve(e)
  const last = workers?.chosen === undefined ? undefined : workers.ended.get(workers.chosen)
  if (workers !== undefined && workers.workers.length === 0 && last !== undefined) {
    return Text({ children: [`${last} ended, and no other worker is open.`] })
  }
  if (workers === undefined || workers.workers.length === 0) {
    return Text({ dimColor: true, children: ["No workers yet: main raises one with rewake space open <name> --harness claude|pi from its shell."] })
  }
  const items = workers.workers.slice(0, 9).map((worker, index) =>
    Button({
      key: 'worker-' + worker.space,
      hotkey: String(index + 1),
      plain: true,
      dimColor: worker.space !== workers.chosen,
      label: (worker.space === workers.chosen ? '▸ ' : '  ') + line(worker),
      onPress: () => {
        if (workers.choose(worker.space)) $.ui.invalidate('ui.render')
      },
    }))
  const chosen = workers.workers.find((one) => one.space === workers.chosen)
  const body = [...items]
  const ended = workers.chosen === undefined ? undefined : workers.ended.get(workers.chosen)
  if (ended !== undefined) {
    body.push(Text({ children: [`${ended} ended: its screen is gone; rewake space list says how it closed.`] }))
  } else if (e.surface !== 'terminal' || Raster === undefined || Client === undefined) {
    body.push(Text({ dimColor: true, children: ["A worker's screen shows in the terminal; here, rewake attach <name> from a terminal reaches it."] }))
  } else if (chosen !== undefined) {
    const columns = Math.max(1, Math.min(512, Number(e.props?.bodyColumns) || 80))
    const rows = Math.max(1, Math.min(512, (Number(e.props?.scroll?.bodyRows) || 24) - items.length - 1))
    workers.drawn(columns, rows)
    const screen = workers.screen?.space === chosen.space ? workers.screen : undefined
    const shown = screen ?? { columns, rows, cells: blankCells(columns, rows) }
    // A Raster takes no click: Claude Code makes a click on it a selection,
    // and the keys stay the prompt's. So the key surface spans the screen
    // and its own line under it, drawn over the Raster: a click anywhere on
    // the screen gives it the keyboard, and it draws only its last line.
    const height = shown.rows + 1
    body.push(Box({ flexDirection: 'column', width: shown.columns, height, children: [
      Raster({ key: 'screen', columns: shown.columns, rows: shown.rows, cells: shown.cells }),
      Box({ position: 'absolute', top: 0, left: 0, width: shown.columns, height, children: [
        Client({ key: 'keys', module: './keys.ts', props: { name: chosen.name, rows: height }, width: shown.columns, height }),
      ] }),
    ] }))
  }
  return Box({ flexDirection: 'column', children: body })
}
