# The worker's settings as a file, migrated on launch

October 19, 2026, step 4 of [the worker profile](../v2/worker-profile.md), by the owner's
decision of that day. **Done.** A Claude Code worker's `--settings` is the service folder's
`settings.json`, passed as it is: rewake writes its defaults there marked `"rewake":
{"defaults": 1}`; a build with newer defaults adds only the steps after the file's version, and
takes a retired one out only where it still holds rewake's value. `rewake.keep` and the merge
are gone; `claudeMdExcludes` lives in the file. An unreadable file refuses the launch with its
line. Live with haiku: the owner's `{}` gets every default, `Agent` taken out comes back.
**Open.** The review; the owner's folder upgrades on the first worker launch of this build.
