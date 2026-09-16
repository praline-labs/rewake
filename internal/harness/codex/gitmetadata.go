package codex

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Git pointer files are small and stable formats. Reading them avoids requiring
// Git in PATH at launch, and grants only the per-worktree and common metadata
// directories rather than their parent or the other worktrees' checkouts.
func gitMetadataDirectories(cwd string) ([]string, error) {
	entry, info, err := findGitEntry(cwd)
	if err != nil {
		return nil, err
	}
	directory := entry
	if !info.IsDir() {
		pointer, err := readGitPath(entry, "gitdir: ")
		if err != nil {
			return nil, err
		}
		directory = resolveGitPath(filepath.Dir(entry), pointer)
	}
	directory, err = realGitDirectory(directory)
	if err != nil {
		return nil, err
	}
	if err := validateGitMetadata(directory, false); err != nil {
		return nil, err
	}
	directories := []string{directory}
	commonFile := filepath.Join(directory, "commondir")
	if _, err := os.Lstat(commonFile); os.IsNotExist(err) {
		if err := validateGitMetadata(directory, true); err != nil {
			return nil, err
		}
		return directories, nil
	} else if err != nil {
		return nil, fmt.Errorf("cannot inspect %s: %w", commonFile, err)
	}
	commonPath, err := readGitPath(commonFile, "")
	if err != nil {
		return nil, err
	}
	common, err := realGitDirectory(resolveGitPath(directory, commonPath))
	if err != nil {
		return nil, err
	}
	if err := validateGitMetadata(common, true); err != nil {
		return nil, err
	}
	if common != directory {
		directories = append(directories, common)
	}
	return directories, nil
}

func resolveGitPath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	// Preserve components until symlinks have been checked: cleaning link/..
	// first can hide a path that escapes through the link.
	return base + string(os.PathSeparator) + path
}

func realGitDirectory(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve Git metadata directory %s: %w", path, err)
	}
	if resolved != filepath.Clean(path) {
		return "", fmt.Errorf("Git metadata directory %s crosses a symlink", path)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("cannot inspect Git metadata directory %s: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("Git metadata path %s is not a directory", path)
	}
	return resolved, nil
}

// Refuse symlinks and special files before opening, and bound reads so a bad
// pointer cannot make launch read an arbitrary large file into memory.
func readGitPath(path, prefix string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("cannot inspect %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("Git pointer %s is not a regular file (symlinks are not followed)", path)
	}
	const maximum = 64 * 1024
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}
	text := strings.TrimRight(string(raw), "\r\n")
	value, ok := strings.CutPrefix(text, prefix)
	if len(raw) > maximum || !ok || value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("Git pointer %s has an invalid path", path)
	}
	return value, nil
}

// Stop at the nearest .git, including an invalid pointer: falling back to an
// outer repository would grant unrelated metadata to a broken nested checkout.
func findGitEntry(cwd string) (string, os.FileInfo, error) {
	for directory := cwd; ; directory = filepath.Dir(directory) {
		entry := filepath.Join(directory, ".git")
		info, err := os.Lstat(entry)
		if err == nil {
			return entry, info, nil
		}
		if !os.IsNotExist(err) {
			return "", nil, fmt.Errorf("cannot inspect %s: %w", entry, err)
		}
		if filepath.Dir(directory) == directory {
			return "", nil, fmt.Errorf("no repository metadata found above %s", cwd)
		}
	}
}

// Worktree-private metadata has HEAD, but objects and refs live in commondir.
// Merely existing is not enough: a pointer to .. must not grant its whole parent.
func validateGitMetadata(directory string, shared bool) error {
	head, err := os.Lstat(filepath.Join(directory, "HEAD"))
	if err != nil || !head.Mode().IsRegular() {
		return fmt.Errorf("Git metadata directory %s has no regular HEAD file", directory)
	}
	if shared {
		for _, name := range []string{"objects", "refs"} {
			if _, err := realGitDirectory(filepath.Join(directory, name)); err != nil {
				return fmt.Errorf("invalid shared Git metadata: %w", err)
			}
		}
	}
	return nil
}
