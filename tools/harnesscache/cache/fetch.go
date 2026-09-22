package cache

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store is a cache directory and the registry it fills from.
type Store struct {
	// Root holds one directory per harness, and in each one directory per
	// exact version.
	Root string
	// Registry is the npm registry's base address.
	Registry string
	// HTTP is the client for the registry; nil is the default client, which
	// honors the proxy in the environment.
	HTTP *http.Client
	// Log receives one line per step a person would want to see: whether the
	// network was used, how much came down, how long it took. Nil is silent.
	Log io.Writer
	// DownloadTimeout bounds one tarball; zero is DefaultDownloadTimeout. A
	// caller that runs under a timeout of its own sets a shorter one, so the
	// download fails with its reason instead of the caller being killed.
	DownloadTimeout time.Duration
}

// Version is one harness version present in the cache.
type Version struct {
	Manifest
	// Dir is the version's directory; Executable is the absolute path of
	// the binary inside it.
	Dir        string `json:"dir"`
	Executable string `json:"executablePath"`
	// Downloaded is true when this call fetched it, false when it was
	// already cached.
	Downloaded bool `json:"downloaded"`
}

// Manifest is written beside the unpacked package, last, so its presence is
// what marks a version as complete.
type Manifest struct {
	Harness         string    `json:"harness"`
	Version         string    `json:"version"`
	Package         string    `json:"package"`
	PackageVersion  string    `json:"packageVersion"`
	Tarball         string    `json:"tarball"`
	Integrity       string    `json:"integrity"`
	ExecutableRel   string    `json:"executable"`
	DownloadedBytes int64     `json:"downloadedBytes"`
	UnpackedBytes   int64     `json:"unpackedBytes"`
	FetchedAt       time.Time `json:"fetchedAt"`
}

const manifestName = "manifest.json"

// partialPrefix marks a directory a fetch is still writing, or one that a
// fetch killed halfway left behind. Such a directory is never taken for a
// version: only a rename gives a version its name, and the rename is the last
// step.
const partialPrefix = ".partial-"

// DefaultDownloadTimeout bounds one tarball when the caller sets none. The
// largest seen so far is a little under 150 MB compressed; a transfer taking
// longer than this has stalled.
const DefaultDownloadTimeout = 15 * time.Minute

func (s *Store) downloadTimeout() time.Duration {
	if s.DownloadTimeout > 0 {
		return s.DownloadTimeout
	}
	return DefaultDownloadTimeout
}

// downloadCeiling refuses a tarball larger than any harness has been
// published at, several times over.
const downloadCeiling = 1 << 30

// unpackCeiling refuses a tarball that expands past any size a harness has
// been published at, many times over, so a malformed archive cannot fill the
// disk before its integrity is known.
const unpackCeiling = 4 << 30

// Get answers the version a selector names, fetching it only when it is not
// in the cache yet.
func (s *Store) Get(ctx context.Context, h Harness, selector string) (Version, error) {
	if selector == Latest {
		s.logf("resolving %s latest asks the registry, even when the version it names is cached", h.Name)
	}
	version, err := s.Resolve(ctx, h, selector)
	if err != nil {
		return Version{}, err
	}
	if cached, err := s.Cached(h, version); err == nil {
		s.logf("%s %s is cached at %s; no download", h.Name, version, cached.Dir)
		return cached, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Version{}, err
	}
	return s.fetch(ctx, h, version)
}

// Cached answers a version already in the cache, or an error wrapping
// fs.ErrNotExist when it is not there.
func (s *Store) Cached(h Harness, version string) (Version, error) {
	if !IsExact(version) {
		return Version{}, fmt.Errorf("%q is not an exact version", version)
	}
	dir := filepath.Join(s.Root, h.Name, version)
	raw, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return Version{}, fmt.Errorf("%s %s is not cached: %w", h.Name, version, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Version{}, fmt.Errorf("the manifest of %s is unreadable, remove the version and fetch it again: %w", dir, err)
	}
	executable := filepath.Join(dir, manifest.ExecutableRel)
	if _, err := os.Stat(executable); err != nil {
		return Version{}, fmt.Errorf("%s lost its executable, remove the version and fetch it again: %w", dir, err)
	}
	return Version{Manifest: manifest, Dir: dir, Executable: executable}, nil
}

