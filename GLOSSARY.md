# Glossary

The words this project gives a meaning of its own. One article per term; a new term gets its
article when it settles, a changed meaning rewrites its article.

## Room
Variants: rooms, комната
Means: the sessions started from one launch directory: one main and the workers it raised.
Where: the room's state lives under `v2/rooms/<room>` of the state directory; the owner says «комната» for that folder: письма, брифы, кто жив.

## main
Means: the orchestrator session of a room: it hands out tasks, reads the reports and decides; it writes no product code itself. `rewake claude` or `rewake pi` started by a person always starts a main.

## Worker
Variants: workers
Means: a session main raised from a worker template to do one kind of work; a person never starts one from a terminal.

## Worker template
Variants: template, templates
Means: a named recipe for a worker — harness, model, effort and role — raised with `rewake space open --alias`. The owner's room has three: two writers and one reviewer-advisor.
Where: the owner says «шаблон воркера».

## Shell
Variants: оболочка
Means: the program on bun that draws the Экран of a main started at a terminal; a child of main's Место, restarted without ending a session.
Avoid: host mode
Where: `shell/`, on OpenTUI and Solid.

## Face
Variants: faces, native face, rewake face
Means: what a worker's pane shows: `native`, the harness's own TUI, or `rewake`, the shell's drawing of the session's mirror.
Where: [docs/face.md](docs/face.md).

## Mirror
Means: the harness-free model of a session's conversation kept in memory in the session's wrapper and served to the shell's face; it reads no transcript and writes nothing.
Where: `internal/core/mirror`, [docs/face.md](docs/face.md).

## Part
Variants: parts, manifest, restart class
Means: one piece of rewake as installed — `rewake`, `shell`, `face`, a harness rewake ships — listed with its digest, contracts and restart class (live, shell, sessions, room) in the manifest beside the installed `rewake`; `rewake restart` with no part says which parts the room must restart.
Where: `internal/infra/parts`, [docs/install.md](docs/install.md#the-parts-and-what-an-update-restarts).

## Dev worker
Variants: dev workers
Means: a worker raised with `rewake space open --dev`, or any pi worker, whose wrapper serves its mirror, so the shell can draw it with rewake's face.

## Task
Variants: tasks
Means: a letter sent with `rewake send` without a flag; it binds the recipient to a report.
Where: the owner says «задача».

## Brief
Variants: briefs, `rewake brief`, `--brief`
Means: a markdown document for one task that main keeps in the room — its goal, its checkbox items, anything markdown holds; main alone writes it, a worker it was given to with `send --brief` ticks its items and comments, a reviewer given it with `--brief --review` reads and comments but never ticks, the person reads it in the shell. Not the briefing, the text a session gets at its launch; a brief in `.scratch/` is a file a task names, which may hold the same.
Where: [docs/briefs.md](docs/briefs.md); the records under `briefs/` of the room.

## Notify
Variants: `--notify`
Means: a letter that points to something or informs; it owes no report.
Where: the owner says «уведомление».

## finished
Means: the report of a worker's turn that ended normally; it reaches whoever awaits the report.

## error
Means: the report of a turn that broke off — a policy filter, the network, a limit — with the reason as it came; the task may be sent again.

## stopped
Means: the report of a turn a person stopped with Ctrl+C or Esc; only those awaiting a report get it, nothing is owed, and the task is not sent again — the person decides.

## `rewake.next`
Means: a new build installed beside the running `rewake2`; `tools/land` swaps it in right after landing and green checks when rewake's sources changed, and says what the room must restart to run it.

## `unread/`
Means: the folder of a session's mailbox holding the letters not yet read, one JSON file each with `from`, `kind` and `text`.

## `done/`
Means: the folder of a session's mailbox holding the letters already read.

## `.scratch/`
Means: the git-ignored folder for the working files of a delegation or a review — briefs, reports, decision files; `.scratch/process/` is main's and survives a compaction and a reboot.

## Экран
Means: все, что человек видит в терминале main: два Окна, у каждого Шапка, Хвост и Статус, внизу Подсказки и Навигация; его рисует Оболочка.
Where: слово владельца; в документах — the shell's screen ([docs/shell.md](docs/shell.md)).

## Окно
Variants: окно main, окно воркера
Means: часть Экрана с сессией: слева main, справа выбранный в Навигации воркер.
Avoid: панель
Where: в документах — pane.

## Шапка
Means: верхняя строка над Окном: путь, ветка и имя сессии.
Where: в документах — the pane's top row.

## Хвост
Means: строка над вводом в Окне, работа комнаты: брифы с галочками, ⧗ ожидаемые отчеты; у claude ее рисует mod, у pi — plugin вместе с уровнем thinking.
Where: в документах — the mod's tail и pi's input rule.

## Статус
Means: строка под Окном: модель, effort, ttl кеша, контекст; у main еще лимиты 5h/7d и ⧗N.
Avoid: статуслиния — так зовется своя строка Claude Code, в rewake выключенная
Where: в документах — the strip.

## Подсказки
Means: нижняя строка Экрана слева: неочевидные клавиши и плашка обновления.
Where: в документах — the hints row.

## Навигация
Means: нижняя строка Экрана справа: вкладки воркеров и [briefs]; без воркеров ее нет.
Where: в документах — the bar.

## Место
Variants: место main, место воркера
Means: Go-процесс, который держит терминал сессии: у main он же запускает Оболочку и Обертку main, у воркера это space host.
Avoid: хост — у владельца так зовется внешняя машина
Where: в коде и документах — main's host (`internal/host/mainhost`) и space host (`internal/host/spacehost`).

## Обертка
Means: Go-процесс одной сессии: почта, доставка, отчеты о ходах, Статус; запускает Агента и держит Go-половину Коннектора.
Where: в документах — the wrapper (`internal/host`).

## Коннектор
Means: связь Обертки с Агентом из двух половин через сокет: Go-адаптер в Обертке и mod внутри claude или plugin (расширение rewake) внутри pi.
Where: `internal/adapter/claude` с модом, `internal/adapter/pi` с расширением; в документах — the adapter.

## Агент
Means: сам claude или pi, которого запускает Обертка.
Where: в документах — the harness.

## Ядро
Means: не отдельный процесс, а Go-код внутри каждой Обертки: почта и доставка, реестр, брифы, отчеты, переключение сборки; общее состояние лежит в Комнате.
Where: `internal/core`.
