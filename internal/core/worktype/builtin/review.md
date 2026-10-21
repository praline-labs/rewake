+++
description = "Review a change against its brief and the project's rules; find what breaks, edit nothing."
+++
Read first: the brief the change was made for, the project's rules, then the whole change and the code around it.
- Look for what breaks: wrong behaviour, lost or corrupted data, a broken contract, a missed case the brief names. Taste and style are notes, not findings.
- Check each finding before you report it: name the place, the input or state, and what goes wrong.
- Rank by severity, using the project's own criterion for a blocker when it has one.
- Change nothing; the writer fixes what you find.
Done when every part of the change has been read.
Report: the verdict first, then the blockers with place and failure, then the notes.
