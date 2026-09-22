package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// DefaultRegistry is the public npm registry. It is settable so the tests can
// serve a registry of their own, and so can a machine behind a mirror.
const DefaultRegistry = "https://registry.npmjs.org"

// metadataTimeout bounds one metadata request. The documents asked for are a
// few kilobytes: a registry that takes longer is not answering.
const metadataTimeout = 30 * time.Second

// published is the part of a registry version document this package reads.
type published struct {
	Version string `json:"version"`
	Dist    struct {
		Tarball      string `json:"tarball"`
		Integrity    string `json:"integrity"`
		UnpackedSize int64  `json:"unpackedSize"`
	} `json:"dist"`
}

// versionDocument asks the registry for one version of one package. The
// per-version document rather than the whole packument: the Codex packument
// lists every alpha for every platform and runs to megabytes, and one version
// is all this ever needs.
//
// asked is the version as the caller named it, for the refusal: a person who
// typed 0.0.1 should read 0.0.1 back, not the platform package it maps to.
func (s *Store) versionDocument(ctx context.Context, h Harness, asked, pkg, version string) (published, error) {
	ctx, stop := context.WithTimeout(ctx, metadataTimeout)
	defer stop()
	address := strings.TrimSuffix(s.Registry, "/") + "/" + url.PathEscape(pkg) + "/" + url.PathEscape(version)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return published{}, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := s.client().Do(request)
	if err != nil {
		return published{}, fmt.Errorf("asking the registry for %s@%s: %w", pkg, version, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		return published{}, fmt.Errorf("the registry has no %s %s; `npm view %s versions` lists the published ones", h.Name, asked, h.Package)
	}
	if response.StatusCode != http.StatusOK {
		return published{}, fmt.Errorf("the registry answered %s for %s@%s", response.Status, pkg, version)
	}
	var document published
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&document); err != nil {
		return published{}, fmt.Errorf("the registry's document for %s@%s is unreadable: %w", pkg, version, err)
	}
	if document.Dist.Tarball == "" || document.Dist.Integrity == "" {
		// Without an integrity there is nothing to verify the download
		// against, and an unverified binary is not one to run.
		return published{}, fmt.Errorf("the registry's document for %s@%s names no tarball or no integrity", pkg, version)
	}
	return document, nil
}

func (s *Store) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return http.DefaultClient
}

// Selector kinds a caller may name besides an exact version.
const (
	Latest    = "latest"
	Installed = "installed"
)

// Resolve turns a selector into an exact version. Only "latest" needs the
// registry; an exact version is answered as it is and "installed" asks the
// local binary.
func (s *Store) Resolve(ctx context.Context, h Harness, selector string) (string, error) {
	switch {
	case IsExact(selector):
		return selector, nil
	case selector == Latest:
		document, err := s.versionDocument(ctx, h, Latest, h.Package, Latest)
		if err != nil {
			return "", err
		}
		if !IsExact(document.Version) {
			return "", fmt.Errorf("the registry's latest %s is %q, which is not a version", h.Package, document.Version)
		}
		return document.Version, nil
	case selector == Installed:
		return installedVersion(ctx, h.Name)
	default:
		return "", fmt.Errorf("%q is not a version selector; name an exact version such as 0.155.1, %s or %s", selector, Latest, Installed)
	}
}

// versionToken finds a version in whatever a harness prints for --version:
// Codex answers "codex-cli 0.155.1", Claude Code "2.1.280 (Claude Code)".
var versionToken = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?`)

// installedVersion asks the binary on PATH which version it is. It is this
// package's one contact with the owner's installation, and it only runs
// --version: nothing is read out of its directory, which is exactly what the
// cache exists to avoid.
func installedVersion(ctx context.Context, name string) (string, error) {
	binary, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("there is no %s on PATH to ask for its version; name an exact version instead", name)
	}
	ctx, stop := context.WithTimeout(ctx, metadataTimeout)
	defer stop()
	out, err := exec.CommandContext(ctx, binary, "--version").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("%s --version failed: %v: %s", binary, err, strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("%s --version: %w", binary, err)
	}
	version := versionToken.FindString(string(out))
	if version == "" {
		return "", fmt.Errorf("%s --version printed no version: %q", binary, strings.TrimSpace(string(out)))
	}
	return version, nil
}
