package core

import (
	"io/fs"
	"os"
	"path/filepath"
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

func walkCandidateFiles(root string, opts *ClocOptions, visit func(string) error) error {
	vcsInRoot := isVCSDir(root)
	var scope rootFilter
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			opts.warn(err)
			return nil
		}
		if opts.filter != nil {
			if path == root {
				scope, err = opts.filter.forRoot(root, entry.IsDir())
				if err != nil {
					return err
				}
			}
			accepted, err := scope.accepts(path, entry)
			if err != nil {
				return err
			}
			if !accepted {
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
			return nil
		}
		return visit(path)
	})
}
