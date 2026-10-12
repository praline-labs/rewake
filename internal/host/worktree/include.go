package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// IncludeFile names, at the top of a repository, the ignored files a new
// checkout gets a copy of: a .env, a local configuration. Claude Code reads
// the same file for its own worktrees, and one file serves both.
const IncludeFile = ".worktreeinclude"

// include copies into a new checkout the files of the source that git ignores
// and IncludeFile matches, as Claude Code 2.1.280 does for its worktrees: a
// copy, never a link, of regular files only — a symbolic link is skipped. The
// copy is best effort: a file that cannot be copied is named in Skipped and
// the checkout stands without it.
//
// The listing follows Claude Code's: the ignored entries of the source with
// wholly ignored directories collapsed, so a node_modules no pattern names is
// never walked; a collapsed directory is opened only when a pattern names a
// path in it, or it itself.
func include(record Record) Record {
	text, err := os.ReadFile(filepath.Join(record.Source, IncludeFile))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			record.Skipped = append(record.Skipped, fmt.Sprintf("%s: %v", IncludeFile, err))
		}
		return record
	}
	rules, unread := parseIgnoreRules(string(text))
	record.Skipped = append(record.Skipped, unread...)
	if len(rules) == 0 {
		return record
	}
	entries, err := ignoredEntries(record.Source, "--directory")
	if err != nil {
		record.Skipped = append(record.Skipped, fmt.Sprintf("listing the ignored files of %s: %v", record.Source, err))
		return record
	}
	var files, expand []string
	for _, entry := range entries {
		dir, collapsed := strings.CutSuffix(entry, "/")
		switch {
		case !collapsed:
			if rules.matches(entry, false) {
				files = append(files, entry)
			}
		case rules.opens(dir):
			expand = append(expand, entry)
		}
	}
	if len(expand) > 0 {
		inside, err := ignoredEntries(record.Source, append([]string{"--"}, expand...)...)
		if err != nil {
			record.Skipped = append(record.Skipped, fmt.Sprintf("listing the ignored files of %s: %v", strings.Join(expand, ", "), err))
		}
		for _, entry := range inside {
			if rules.matches(entry, false) {
				files = append(files, entry)
			}
		}
	}
	top, err := filepath.EvalSymlinks(record.Path)
	if err != nil {
		record.Skipped = append(record.Skipped, err.Error())
		return record
	}
	for _, file := range files {
		copied, err := copyIncluded(record.Source, top, file)
		switch {
		case err != nil:
			record.Skipped = append(record.Skipped, fmt.Sprintf("%s: %v", file, err))
		case copied:
			record.Included = append(record.Included, file)
		}
	}
	return record
}

// ignoredEntries lists what git ignores in a checkout, relative to its top.
func ignoredEntries(top string, args ...string) ([]string, error) {
	out, err := gitOutput(top, append([]string{"ls-files", "-z", "--others", "--ignored", "--exclude-standard"}, args...)...)
	if err != nil {
		return nil, err
	}
	var entries []string
	for _, entry := range strings.Split(out, "\x00") {
		if entry != "" {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// copyIncluded copies one regular file from the source into the checkout, and
// says whether it did: a symbolic link, or a file already there, is left.
func copyIncluded(source, top, file string) (bool, error) {
	from := filepath.Join(source, filepath.FromSlash(file))
	info, err := os.Lstat(from)
	if err != nil || !info.Mode().IsRegular() {
		return false, err
	}
	if err := makeParents(top, file); err != nil {
		return false, err
	}
	to := filepath.Join(top, filepath.FromSlash(file))
	in, err := os.Open(from)
	if err != nil {
		return false, err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(to)
		return false, err
	}
	return true, out.Close()
}

// makeParents makes the directories file goes to within the checkout, one
// component at a time, and refuses a component that is not a directory: a link
// the commit holds must not lead the copy, or a directory made for it, out of
// the checkout.
func makeParents(top, file string) error {
	dir := top
	for _, part := range strings.Split(path.Dir(file), "/") {
		if part == "." {
			continue
		}
		dir = filepath.Join(dir, part)
		info, err := os.Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			if err := os.Mkdir(dir, 0o755); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%s in the checkout is not a directory", strings.TrimPrefix(dir, top+string(filepath.Separator)))
		}
	}
	return nil
}

// includedCopy says whether an ignored entry of a checkout, a file or a
// directory with a slash at its end, holds only copies rewake made that still
// equal the source's files: removing those loses nothing.
func includedCopy(record Record, entry string) bool {
	if len(record.Included) == 0 {
		return false
	}
	dir, isDir := strings.CutSuffix(entry, "/")
	if !isDir {
		return sameCopy(record, entry)
	}
	clean := true
	err := filepath.WalkDir(filepath.Join(record.Path, filepath.FromSlash(dir)), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(record.Path, path)
		if err != nil || !sameCopy(record, filepath.ToSlash(relative)) {
			clean = false
			return fs.SkipAll
		}
		return nil
	})
	return err == nil && clean
}

func sameCopy(record Record, file string) bool {
	if !slices.Contains(record.Included, file) {
		return false
	}
	same, err := sameContent(filepath.Join(record.Path, filepath.FromSlash(file)), filepath.Join(record.Source, filepath.FromSlash(file)))
	return err == nil && same
}

// sameContent says whether two regular files hold the same bytes. ls, rm and
// finish ask it of every copy, and a copy can be large, so it compares sizes
// first and then reads both a block at a time rather than whole. Anything but a
// regular file is not a copy rewake made, and opening one — a pipe — could wait
// forever.
func sameContent(one, other string) (bool, error) {
	oneInfo, err := os.Lstat(one)
	if err != nil {
		return false, err
	}
	otherInfo, err := os.Lstat(other)
	if err != nil {
		return false, err
	}
	if !oneInfo.Mode().IsRegular() || !otherInfo.Mode().IsRegular() || oneInfo.Size() != otherInfo.Size() {
		return false, nil
	}
	a, err := os.Open(one)
	if err != nil {
		return false, err
	}
	defer func() { _ = a.Close() }()
	b, err := os.Open(other)
	if err != nil {
		return false, err
	}
	defer func() { _ = b.Close() }()
	aBlock, bBlock := make([]byte, 64<<10), make([]byte, 64<<10)
	for {
		aRead, aErr := io.ReadFull(a, aBlock)
		bRead, bErr := io.ReadFull(b, bBlock)
		if aRead != bRead || !bytes.Equal(aBlock[:aRead], bBlock[:bRead]) {
			return false, nil
		}
		aDone := errors.Is(aErr, io.EOF) || errors.Is(aErr, io.ErrUnexpectedEOF)
		bDone := errors.Is(bErr, io.EOF) || errors.Is(bErr, io.ErrUnexpectedEOF)
		switch {
		case aErr != nil && !aDone:
			return false, aErr
		case bErr != nil && !bDone:
			return false, bErr
		case aDone || bDone:
			return aDone && bDone, nil
		}
	}
}
