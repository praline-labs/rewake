# A failed pi turn reports pi's error

October 19, 2026. A pi turn that failed reached main as an error with an empty body: pi ends a
failed run with an assistant message whose text is empty and whose `errorMessage` holds the
cause. The extension now sends that message as the turn's text when the text is empty, as
Claude Code's failed turns carry their own. Live on pi 1.2.0 without a login: a task to a pi
session on an unconfigured provider reached main as `error` with `Provider is not configured:
<provider>`, where the build before showed an empty body. Open: nothing.
