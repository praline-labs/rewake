package main

import (
	"errors"
	"testing"
)

// Only npm's own E404 frees a version. Everything else stops the release:
// a gate that took every failure for "not there" would publish over a network
// error, and one that matched a phrase took "403 Access denied" for free.
func TestClassify(t *testing.T) {
	for name, tc := range map[string]struct {
		r    result
		want presence
	}{
		"E404 as JSON":          {result{code: 1, stdout: `{"error":{"code":"E404","summary":"No match found for version 1.0.0"}}`}, absent},
		"E404 in the log only":  {result{code: 1, stderr: "npm error code E404\nnpm error 404 Not Found\n"}, absent},
		"the version printed":   {result{stdout: `"1.0.0"`}, present},
		"E403":                  {result{code: 1, stdout: `{"error":{"code":"E403","summary":"403 Forbidden"}}`}, unknown},
		"a network failure":     {result{code: 1, stderr: "npm error code ECONNRESET\nnpm error network aborted\n"}, unknown},
		"no answer in time":     {result{code: -1, err: errors.New("no answer within 1m0s")}, unknown},
		"exit 0 with nothing":   {result{stdout: ""}, unknown},
		"another version":       {result{stdout: `"0.9.0"`}, unknown},
		"a 404 phrase, no code": {result{code: 1, stderr: "npm error 404 Not Found - GET https://e404.example/x\n"}, unknown},
		"E404 inside a line":    {result{code: 1, stderr: "npm error request to https://registry.example/E404 failed\n"}, unknown},
		"unparsable output":     {result{code: 1, stdout: "<html>proxy error</html>"}, unknown},
	} {
		if got, why := classify(tc.r, "1.0.0"); got != tc.want {
			t.Errorf("%s: got %d (%s), want %d", name, got, why, tc.want)
		}
	}
}
