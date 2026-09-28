package main

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// localRegistry serves one release to npm from this process, the way the
// public registry will serve it once published: one packument naming every
// version with its dist-tags, and the tarballs npm pack made. Only a registry
// makes npm resolve the entry's npm: aliases and choose a build by os and
// cpu; an install from tarballs on disk cannot, so without one the gate would
// prove the files and never the install.
type localRegistry struct {
	url    string
	server *http.Server
}

// served is one packed upload as the packument lists it.
type served struct {
	manifest map[string]any
	file     string
}

// serveRelease packs every upload of the release into dir and starts a
// registry for them on the loopback interface.
func (g *gate) serveRelease(ctx context.Context, dir string) (*localRegistry, error) {
	var uploads []served
	for _, p := range packages {
		raw, err := os.ReadFile(filepath.Join(g.dist(p), "package.json"))
		if err != nil {
			return nil, err
		}
		var manifest map[string]any
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return nil, fmt.Errorf("%s: package.json does not parse: %w", p.ref(g.version), err)
		}
		r := g.run(ctx, packLimit, g.dist(p), "npm", "pack", "--json", "--ignore-scripts", "--pack-destination", dir)
		if !r.ok() {
			return nil, fmt.Errorf("%s: npm pack failed: %s", p.ref(g.version), r.why())
		}
		var answer []struct {
			Filename string `json:"filename"`
		}
		if err := json.Unmarshal([]byte(r.stdout), &answer); err != nil || len(answer) != 1 || answer[0].Filename == "" {
			return nil, fmt.Errorf("%s: npm pack --json named no single tarball: %v", p.ref(g.version), err)
		}
		uploads = append(uploads, served{manifest: manifest, file: filepath.Join(dir, filepath.Base(answer[0].Filename))})
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	base := "http://" + listener.Addr().String() + "/"
	packument, tarballs, err := g.packument(base, uploads)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// npm asks for a scoped name with its slash escaped, and Go's router
		// would read the escaped form as two segments.
		path, err := url.PathUnescape(r.URL.EscapedPath())
		switch {
		case err != nil:
			http.NotFound(w, r)
		case path == "/"+packageName:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(packument)
		case tarballs[path] != "":
			http.ServeFile(w, r, tarballs[path])
		default:
			http.NotFound(w, r)
		}
	})
	registry := &localRegistry{url: base, server: &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}}
	go func() { _ = registry.server.Serve(listener) }()
	return registry, nil
}

// packument is the registry's document for the package, and the tarball each
// of its paths serves.
func (g *gate) packument(base string, uploads []served) ([]byte, map[string]string, error) {
	versions := map[string]any{}
	tags := map[string]string{}
	tarballs := map[string]string{}
	for index, upload := range uploads {
		p := packages[index]
		content, err := os.ReadFile(upload.file)
		if err != nil {
			return nil, nil, err
		}
		sha := sha1.Sum(content)
		integrity := sha512.Sum512(content)
		path := "/tarballs/" + filepath.Base(upload.file)
		tarballs[path] = upload.file
		manifest := upload.manifest
		manifest["_id"] = p.ref(g.version)
		manifest["dist"] = map[string]string{
			"tarball":   base + path[1:],
			"shasum":    hex.EncodeToString(sha[:]),
			"integrity": "sha512-" + base64.StdEncoding.EncodeToString(integrity[:]),
		}
		versions[p.version(g.version)] = manifest
		tags[p.tag(g.version)] = p.version(g.version)
	}
	document, err := json.Marshal(map[string]any{"name": packageName, "dist-tags": tags, "versions": versions})
	return document, tarballs, err
}

// close stops the registry.
func (r *localRegistry) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		_ = r.server.Close()
	}
}
