# Three small shell faults

October 23, 2026. **Done; the owner's look open.**
1. `/logout` ended with `session.end` reason `logout`, which the mod did not tell its wrapper (`exitReasons`), so no closing mark came and the shell waited for main's process; the reason is added. Read from Claude Code 2.1.302's reason list, not seen live: a real `/logout` would sign the owner's account out.
2. The strip's cache part is cyan while warm, dim once expired.
3. The bar under main's pane sits inside the column's padding.
Live under a stand-in terminal with stand-in main and hosts: bar at column 1 like main's text; cache cyan, then default after expiry (the stand-in terminal reads no dim); the mark alone drew the spinner in 11 ms. **Open:** `/logout` itself live.
