package cli

import (
	"encoding/json"
	"fmt"
	"strings"
)

/*
Every command builds a model and hands it to a formatter. --json prints the
model, the default prints the lines. Going through one function is what makes
"--json works everywhere" true by construction instead of by discipline.
*/

// emit prints result lines to stdout.
func emit(ctx *Context, lines ...string) {
	for _, line := range lines {
		fmt.Fprintln(ctx.Stdout, line)
	}
}

// warn prints to stderr: a side note that does not turn a success into a
// failure, such as a message delivered while something optional did not happen.
func warn(ctx *Context, line string) {
	fmt.Fprintln(ctx.Stderr, line)
}

// printValue prints the model as JSON, or the formatted lines.
func printValue(ctx *Context, model any, lines func() []string) error {
	if ctx.JSON {
		encoded, err := json.MarshalIndent(model, "", "  ")
		if err != nil {
			return failf("could not encode the result as JSON: %v", err)
		}
		emit(ctx, string(encoded))
		return nil
	}
	emit(ctx, lines()...)
	return nil
}

// printText prints a block that has no model of its own, such as the guide.
func printText(ctx *Context, text string) {
	emit(ctx, strings.Split(text, "\n")...)
}
