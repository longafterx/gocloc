package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
)

func TestWalkGlobPrunesExcludedDirectory(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not prevent ReadDir on Windows")
	}
	root := t.TempDir()
	blocked := filepath.Join(root, "target")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(blocked, 0o700); err != nil {
			t.Error(err)
		}
	})
	if _, err := os.ReadDir(blocked); err == nil {
		t.Skip("current user can read directories without permission")
	}
	var diagnostics bytes.Buffer
	opts := NewClocOptions()
	opts.Diagnostics = &diagnostics
	opts = opts.withDiagnostics()
	if err := walkCandidateFiles(root, opts, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diagnostics.String(), "target") {
		t.Fatal("unfiltered traversal did not attempt to read the blocked directory")
	}
	diagnostics.Reset()
	filter, err := prepareExcludes([]string{"target/"})
	if err != nil {
		t.Fatal(err)
	}
	opts.excluder = filter
	if err := walkCandidateFiles(root, opts, func(path string) error {
		t.Errorf("unexpected candidate %q", path)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("excluded directory was visited: %s", diagnostics.String())
	}
}

func BenchmarkWalkCandidateFiles(b *testing.B) {
	root := b.TempDir()
	for _, dir := range []string{"src", "target", "src/target", "gcc.target"} {
		fullDir := filepath.Join(root, filepath.FromSlash(dir))
		if err := os.MkdirAll(fullDir, 0o700); err != nil {
			b.Fatal(err)
		}
		for i := range 128 {
			name := filepath.Join(fullDir, fmt.Sprintf("file%03d.go", i))
			if err := os.WriteFile(name, []byte("package sample\n"), 0o600); err != nil {
				b.Fatal(err)
			}
		}
	}
	for _, tc := range []struct {
		name string
		glob []string
		re   string
		want int
	}{
		{name: "no_filters", want: 512},
		{name: "glob", glob: []string{"{dist,node_modules,target}/"}, want: 256},
		{name: "regex", re: `(^|[/\\])(dist|node_modules|target)([/\\]|$)`, want: 256},
	} {
		b.Run(tc.name, func(b *testing.B) {
			opts := NewClocOptions()
			filter, err := prepareExcludes(tc.glob)
			if err != nil {
				b.Fatal(err)
			}
			opts.excluder = filter
			if tc.re != "" {
				opts.ReNotMatchDir = regexp.MustCompile(tc.re)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				count := 0
				if err := walkCandidateFiles(root, opts, func(string) error {
					count++
					return nil
				}); err != nil {
					b.Fatal(err)
				}
				if count != tc.want {
					b.Fatalf("candidates=%d, want %d", count, tc.want)
				}
			}
		})
	}
}

