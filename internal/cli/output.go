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
func emit(ctx *Context, lines ...string) error {
	for _, line := range lines {
		if _, err := fmt.Fprintln(ctx.Stdout, line); err != nil {
			return failf("could not print the result: %v", err)
		}
	}
	return nil
}

// printValue prints the model as JSON, or the formatted lines.
func printValue(ctx *Context, model any, lines func() []string) error {
	if ctx.JSON {
		encoded, err := json.MarshalIndent(model, "", "  ")
		if err != nil {
			return failf("could not encode the result as JSON: %v", err)
		}
		return emit(ctx, string(encoded))
	}
	return emit(ctx, lines()...)
}

// printText prints a block that has no model of its own, such as the guide.
func printText(ctx *Context, text string) {
	_ = emit(ctx, strings.Split(text, "\n")...)
}
