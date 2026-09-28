package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// registry is where the packages go. It is named on every npm call rather
// than left to the machine's configuration, and for the scope too, since a
// scope's own registry setting outranks --registry: a mirror or a private
// registry configured here would otherwise answer the version check, and
// receive the upload.
const registry = "https://registry.npmjs.org/"

// registryFlags point one npm call at the public registry.
var registryFlags = []string{"--registry", registry, "--" + scope + ":registry=" + registry}

const (
	viewLimit    = time.Minute
	publishLimit = 5 * time.Minute
)

// presence is what the registry says about one package at one version.
type presence int

const (
	// absent: npm answered E404, the one answer that means the version is free.
	absent presence = iota
	// present: npm printed the version back.
	present
	// unknown: anything else — a network failure, a 403, an answer that does
	// not parse. Publishing on it would treat "could not ask" as "free".
	unknown
)

// npmCode finds npm's own error code in its log lines, anchored to the line
// npm writes it on: a registry host or a message that merely contains "E404"
// or "404" is not an answer.
var npmCode = regexp.MustCompile(`(?m)^npm (?:error|ERR!) code (\S+)\s*$`)

// lookup asks the registry whether name@version exists, and says why when
// it cannot tell.
func (g *gate) lookup(ctx context.Context, name, version string) (presence, string) {
	args := append([]string{"view", name + "@" + version, "version", "--json"}, registryFlags...)
	return classify(g.run(ctx, viewLimit, g.root, "npm", args...), version)
}

// classify reads one npm view answer. With --json npm prints the version as
// a JSON string, or an {"error":{"code":...}} object when it fails; the code
// from its log lines is taken when standard output holds nothing to parse.
func classify(r result, version string) (presence, string) {
	if r.err != nil {
		return unknown, r.err.Error()
	}
	out := strings.TrimSpace(r.stdout)
	if r.code == 0 {
		var printed string
		if err := json.Unmarshal([]byte(out), &printed); err != nil || printed == "" {
			return unknown, fmt.Sprintf("npm exited 0 with %q, which names no version", out)
		}
		if printed != version {
			return unknown, fmt.Sprintf("npm answered %s for %s", printed, version)
		}
		return present, ""
	}
	var failure struct {
		Error struct {
			Code    string `json:"code"`
			Summary string `json:"summary"`
		} `json:"error"`
	}
	code, summary := "", ""
	if json.Unmarshal([]byte(out), &failure) == nil {
		code, summary = failure.Error.Code, failure.Error.Summary
	}
	if code == "" {
		if match := npmCode.FindStringSubmatch(r.stderr); match != nil {
			code = match[1]
		}
	}
	if code == "E404" {
		return absent, ""
	}
	if code == "" {
		return unknown, r.why()
	}
	if summary != "" {
		return unknown, code + ": " + summary
	}
	return unknown, code
}

// checkRegistry is the precondition: no upload of the release has its
// version yet, as far as the registry can say. Each version is asked for
// exactly, so a free release does not hide a build already taken.
func (g *gate) checkRegistry(ctx context.Context) {
	for _, p := range packages {
		ref := p.ref(g.version)
		switch answer, why := g.lookup(ctx, packageName, p.version(g.version)); answer {
		case absent:
			g.pass("%s is not in the registry", ref)
		case present:
			g.fail("%s is already in the registry: a version cannot be published twice, release the next one", ref)
		default:
			g.fail("could not tell whether %s is in the registry, so publishing is unsafe to decide: %s", ref, why)
		}
	}
}

// refused is an npm code the registry sends back when it turned an upload
// down: a 4xx status, or a one-time password it asks for. Anything else —
// ECONNRESET, a timeout, a 5xx, no code at all — may have come after the
// registry stored the package, so the upload's outcome is unknown.
var refused = regexp.MustCompile(`^(E4\d\d|EOTP)$`)