func TestWalkCandidateFilesMatchesLegacyWalk(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"main.go", "target/a.go", "target/sub/b.go", "target-other/c.go",
		".git/config.go", ".hidden/visible.go", "src/keep.go", "src/skip.txt",
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "src"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []string{"", "target", "target$", `target\b`, "target|src", "(?m)target$"} {
		for _, root := range []string{dir, filepath.Join(dir, ".git"), filepath.Join(dir, "main.go")} {
			t.Run(pattern+"/"+filepath.Base(root), func(t *testing.T) {
				opts := NewClocOptions()
				if pattern != "" {
					opts.ReNotMatchDir = regexp.MustCompile(pattern)
				}
				want := []string{}
				err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if !checkDefaultIgnore(path, info, isVCSDir(root)) && checkOptionMatch(path, info, opts) {
						want = append(want, path)
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				got := []string{}
				if err := walkCandidateFiles(root, opts, func(path string) error {
					got = append(got, path)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("candidates = %v, legacy = %v", got, want)
				}
			})
		}
	}
}

func TestCanPruneDirectory(t *testing.T) {
	for pattern, want := range map[string]bool{
		"dist|node_modules|target": true,
		"^/src/target":             true,
		"target$":                  false,
		`target\b`:                 false,
		`target\B`:                 false,
		"(?m)target$":              false,
	} {
		if got := canPruneDirectory(regexp.MustCompile(pattern)); got != want {
			t.Errorf("canPruneDirectory(%q) = %v, want %v", pattern, got, want)
		}
	}
}

// Kept only as the legacy Walk oracle; production traversal uses DirEntry.
func checkDefaultIgnore(path string, info os.FileInfo, isVCS bool) bool {
	return info.IsDir() || (!isVCS && isVCSDir(path))
}

func TestCheckDefaultIgnore(t *testing.T) {
	appFS := afero.NewMemMapFs()
	if err := appFS.Mkdir("/test", os.ModeDir); err != nil {
		t.Fatal(err)
	}
	_, _ = appFS.Create("/test/one.go")

	fileInfo, _ := appFS.Stat("/")
	if !checkDefaultIgnore("/", fileInfo, false) {
		t.Errorf("invalid logic: this is directory")
	}

	if !checkDefaultIgnore("/", fileInfo, true) {
		t.Errorf("invalid logic: this is vcs file or directory")
	}

	fileInfo, _ = appFS.Stat("/test/one.go")
	if checkDefaultIgnore("/test/one.go", fileInfo, false) {
		t.Errorf("invalid logic: should not ignore this file")
	}
}

type MockFileInfo struct {
	FileName    string
	IsDirectory bool
}

func (mfi MockFileInfo) Name() string       { return mfi.FileName }
func (mfi MockFileInfo) Size() int64        { return int64(8) }
func (mfi MockFileInfo) Mode() os.FileMode  { return os.ModePerm }
func (mfi MockFileInfo) ModTime() time.Time { return time.Now() }
func (mfi MockFileInfo) IsDir() bool        { return mfi.IsDirectory }
func (mfi MockFileInfo) Sys() interface{}   { return nil }

func TestCheckOptionMatch(t *testing.T) {
	opts := &ClocOptions{}
	fi := MockFileInfo{FileName: "/", IsDirectory: true}
	if !checkOptionMatch("/", fi, opts) {
		t.Errorf("invalid logic: renotmatchdir is nil")
	}

	opts.ReNotMatchDir = regexp.MustCompile("thisisdir-not-match")
	fi = MockFileInfo{FileName: "one.go", IsDirectory: false}
	if !checkOptionMatch("/thisisdir/one.go", fi, opts) {
		t.Errorf("invalid logic: renotmatchdir is nil")
	}

	opts.ReNotMatchDir = regexp.MustCompile("thisisdir")
	fi = MockFileInfo{FileName: "one.go", IsDirectory: false}
	if checkOptionMatch("/thisisdir/one.go", fi, opts) {
		t.Errorf("invalid logic: renotmatchdir is ignore")
	}

	opts = &ClocOptions{}
	opts.ReMatchDir = regexp.MustCompile("thisisdir")
	fi = MockFileInfo{FileName: "one.go", IsDirectory: false}
	if !checkOptionMatch("/thisisdir/one.go", fi, opts) {
		t.Errorf("invalid logic: renotmatchdir is not ignore")
	}

	opts.ReMatchDir = regexp.MustCompile("thisisdir-not-match")
	fi = MockFileInfo{FileName: "one.go", IsDirectory: false}
	if checkOptionMatch("/thisisdir/one.go", fi, opts) {
		t.Errorf("invalid logic: renotmatchdir is ignore")
	}

	opts = &ClocOptions{}
	opts.ReNotMatchDir = regexp.MustCompile("thisisdir-not-match")
	opts.ReMatchDir = regexp.MustCompile("thisisdir")
	fi = MockFileInfo{FileName: "one.go", IsDirectory: false}
	if !checkOptionMatch("/thisisdir/one.go", fi, opts) {
		t.Errorf("invalid logic: renotmatchdir is not ignore")
	}

	t.Run("--match option", func(t *testing.T) {
		opts = &ClocOptions{
			ReMatch: regexp.MustCompile("app.py"),
		}
		fi = MockFileInfo{FileName: "app.py", IsDirectory: false}
		if !checkOptionMatch("test_dir/app.py", fi, opts) {
			t.Errorf("invalid logic: match is not ignore")
		}
	})

	t.Run("--match option with --fullpath option", func(t *testing.T) {
		opts = &ClocOptions{
			ReMatch:  regexp.MustCompile("test_dir/app.py"),
			Fullpath: true,
		}
		fi = MockFileInfo{FileName: "app.py", IsDirectory: false}
		if !checkOptionMatch("test_dir/app.py", fi, opts) {
			t.Errorf("invalid logic: match(with fullpath) is not ignore")
		}
		if checkOptionMatch("app.py", fi, opts) {
			t.Errorf("invalid logic: match(with fullpath) is ignore")
		}
	})

	t.Run("--not-match option with --fullpath option", func(t *testing.T) {
		opts = &ClocOptions{
			ReNotMatch: regexp.MustCompile("test_dir/app.py"),
			Fullpath:   true,
		}
		fi = MockFileInfo{FileName: "app.py", IsDirectory: false}
		if checkOptionMatch("test_dir/app.py", fi, opts) {
			t.Errorf("invalid logic: not-match(with fullpath) is ignore")
		}
		if !checkOptionMatch("app.py", fi, opts) {
			t.Errorf("invalid logic: not-match(with fullpath) is not ignore")
		}
	})
}
