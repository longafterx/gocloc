package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
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
	for _, tc := range []struct {
		name string
		opts ClocOptions
	}{
		{name: "exclude", opts: ClocOptions{Exclude: []string{"target/"}}},
		{name: "not match directory", opts: ClocOptions{NotMatchDir: "target/"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diagnostics.Reset()
			tc.opts.Diagnostics = &diagnostics
			opts := tc.opts.withDiagnostics()
			filter, err := preparePathFilter(opts)
			if err != nil {
				t.Fatal(err)
			}
			opts.filter = filter
			if err := walkCandidateFiles(root, opts, func(path string) error {
				t.Errorf("unexpected candidate %q", path)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if diagnostics.Len() != 0 {
				t.Fatalf("excluded directory was visited: %s", diagnostics.String())
			}
		})
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
		opts ClocOptions
		want int
	}{
		{name: "no_filters", want: 512},
		{name: "exclude", opts: ClocOptions{Exclude: []string{"{dist,node_modules,target}/"}}, want: 256},
		{name: "not_match_directory", opts: ClocOptions{NotMatchDir: "{dist,node_modules,target}/"}, want: 256},
	} {
		b.Run(tc.name, func(b *testing.B) {
			opts := &tc.opts
			filter, err := preparePathFilter(opts)
			if err != nil {
				b.Fatal(err)
			}
			opts.filter = filter
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

func TestWalkCandidateFilesDefaultTraversal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{
		"main.go", "target/a.go", "target/sub/b.go", "target-other/c.go",
		".git/config.go", ".hidden/visible.go", "src/keep.go", "src/skip.txt",
	} {
		filename := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		".hidden/visible.go", "main.go", "src/keep.go", "src/skip.txt",
		"target-other/c.go", "target/a.go", "target/sub/b.go",
	}
	if err := os.Symlink(filepath.Join(dir, "src"), filepath.Join(dir, "link")); err == nil {
		// WalkDir visits the symlink but must not follow it into src again.
		want = append(want, "link")
		sort.Strings(want)
	} else {
		t.Logf("symlink unavailable: %v", err)
	}
	for _, tc := range []struct {
		name string
		root string
		want []string
	}{
		{name: "directory", root: dir, want: want},
		{name: "explicit VCS root", root: filepath.Join(dir, ".git"), want: []string{".git/config.go"}},
		{name: "explicit file", root: filepath.Join(dir, "main.go"), want: []string{"main.go"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := []string{}
			if err := walkCandidateFiles(tc.root, NewClocOptions(), func(filename string) error {
				relative, err := filepath.Rel(dir, filename)
				if err != nil {
					return err
				}
				got = append(got, filepath.ToSlash(relative))
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("candidates=%q, want %q", got, tc.want)
			}
		})
	}
}
