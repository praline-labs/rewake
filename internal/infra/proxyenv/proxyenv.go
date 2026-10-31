// Package proxyenv picks the proxy variables out of an environment, for a
// test that starts a harness in an environment built from nothing: this
// machine reaches the network only through its proxy, and the owner's
// decision of October 31, 2026 is that every launch goes through it, a
// test's included. A launch made from the person's environment keeps them
// already (docs/design.md, "Environment the harness receives").
package proxyenv

import "strings"

// names are the variables a harness's network calls read the proxy from,
// each in both cases, as programs read either.
var names = []string{"http_proxy", "https_proxy", "all_proxy", "no_proxy"}

// Of is the entries of env that name the proxy, in env's order.
func Of(env []string) []string {
	var out []string
	for _, entry := range env {
		name, _, found := strings.Cut(entry, "=")
		for _, proxy := range names {
			if found && (name == proxy || name == strings.ToUpper(proxy)) {
				out = append(out, entry)
			}
		}
	}
	return out
}
