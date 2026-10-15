package core

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"strings"
)

func isVCSDir(path string) bool {
	if len(path) > 1 && path[0] == os.PathSeparator {
		path = path[1:]
	}
	vcsDirs := []string{".bzr", ".cvs", ".hg", ".git", ".svn"}
	for _, dir := range vcsDirs {
		if strings.Contains(path, dir) {
			return true
		}
	}
	return false
}

func checkOptionMatch(path string, info interface{ Name() string }, opts *ClocOptions) bool {
	// check match directory & file options
	targetFile := info.Name()
	if opts.Fullpath {
		targetFile = path
	}

	if opts.ReNotMatch != nil && opts.ReNotMatch.MatchString(targetFile) {
		return false
	}
	if opts.ReMatch != nil && !opts.ReMatch.MatchString(targetFile) {
		return false
	}

	dir := filepath.Dir(path)
	if opts.ReNotMatchDir != nil && opts.ReNotMatchDir.MatchString(dir) {
		return false
	}

	if opts.ReMatchDir != nil && !opts.ReMatchDir.MatchString(dir) {
		return false
	}

	return true
}

// A match without end or word-boundary assertions survives appending a child
// path. Other expressions retain per-file filtering to preserve their semantics.
func canPruneDirectory(re *regexp.Regexp) bool {
	if re == nil {
		return false
	}
	expr, err := syntax.Parse(re.String(), syntax.Perl)
	if err != nil {
		return false
	}
	var stable func(*syntax.Regexp) bool
	stable = func(expr *syntax.Regexp) bool {
		switch expr.Op {
		case syntax.OpEndLine, syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
			return false
		}
		for _, sub := range expr.Sub {
			if !stable(sub) {
				return false
			}
		}
		return true
	}
	return stable(expr)
}

func walkCandidateFiles(root string, opts *ClocOptions, visit func(string) error) error {
	vcsInRoot := isVCSDir(root)
	pruneExcluded := canPruneDirectory(opts.ReNotMatchDir)
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			opts.warn(err)
			return nil
		}
		if opts.excluder != nil {
			excluded, err := opts.excluder.excludedEntry(root, path, entry)
			if err != nil {
				return err
			}
			if excluded {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if !vcsInRoot && isVCSDir(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			// Join removes a relative root's ".", so it is not a stable prefix.
			dir := filepath.Clean(path)
			if pruneExcluded && dir != "." && opts.ReNotMatchDir.MatchString(dir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !checkOptionMatch(path, entry, opts) {
			return nil
		}
		return visit(path)
	})
}
