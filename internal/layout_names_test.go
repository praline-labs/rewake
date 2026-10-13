package internal

// nameExceptions held the harness words rule 2 found on its first run over the
// 1.x tree, file by file, each with the step that took it out
// (docs/v2/stage3-packages.md) and the count and sum of the mentions it excused
// (excuse). S19 emptied it. A mention added, removed or replaced fails until an
// entry says so, and an entry with none left fails, so an entry is a decision
// recorded here, never a mention let through.
var nameExceptions = map[nameException]excuse{}
