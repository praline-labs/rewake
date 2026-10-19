# Mail an earlier run never read is shown to the next run

October 19, 2026, from a live case: a main restarted under its name never saw the reports
announced to its earlier run, which ended before reading them — they stayed in `unread/`,
filtered out by the run they were written for. `rewake inbox`, with `--peek` and
`--message`, now reads them with the run's own mail, marked `for an earlier run`
(`earlierRun` in JSON); a task or a question among them records no wait, since its sender
counts it lost. Checked live on pi 1.2.0: a worker's error report to main, then main
restarted, read by the new run. Open: `--owed`'s unread count leaves such tasks out.
