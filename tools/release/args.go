package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// call is a parsed command line.
type call struct {
	help    bool
	version string
	publish bool
	otp     string
}

// semver is the grammar of semver.org 2.0.0, the one npm accepts.
var semver = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
	`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// parseArgs reads the command line. An unknown flag, a repeated one and a
// stray word are refusals: a release that silently ignored one would publish
// something other than what was asked.
func parseArgs(args []string) (call, error) {
	var c call
	if len(args) == 0 {
		return call{help: true}, nil
	}
	seen := map[string]bool{}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		flag, joined, hasValue := strings.Cut(argument, "=")
		if strings.HasPrefix(flag, "-") && seen[flag] {
			return c, fmt.Errorf("%s is given twice", flag)
		}
		seen[flag] = true
		switch flag {
		case "--help", "-h":
			return call{help: true}, nil
		case "--publish":
			if hasValue {
				return c, errors.New("--publish takes no value")
			}
			c.publish = true
		case "--otp":
			if !hasValue {
				if index+1 >= len(args) {
					return c, errors.New("--otp needs the code")
				}
				index++
				joined = args[index]
			}
			if joined == "" {
				return c, errors.New("--otp needs the code")
			}
			c.otp = joined
		default:
			if strings.HasPrefix(argument, "-") {
				return c, fmt.Errorf("no flag %s; the flags are --publish, --otp and --help", argument)
			}
			if c.version != "" {
				return c, fmt.Errorf("one version only: %s and %s", c.version, argument)
			}
			c.version = argument
		}
	}
	if c.version == "" {
		return c, errors.New("name the version to release, such as 1.0.0")
	}
	if strings.HasPrefix(c.version, "v") && semver.MatchString(c.version[1:]) {
		return c, fmt.Errorf("the version goes without v: %s", c.version[1:])
	}
	if base, build, ok := strings.Cut(c.version, "+"); ok && semver.MatchString(c.version) {
		// npm strips build metadata when it publishes, so the gate would
		// check and tag one number and the registry would get another.
		return c, fmt.Errorf("npm publishes %s as %s, dropping the build metadata +%s: name the version without it, %s", c.version, base, build, base)
	}
	if !semver.MatchString(c.version) {
		return c, fmt.Errorf("%q is not a semver version such as 1.0.0 or 1.1.0-rc.1", c.version)
	}
	for _, p := range packages {
		// A build's version is a prerelease of its release: named as the
		// release, it would build versions like 1.0.2-linux-x64-linux-x64
		// beside an entry that is itself only a build's number.
		if base, found := strings.CutSuffix(c.version, "-"+p.platform); found && p.platform != "" {
			return c, fmt.Errorf("%s is the version of the %s build of %s, not a release: name the release, %s", c.version, p.platform, base, base)
		}
	}
	if c.otp != "" && !c.publish {
		return c, errors.New("--otp is for --publish: a dry run uploads nothing")
	}
	return c, nil
}
