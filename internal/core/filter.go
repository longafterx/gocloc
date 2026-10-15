package core

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type globRule struct {
	pattern  string
	nameOnly bool
	dirOnly  bool
}

type pathExcluder struct {
	rules    []globRule
	hasPaths bool
}

func prepareExcludes(patterns []string) (*pathExcluder, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	filter := &pathExcluder{rules: make([]globRule, 0, len(patterns))}
	for i, pattern := range patterns {
		rule, err := prepareGlobRule(pattern)
		if err != nil {
			return nil, &OptionError{
				Field: "Exclude",
				Err: fmt.Errorf(
					"pattern %d %q: %w",
					i+1,
					pattern,
					err,
				),
			}
		}
		filter.rules = append(filter.rules, rule)
		filter.hasPaths = filter.hasPaths || !rule.nameOnly
	}
	return filter, nil
}

func prepareGlobRule(pattern string) (globRule, error) {
	for strings.HasPrefix(pattern, "./") {
		pattern = strings.TrimPrefix(pattern, "./")
	}
	anchored := strings.HasPrefix(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	dirOnly := strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	if pattern == "" {
		return globRule{}, errors.New("empty glob pattern")
	}
	if !doublestar.ValidatePattern(pattern) {
		return globRule{}, fmt.Errorf("invalid glob syntax: %w", doublestar.ErrBadPattern)
	}
	if strings.HasPrefix(pattern, "/") {
		return globRule{}, errors.New("pattern must be relative to a scan root")
	}
	if len(pattern) >= 2 && pattern[1] == ':' {
		letter := pattern[0]
		upperDrive := letter >= 'A' && letter <= 'Z'
		lowerDrive := letter >= 'a' && letter <= 'z'
		if upperDrive || lowerDrive {
			return globRule{}, errors.New("windows drive paths are not supported; use / relative to a scan root")
		}
	}
	for _, segment := range strings.Split(pattern, "/") {
		if segment == ".." {
			return globRule{}, errors.New("parent directory segments are not supported")
		}
	}
	return globRule{
		pattern:  pattern,
		nameOnly: !anchored && !strings.Contains(pattern, "/"),
		dirOnly:  dirOnly,
	}, nil
}

// An empty relative path checks only name rules, including the scan root itself.
func (filter *pathExcluder) excluded(name, relative string, isDir bool) bool {
	for _, rule := range filter.rules {
		if rule.dirOnly && !isDir {
			continue
		}
		target := name
		if !rule.nameOnly {
			if relative == "" {
				continue
			}
			target = relative
		}
		if doublestar.MatchUnvalidated(rule.pattern, target) {
			return true
		}
	}
	return false
}

func (filter *pathExcluder) excludedEntry(root, path string, entry fs.DirEntry) (bool, error) {
	if path == root {
		if !entry.IsDir() {
			root = filepath.Dir(root)
		}
		absolute, err := filepath.Abs(root)
		if err != nil {
			return false, fmt.Errorf("resolve glob root %q: %w", root, err)
		}
		if filter.excluded(filepath.Base(absolute), "", true) {
			return true, nil
		}
		if entry.IsDir() {
			return false, nil
		}
	}
	relative := ""
	if filter.hasPaths {
		pathFromRoot, err := filepath.Rel(root, path)
		if err != nil {
			return false, fmt.Errorf("make glob path %q relative to %q: %w", path, root, err)
		}
		relative = filepath.ToSlash(pathFromRoot)
	}
	return filter.excluded(entry.Name(), relative, entry.IsDir()), nil
}
