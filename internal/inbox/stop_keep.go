package inbox

import (
	"path/filepath"
	"strings"
)

// keptByStop is what a mailbox's sweeps leave because an open occurrence of
// its stop names it (stop.go): a record whose effect is unknown stays until
// evidence resolves the occurrence, since a sweep that removed it would leave
// the next plan reading "no such file" there, which is no evidence. A stop
// that cannot be listed, or an occurrence whose record does not read, leaves
// what it names unknown, and then the sweeps remove nothing. The caller
// holds the mailbox lock, as every sweep does.
type keptByStop struct {
	paths []string
	all   bool
}

func keptByStopOf(dir, name string) keptByStop {
	open, err := stopState(dir, name)
	if err != nil {
		return keptByStop{all: true}
	}
	var kept keptByStop
	for _, stop := range open {
		if stop.unreadable != nil {
			return keptByStop{all: true}
		}
		for _, rel := range stop.record.Paths {
			kept.paths = append(kept.paths, absolute(dir, rel))
		}
	}
	return kept
}

// keeps says a sweep may not remove path: it is named, lies under a named
// directory, or holds a named path below it.
func (k keptByStop) keeps(path string) bool {
	if k.all {
		return true
	}
	for _, named := range k.paths {
		if named == path || isUnder(named, path) || isUnder(path, named) {
			return true
		}
	}
	return false
}

func isUnder(path, root string) bool {
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

// names says path is named itself or holds a named path below it: what a
// rename onto path would replace. A file moved into a named directory
// replaces nothing named.
func (k keptByStop) names(path string) bool {
	if k.all {
		return true
	}
	for _, named := range k.paths {
		if named == path || isUnder(named, path) {
			return true
		}
	}
	return false
}
