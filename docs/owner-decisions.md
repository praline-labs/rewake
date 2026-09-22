# Owner decisions that no single diff can recover

Moved into the project on September 22, 2026 from a handoff set that was deleted with
the rest of the archives. The quotations were not edited, typos included: an edited
quotation stops being a quotation. They are kept in the owner's own language on
purpose, each followed by a short English gloss in parentheses; the gloss is a reading
aid, the quotation is the record. What these decisions say lives in the code and in
the project's rules; their authorship and exact wording exist only here.

The verbatim phrases are taken from the conversation; dates inside the September 17–20
range were not recovered more precisely unless stated. These are the decisions in
force, not every intermediate variant. Older, incompatible decisions survive only in
history.

## The transition and the next tasks — September 20

> Теперь пиши handoff
> я думаю переходить в claude code для оркестрации, в codex лимиты недельные скоро закончатся

(Write the handoff now; I am thinking of moving orchestration to Claude Code, because
the weekly Codex limits are about to run out.)

> нужно будет составить карту фичей, что умеет codex, и что нужно "дотянуть" для claude code (отдельным файлом нужно, с таблицей, чтобы можно было потом докидывать harness и видеть, что осталось)

(A feature map of what Codex can do and what Claude Code still has to catch up on —
a separate file with a table, so that harnesses can be added later and what remains
stays visible.)

> и нужно сделать автоматизацию тестирования, чтобы не забивать логами контекст агентов, но какие-то правильные тесты для нас, чтобы можно было тестировать сам процесс фичей (это будет исследоваться)

(And test automation, so that logs do not fill the agents' context — proper tests for
us, able to test the process of a feature itself; to be researched.)

The map and the plan were prepared as permanent documents. The parity/runner
implementation is not done yet. The owner gave no new figure for available tokens and
no time limit.

## Orchestration and process

> если агенты отвалятся, не пытайся их оживить, я приду и помогу

(If the agents drop off, do not try to revive them; I will come and help.)

> а зачем ты смотришь за ним? пусть он работает, а ты жди

(Why are you watching it? Let it work, and you wait.)

> это понятно, просто как сама задача поставлена, в этом все и дело
> мы же программу пишем rewake, чтобы налажить связь между агентами в харнесс

(That is understood; the point is how the task itself is stated — we are writing
rewake to make agents in harnesses talk to each other.)

> ты очень конечно строг в формулировках))) не переусложняй)

(You are very strict in your wording; do not overcomplicate.)

> а вот если бы я ребутнул его, то такое задание ему бы было не понятным
> почему не прекрепил исследование?

(Had I rebooted it, such a task would have made no sense to it; why did you not
attach the research?)

> Ревьювер теперь будет он
> подготовь ему сообщение понятное, чем он будет заниматься, пусть прочитает и получит необходимый контекст

(It is the reviewer now; prepare it a clear message on what it will be doing, so it
reads it and gets the context it needs.)

The last one is about general-claude, not the already closed general-codex.
Delegation through rewake, a self-sufficient brief with exact paths, wait for the
final result, then an independent review. Do not watch the writer's intermediate
code. Do not answer a stop with an automatic resend.

> пусть write делает коммит если все ок и пушит

(Let write commit and push if everything is fine.)

That permission was used after review, checks and acceptance. Not permitted: force
push, npm publication, an unexpected move of state, or automatic restarts of windows.

## Roles and names

> убери эту логику, и чтобы теперь явный --main задавал роль оркестратора

(Remove that logic; from now on an explicit --main sets the orchestrator role.)

> Да, единое правило для всех ролей

(Yes, one rule for all roles.)

The last one confirms names of the form role-harness or explicit-prefix-harness with
an automatic collision number. Do not bring back first-comer-main or auto-main.

## Delivery, batches and stopping

> тут важно подчеркнуть
> пачка доставляется между ходами, при любой возможности. Если агент встал в idle, пачка его будит

(Important to stress: a batch is delivered between turns, at every opportunity; if
the agent has gone idle, the batch wakes it.)

> rewake будет да, но только в том случае, если агент в idle, в остальное время все приходит между вызовами тулзов

(rewake does wake, but only when the agent is idle; the rest of the time everything
arrives between tool calls.)

> Если есть что-то в rewake недоставленное, он закидывет как steer

(If rewake holds anything undelivered, it throws it in as a steer.)