// publishAll uploads the packages in order, platforms before the entry that
// names them: an install landing between two uploads must never find an
// entry whose binary is not there yet. It stops at the first upload that did
// not go through, or whose outcome is not known.
func (g *gate) publishAll(ctx context.Context) bool {
	var done []string
	for index, p := range packages {
		ref, tag := p.ref(g.version), p.tag(g.version)
		g.say("publishing %s under %s", ref, tag)
		// --dry-run=false on the command line outranks a dry-run setting
		// inherited from the environment or an npmrc, under which npm
		// skips the upload and still exits 0. The tag is always named: npm
		// would give latest to whatever it publishes without one.
		args := append([]string{"publish", "--dry-run=false", "--access", "public", "--tag", tag, "--ignore-scripts"}, registryFlags...)
		if g.otp != "" {
			args = append(args, "--otp", g.otp)
		}
		r := g.run(ctx, publishLimit, g.dist(p), "npm", args...)
		if r.ok() {
			done = append(done, ref)
			continue
		}
		g.say("")
		g.say("publishing stopped at %s: %s", ref, r.why())
		code := ""
		if match := npmCode.FindStringSubmatch(r.stderr); match != nil {
			code = match[1]
		}
		var unknownRef string
		if r.err != nil || !refused.MatchString(code) {
			// The request may have been stored before its answer was lost:
			// ask once which it was.
			switch answer, why := g.lookup(ctx, packageName, p.version(g.version)); answer {
			case present:
				g.say("the registry has %s all the same: its upload went through, and the next one follows", ref)
				done = append(done, ref)
				continue
			case absent:
				g.say("the registry does not show %s now, which may still be its delay", ref)
				unknownRef = ref
			default:
				g.say("the registry could not be asked about %s: %s", ref, why)
				unknownRef = ref
			}
		}
		var rest []string
		if unknownRef == "" {
			rest = append(rest, ref)
		}
		for _, later := range packages[index+1:] {
			rest = append(rest, later.ref(g.version))
		}
		g.say("published:     %s", listOrNone(done))
		if unknownRef != "" {
			g.say("unknown:       %s", unknownRef)
		}
		g.say("not published: %s", listOrNone(rest))
		switch {
		case len(done) > 0:
			g.say("%s cannot be reused: the registry refuses a version once taken, even after an unpublish, and this gate refuses a release any of whose versions is taken. Release the next version.", g.version)
		case unknownRef != "":
			g.say("do not reuse %s until npm view %s version answers E404 a while from now; if it shows the version, %s was published and %s cannot be reused.", g.version, unknownRef, unknownRef, g.version)
		default:
			g.say("nothing was published and %s is still free: fix the cause and run the gate again.", g.version)
		}
		return false
	}
	return true
}

// verify asks the registry for what was just published, once, without
// retrying: right after an upload the registry may answer 404 for a while. It
// reports whether every version was seen and every dist-tag points at its
// upload; one that does not may be delay, or an upload npm skipped while
// saying it made it, and only a later look tells.
func (g *gate) verify(ctx context.Context) bool {
	g.say("")
	g.say("in the registry now:")
	var unseen []string
	for _, p := range packages {
		ref := p.ref(g.version)
		switch answer, why := g.lookup(ctx, packageName, p.version(g.version)); answer {
		case present:
			g.say("  %s visible", ref)
		case absent:
			g.say("  %s not visible yet", ref)
			unseen = append(unseen, ref)
		default:
			g.say("  %s could not be asked: %s", ref, why)
			unseen = append(unseen, ref)
		}
	}
	args := append([]string{"view", packageName, "dist-tags", "--json"}, registryFlags...)
	r := g.run(ctx, viewLimit, g.root, "npm", args...)
	var tags map[string]string
	read := r.ok() && json.Unmarshal([]byte(strings.TrimSpace(r.stdout)), &tags) == nil
	var untagged []string
	for _, p := range packages {
		tag, want := p.tag(g.version), p.version(g.version)
		switch {
		case !read:
			g.say("  the tag %s could not be read yet: npm view %s dist-tags", tag, packageName)
		case tags[tag] == want:
			g.say("  %s is %s", tag, want)
			continue
		case tags[tag] == "":
			g.say("  there is no tag %s yet; if it stays so: npm dist-tag add %s %s", tag, p.ref(g.version), tag)
		default:
			g.say("  %s is %s, not %s; if it stays so: npm dist-tag add %s %s", tag, tags[tag], want, p.ref(g.version), tag)
		}
		untagged = append(untagged, tag)
	}
	if len(unseen) > 0 {
		g.say("")
		g.say("npm said every upload went through, but the registry does not show %s yet. That is usually its delay; it can also be an upload npm skipped. Ask again in a few minutes, and tag only once each is there:", strings.Join(unseen, ", "))
		for _, ref := range unseen {
			g.say("  npm view %s version", ref)
		}
	}
	if len(untagged) > 0 {
		g.say("")
		g.say("the dist-tags %s do not point at this release yet; usually the registry's delay again. Tag only once npm view %s dist-tags shows them.", strings.Join(untagged, ", "), packageName)
	}
	return len(unseen) == 0 && len(untagged) == 0
}

func listOrNone(refs []string) string {
	if len(refs) == 0 {
		return "none"
	}
	return strings.Join(refs, ", ")
}