func (s *Store) fetch(ctx context.Context, h Harness, version string) (Version, error) {
	pkg, pkgVersion, err := h.Build(version)
	if err != nil {
		return Version{}, err
	}
	relative, err := h.Executable()
	if err != nil {
		return Version{}, err
	}
	document, err := s.versionDocument(ctx, h, version, pkg, pkgVersion)
	if err != nil {
		return Version{}, err
	}
	parent := filepath.Join(s.Root, h.Name)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Version{}, fmt.Errorf("the cache directory: %w", err)
	}
	partial, err := os.MkdirTemp(parent, partialPrefix+version+"-")
	if err != nil {
		return Version{}, fmt.Errorf("a directory to download into: %w", err)
	}
	// This call's own unfinished directory, not a cached version: removing
	// it is not the automatic removal the cache never does. On success the
	// tree has already been renamed out of it, and only the download is left.
	defer func() { _ = os.RemoveAll(partial) }()

	started := time.Now()
	s.logf("downloading %s@%s (%.0f MB unpacked) from %s", pkg, pkgVersion, float64(document.Dist.UnpackedSize)/1e6, document.Dist.Tarball)
	downloaded, unpacked, err := s.download(ctx, document, partial)
	if err != nil {
		return Version{}, err
	}
	tree := filepath.Join(partial, unpackedName)
	if _, err := os.Stat(filepath.Join(tree, relative)); err != nil {
		return Version{}, fmt.Errorf("%s@%s has no %s; the package layout has changed and this harness's description needs updating", pkg, pkgVersion, relative)
	}
	manifest := Manifest{
		Harness: h.Name, Version: version, Package: pkg, PackageVersion: pkgVersion,
		Tarball: document.Dist.Tarball, Integrity: document.Dist.Integrity, ExecutableRel: relative,
		DownloadedBytes: downloaded, UnpackedBytes: unpacked, FetchedAt: time.Now().UTC(),
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Version{}, err
	}
	if err := os.WriteFile(filepath.Join(tree, manifestName), encoded, 0o644); err != nil {
		return Version{}, err
	}
	dir := filepath.Join(parent, version)
	if err := os.Rename(tree, dir); err != nil {
		// Another fetch of the same version finished first. Its copy is as
		// good as this one, and it is the one already named.
		if cached, cachedErr := s.Cached(h, version); cachedErr == nil {
			return cached, nil
		}
		return Version{}, fmt.Errorf("naming the downloaded version: %w", err)
	}
	s.logf("%s %s: %.1f MB downloaded, %.1f MB unpacked, in %s, cached at %s",
		h.Name, version, float64(downloaded)/1e6, float64(unpacked)/1e6, time.Since(started).Round(100*time.Millisecond), dir)
	return Version{Manifest: manifest, Dir: dir, Executable: filepath.Join(dir, relative), Downloaded: true}, nil
}

// download writes the whole tarball to a file, compares its digest with the
// registry's integrity, and only then unpacks it. The order is the defense:
// nothing an archive says is acted on before the bytes are known to be the
// ones the registry published. The file goes away once unpacked; the unpacked
// tree is the cache.
func (s *Store) download(ctx context.Context, document published, into string) (downloaded, unpacked int64, err error) {
	digest, want, err := integrity(document.Dist.Integrity)
	if err != nil {
		return 0, 0, err
	}
	archivePath := filepath.Join(into, downloadName)
	downloaded, err = s.fetchTarball(ctx, document.Dist.Tarball, archivePath, digest)
	if err != nil {
		return downloaded, 0, err
	}
	if got := digest.Sum(nil); string(got) != string(want) {
		return downloaded, 0, fmt.Errorf("%s does not match its registry integrity %s; nothing was unpacked or cached", document.Dist.Tarball, document.Dist.Integrity)
	}
	archive, err := os.Open(archivePath)
	if err != nil {
		return downloaded, 0, err
	}
	defer func() { _ = archive.Close() }()
	root := filepath.Join(into, unpackedName)
	// 0700 because this directory becomes the version's: the cache is one
	// user's, and nobody else has a reason to read a harness out of it.
	if err := os.Mkdir(root, 0o700); err != nil {
		return downloaded, 0, err
	}
	unpacked, err = unpack(archive, root)
	if err != nil {
		return downloaded, unpacked, fmt.Errorf("unpacking %s: %w", document.Dist.Tarball, err)
	}
	return downloaded, unpacked, os.Remove(archivePath)
}