> rewake должен будить, в этом весь смысл

(rewake must wake; that is the whole point.)

These final words cancelled the intermediate "collect until peek" and the wait for a
whole native turn/completed. Adopted: a short collection of the accumulated batch,
start-or-steer on readiness, and a new immutable batch for later messages. This is
not a promise to merge every message received during work into one row.

> Будить только новыми сообщениями

(Wake only with new messages.)

This is the answer about Ctrl+C: a stopped old task is not resent, new letters may
wake. Do not restore the previous work on a single timeout/error.

## Side, permissions and telemetry

> То есть, главное, чтобы связь rewake не пропадала в этот момент с сессией основной

(The main thing is that rewake's link to the primary session is not lost at that
moment.)

/side is temporary; the link goes to primary, not to the displayed side. After side
is closed the user returns to the main work rather than restarting it.

> если задание предполагает git, то оркестратор решает сам выдать или нет, rewake автоматом не выдает и не решает выдать.
> обычные уведомления и отчеты не про это, они только про общение между агентами

(If a task involves git, the orchestrator itself decides whether to grant it; rewake
neither grants automatically nor decides to. Ordinary notifications and reports are
not about that; they are only about agents talking to each other.)

> Да, видит только оркестратор

(Yes, only the orchestrator sees it.)

The last one is about the collected telemetry of all roles. Ordinary workers are not
shown it, not even in JSON. A future main from another room should have that
possibility, but cross-room functionality is not implemented now. The heading is a
name like write-codex, not agent status and not a repeated rewake:. Main must see
context, window, compactions, model/effort, activity and connect/disconnect.

## Native delivery and UI

> так мне не в интерфейс нужно
> а в сообщения, чтобы не через чат доставлять, а максимально нативным способом

(I do not need it in the interface but in the messages: not delivered through the
chat, but in the most native way possible.)

A model toolOutput and a UI-only event were made separately: the picture does not
stand in for delivery.

> необязательно Rewake, можно и другой нативный, просто rewake это по желанию
> хук регистировать не хотелось бы

(Not necessarily Rewake, another native one will do, rewake is optional; I would
rather not register a hook.)

> а в чем проблема именно "породить" евент, точто также как и остальные?
> Например есть вызовы Ran, Explored

(What is the problem with simply "spawning" an event, the same way as the others?
There are the Ran and Explored calls, for instance.)

This turned main's mistakenly narrow search through ready-made RPCs into downstream
UI adaptation. The owner saw the label Ran rewake notice --display-only, accepted it
and confirmed "вижу" ("I see it") on the installed version. The command in the label
is not executed; the literal heading Ran selects the native renderer, so hooks and
dummy children are not needed.

> да и это нормально для решения. Когда-нибудь позже подумаем как сохранить, но сейсчас и так отлично

(Yes, and that is fine as a solution. Some day we will think about how to keep it,
but for now it is good as it is.)

Verbatim consent of September 20: the UI row is transient, keeping it across
transitions is a future task. Mailbox/read/report stay durable independently of the
row.

## Review and acceptance — September 21

> он должен будет проверять после claude, как финальный приемщик

(It will have to check after claude, as the final acceptor.)

About the session on Codex. Hence the chain: write writes, review-claude reviews
independently, write fixes, review-codex accepts, and only then the commit.

> кодекс дорабатывался сильнее и проверялся больше

> клауд делался первым, потом кодекс

> клауд делался легче, кодекс потребовал титанических усилий в поиске решения

> поэтому в клауд может что-то не так работать, но кодекс сломать нельзя

(Codex was worked on harder and checked more; Claude was made first, then Codex;
Claude came easier, Codex took titanic effort to find a solution; so something in
Claude may work wrong, but Codex must not be broken.)

This is the ground of the whole asymmetry: the acceptor sits on the Codex side, a
regression in the Codex path is the worst that can happen, and in the Claude Code
path defects are more likely. A paraphrase of this argument is in `check-runner.md`
and `check-runner-proposal.md`; verbatim it is only here.

## Pinning harness versions — September 21

> качать и проверять новые версии, чтобы не тестить на мне вживую

(Download and check new versions, so as not to test on me live.)

The aim of the work-queue task: take an arbitrary version of Codex into a disposable
environment, take the protocol schema from it and run our messages against it — to
learn about a breakage before the owner updates their installation.
