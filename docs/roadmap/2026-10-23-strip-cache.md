# The prompt cache in the worker strip

October 23, 2026, the owner's request; main's brief. **Done** in the shell. The strip's last part is the cache as
`rewake list` has it: the hit percent and the time left (`90% 42m`, `90% <1m`, `90% —`, `90%`), counted from
`cacheLife.expiresAt` on the shell's clock every five seconds. Dropped when the line is too long after the model
and effort and before the branch. `shell/src/room.ts` reads `cache` and `cacheLife`; `strip.test.ts` holds the cell
and the dropping order. **Open.** The live check in the owner's room; the colour of the part is the plain
foreground.
