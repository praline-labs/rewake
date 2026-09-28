package main

import (
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want call
	}{
		{nil, call{help: true}},
		{[]string{"--help"}, call{help: true}},
		{[]string{"1.0.0", "-h"}, call{help: true}},
		{[]string{"1.0.0"}, call{version: "1.0.0"}},
		{[]string{"1.1.0-rc.1"}, call{version: "1.1.0-rc.1"}},
		{[]string{"--publish", "1.0.0"}, call{version: "1.0.0", publish: true}},
		{[]string{"1.0.0", "--publish", "--otp", "123456"}, call{version: "1.0.0", publish: true, otp: "123456"}},
		{[]string{"1.0.0", "--publish", "--otp=123456"}, call{version: "1.0.0", publish: true, otp: "123456"}},
	} {
		got, err := parseArgs(tc.args)
		if err != nil || got != tc.want {
			t.Errorf("%v: got %+v %v, want %+v", tc.args, got, err, tc.want)
		}
	}
}

// Every refusal names the way on, and none of these reaches a build: a
// release that ignored a word would publish something other than asked.
func TestParseArgsRefusals(t *testing.T) {
	for _, tc := range []struct {
		args []string
		says string
	}{
		{[]string{"--publish"}, "name the version"},
		{[]string{"v1.0.0"}, "without v: 1.0.0"},
		{[]string{"1.0"}, "not a semver version"},
		{[]string{"01.0.0"}, "not a semver version"},
		{[]string{"1.0.0+build.1"}, "without it, 1.0.0"},
		{[]string{"1.0.0-rc.1+sha.abc"}, "without it, 1.0.0-rc.1"},
		{[]string{"1.0.0", "1.0.1"}, "one version only"},
		{[]string{"1.0.2-linux-x64"}, "the linux-x64 build of 1.0.2, not a release: name the release, 1.0.2"},
		{[]string{"1.1.0-rc.1-linux-arm64"}, "name the release, 1.1.0-rc.1"},
		{[]string{"1.0.0", "--dry-run"}, "no flag --dry-run"},
		{[]string{"1.0.0", "--otp", "1"}, "--otp is for --publish"},
		{[]string{"1.0.0", "--publish", "--otp"}, "--otp needs the code"},
		{[]string{"1.0.0", "--publish", "--publish"}, "given twice"},
		{[]string{"1.0.0", "--publish=yes"}, "takes no value"},
	} {
		if _, err := parseArgs(tc.args); err == nil || !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%v: got %v, want %q", tc.args, err, tc.says)
		}
	}
}