// The two names inside a fetch's temporary directory: the downloaded file,
// and the tree it unpacks into. They are siblings so that no name in an
// archive can collide with the file being read.
const (
	downloadName = "download.tgz"
	unpackedName = "tree"
)

// fetchTarball writes the tarball to path through the digest.
func (s *Store) fetchTarball(ctx context.Context, address, path string, digest hash.Hash) (int64, error) {
	ctx, stop := context.WithTimeout(ctx, s.downloadTimeout())
	defer stop()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return 0, err
	}
	response, err := s.client().Do(request)
	if err != nil {
		return 0, fmt.Errorf("downloading %s: %w", address, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("downloading %s: the registry answered %s", address, response.Status)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	written, err := io.Copy(io.MultiWriter(file, digest), io.LimitReader(response.Body, downloadCeiling+1))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return written, fmt.Errorf("downloading %s: %w", address, err)
	}
	if written > downloadCeiling {
		return written, fmt.Errorf("downloading %s: larger than %d bytes, refused", address, int64(downloadCeiling))
	}
	return written, nil
}

// integrity reads an npm subresource-integrity string. The registry publishes
// sha512 for every version this package fetches; anything weaker is refused
// rather than trusted.
func integrity(value string) (hash.Hash, []byte, error) {
	algorithm, encoded, ok := strings.Cut(value, "-")
	if !ok || algorithm != "sha512" {
		return nil, nil, fmt.Errorf("integrity %q is not sha512, and nothing weaker is accepted", value)
	}
	want, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, nil, fmt.Errorf("integrity %q is not base64: %w", value, err)
	}
	return sha512.New(), want, nil
}

// unpack writes a gzipped tarball under dir. It runs only on an archive whose
// digest matched, and it still trusts nothing in it: only directories and
// regular files are accepted — links of either kind, devices and fifos are
// refused, because a link is how a later entry is written somewhere an
// earlier one pointed. None of the packages this cache fetches contains a
// link. The writes go through an os.Root as well, which refuses a path that
// would leave dir by any route.
func unpack(r io.Reader, dir string) (int64, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Close() }()
	compressed, err := gzip.NewReader(r)
	if err != nil {
		return 0, err
	}
	archive := tar.NewReader(compressed)
	var total int64
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
		name, err := inside(header.Name)
		if err != nil {
			return total, err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o755); err != nil {
				return total, err
			}
		case tar.TypeReg:
			if total += header.Size; total > unpackCeiling {
				return total, fmt.Errorf("the archive expands past %d bytes", int64(unpackCeiling))
			}
			if err := writeFile(root, name, archive, header.FileInfo().Mode().Perm()); err != nil {
				return total, err
			}
		default:
			return total, fmt.Errorf("%s is neither a regular file nor a directory (type %q); links, devices and fifos are refused", header.Name, header.Typeflag)
		}
	}
}

// inside is the lexical half of the check: a name that climbs or is absolute
// is refused with a message naming it, before os.Root refuses it anyway.
func inside(name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("the archive names %q, outside the package", name)
	}
	return clean, nil
}

func writeFile(root *os.Root, name string, from io.Reader, perm fs.FileMode) error {
	if dir := filepath.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	// Owner-writable whatever the archive says, so that removing a version
	// never needs a chmod first, and never writable by anyone else whatever
	// the archive or the umask says: 0777 in an archive under umask 000 would
	// otherwise be a world-writable binary that a later run executes.
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm&0o755|0o200)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, from); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (s *Store) logf(format string, args ...any) {
	if s.Log != nil {
		_, _ = fmt.Fprintf(s.Log, format+"\n", args...)
	}
}
