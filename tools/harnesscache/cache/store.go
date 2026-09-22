package cache

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RootEnv overrides where the cache lives. It is read by the tool and by the
// suite alike, so both see the same versions.
const RootEnv = "REWAKE_HARNESS_CACHE"

// DefaultRoot is the cache directory: RootEnv when set, otherwise
// rewake/harness under the user's cache home ($XDG_CACHE_HOME, or ~/.cache).
func DefaultRoot() (string, error) {
	if root := os.Getenv(RootEnv); root != "" {
		return root, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("no cache home to keep harness versions in; set %s: %w", RootEnv, err)
	}
	return filepath.Join(base, "rewake", "harness"), nil
}

// Entry is one line of the listing: a version, or a directory a fetch left
// unfinished.
type Entry struct {
	Harness string `json:"harness"`
	Version string `json:"version"`
	Dir     string `json:"dir"`
	// Partial is true for a directory an interrupted fetch left behind. It
	// is listed so it can be found and removed; it is never used.
	Partial bool  `json:"partial"`
	Bytes   int64 `json:"bytes"`
}

// List answers what the cache holds, harness by harness, oldest name first.
// The sizes are measured on disk rather than read from the manifests, so an
// unfinished directory has one too.
func (s *Store) List() ([]Entry, error) {
	var entries []Entry
	for _, name := range Names() {
		dirs, err := os.ReadDir(filepath.Join(s.Root, name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, dir := range dirs {
			if !dir.IsDir() {
				continue
			}
			entry := Entry{Harness: name, Version: dir.Name(), Dir: filepath.Join(s.Root, name, dir.Name())}
			if rest, ok := strings.CutPrefix(dir.Name(), partialPrefix); ok {
				entry.Partial = true
				entry.Version = versionOfPartial(rest)
			}
			entry.Bytes = sizeOf(entry.Dir)
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Harness != entries[j].Harness {
			return entries[i].Harness < entries[j].Harness
		}
		return entries[i].Version < entries[j].Version
	})
	return entries, nil
}

// versionOfPartial recovers the version from "<version>-<random>".
func versionOfPartial(rest string) string {
	if cut := strings.LastIndex(rest, "-"); cut > 0 {
		return rest[:cut]
	}
	return rest
}

func sizeOf(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// Remove deletes one version and any unfinished fetch of that same version.
// It answers what it removed, and an error naming the listing when there was
// nothing to remove: a removal that silently did nothing reads as done.
func (s *Store) Remove(h Harness, version string) ([]string, error) {
	if !IsExact(version) {
		return nil, fmt.Errorf("%q is not an exact version; `list` names the cached ones", version)
	}
	entries, err := s.List()
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, entry := range entries {
		if entry.Harness != h.Name || entry.Version != version {
			continue
		}
		if err := os.RemoveAll(entry.Dir); err != nil {
			return removed, err
		}
		removed = append(removed, entry.Dir)
	}
	if len(removed) == 0 {
		return nil, fmt.Errorf("%s %s is not in the cache at %s; `list` names what is", h.Name, version, s.Root)
	}
	return removed, nil
}
