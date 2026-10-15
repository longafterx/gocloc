package gocloc_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/rustyllh/gocloc"
)

func TestAnalyzeDefaultsAndDedup(t *testing.T) {
	t.Parallel()
	dir := writeAPIFixture(t, map[string]string{
		"a.go": "package sample\n// comment\n\n", "b.go": "package sample\n// comment\n\n",
	})
	for _, tc := range []struct {
		name  string
		opts  *gocloc.Options
		files int32
	}{
		{name: "nil", files: 2},
		{name: "zero value", opts: &gocloc.Options{}, files: 2},
		{name: "single worker", opts: &gocloc.Options{Workers: 1}, files: 2},
		{name: "maximum workers", opts: &gocloc.Options{Workers: gocloc.MaxWorkers}, files: 2},
		{name: "dedup single worker", opts: &gocloc.Options{Workers: 1, Dedup: true}, files: 1},
		{name: "dedup automatic", opts: &gocloc.Options{Dedup: true}, files: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := gocloc.Analyze([]string{dir}, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			total := result.Total
			counts := []int32{total.Total, total.Code, total.Comments, total.Blanks}
			if !reflect.DeepEqual(counts, []int32{tc.files, tc.files, tc.files, tc.files}) {
				t.Fatalf("total=%+v, want %d files and one line of each kind per file", total, tc.files)
			}
			if len(result.Files) != int(tc.files) || result.Files[filepath.Join(dir, "a.go")] == nil {
				t.Fatalf("unexpected retained files: %v", result.Files)
			}
			legacy := gocloc.NewClocOptions()
			legacy.Workers = 1
			legacy.SkipDuplicated = tc.files == 2
			want, err := gocloc.NewProcessor(gocloc.NewDefinedLanguages(), legacy).Analyze([]string{dir})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result, want) {
				t.Fatal("new entry point changed counts or retained files")
			}
		})
	}
}

func TestAnalyzeEmptyPaths(t *testing.T) {
	t.Parallel()
	result, err := gocloc.Analyze(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	hasFiles := len(result.Files) != 0 || len(result.Languages) != 0
	if result.Total.Total != 0 || hasFiles {
		t.Fatalf("empty paths must not scan the current directory: %+v", result)
	}
}

func TestAnalyzeGlobExclusions(t *testing.T) {
	t.Parallel()
	dir := writeAPIFixture(t, map[string]string{
		"main.go":                        "package keep\n",
		"dist/generated.go":              "package excluded\n",
		"src/node_modules/lib.go":        "package excluded\n",
		"src/target/nested/generated.go": "package excluded\n",
		"gcc.target/keep.go":             "package keep\n",
		"distribution/keep.go":           "package keep\n",
		"my-dist/keep.go":                "package keep\n",
		"src/skip_test.go":               "package excluded\n",
	})
	want := []string{"distribution/keep.go", "gcc.target/keep.go", "main.go", "my-dist/keep.go"}
	for _, count := range []int{1, 4} {
		t.Run(fmt.Sprintf("workers=%d", count), func(t *testing.T) {
			patterns := []string{"{dist,node_modules,target}/", "*_test.go"}
			var calls atomic.Int32
			opts := &gocloc.Options{
				Workers: count, Exclude: patterns, Match: "*.go",
				OnCode: func(line string) {
					calls.Add(1)
					if line != "package keep" {
						t.Errorf("excluded line reached callback: %q", line)
					}
				},
			}
			result, err := gocloc.Analyze([]string{dir}, opts)
			if err != nil {
				t.Fatal(err)
			}
			if got := apiRelativeFiles(t, dir, result); !reflect.DeepEqual(got, want) {
				t.Fatalf("files=%q, want %q", got, want)
			}
			if calls.Load() != int32(len(want)) || patterns[0] != "{dist,node_modules,target}/" {
				t.Fatal("excluded callbacks ran or caller patterns changed")
			}
			legacy := gocloc.NewClocOptions()
			legacy.Workers = count
			legacy.Exclude = patterns
			legacyResult, err := gocloc.NewProcessor(gocloc.NewDefinedLanguages(), legacy).Analyze([]string{dir})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result, legacyResult) {
				t.Fatal("Analyze and Processor disagree on glob exclusions")
			}
		})
	}
}

func TestAnalyzeGlobRoots(t *testing.T) {
	t.Parallel()
	dir := writeAPIFixture(t, map[string]string{
		"target/generated.go":        "package sample\n",
		"src/target/generated.go":    "package sample\n",
		"src/generated/generated.go": "package sample\n",
		"tests/generated/test.go":    "package sample\n",
		"src/main.go":                "package sample\n",
	})
	for _, tc := range []struct {
		name    string
		paths   []string
		pattern string
		want    []string
	}{
		{
			name: "anchored directory", paths: []string{dir}, pattern: "/target/",
			want: []string{"src/generated/generated.go", "src/main.go", "src/target/generated.go", "tests/generated/test.go"},
		},
		{
			name: "independent roots", paths: []string{filepath.Join(dir, "src"), filepath.Join(dir, "tests")},
			pattern: "/generated/", want: []string{"src/main.go", "src/target/generated.go"},
		},
		{
			name: "root directory name", paths: []string{filepath.Join(dir, "target")},
			pattern: "target/", want: []string{},
		},
		{
			name: "dot root name", paths: []string{filepath.Join(dir, "target") + string(os.PathSeparator) + "."},
			pattern: "target/", want: []string{},
		},
		{
			name: "explicit file parent", paths: []string{filepath.Join(dir, "target/generated.go")},
			pattern: "target/", want: []string{},
		},
		{
			name: "explicit filename", paths: []string{filepath.Join(dir, "src/main.go")},
			pattern: "/main.go", want: []string{},
		},
		{
			name: "outside ancestors ignored", paths: []string{filepath.Join(dir, "src/target")},
			pattern: "src/", want: []string{"src/target/generated.go"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cwd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			relativePaths := make([]string, 0, len(tc.paths))
			inputs := [][]string{tc.paths}
			for _, input := range tc.paths {
				relative, err := filepath.Rel(cwd, input)
				if err != nil {
					// Windows temporary directories may be on another volume.
					t.Logf("relative input unavailable: %v", err)
					break
				}
				relativePaths = append(relativePaths, relative)
			}
			if len(relativePaths) == len(tc.paths) {
				inputs = append(inputs, relativePaths)
			}
			for _, paths := range inputs {
				result, err := gocloc.Analyze(paths, &gocloc.Options{Exclude: []string{tc.pattern}})
				if err != nil {
					t.Fatal(err)
				}
				if got := apiRelativeFiles(t, dir, result); !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("files=%q, want %q", got, tc.want)
				}
			}
		})
	}
}

func TestAnalyzeDirectoryGlobRoots(t *testing.T) {
	t.Parallel()
	dir := writeAPIFixture(t, map[string]string{
		"src/main.go":             "package sample\n",
		"src/lib/deep.go":         "package sample\n",
		"src/target/generated.go": "package sample\n",
		"src/target-old/keep.go":  "package sample\n",
		"tests/main.go":           "package sample\n",
		"tests/lib/deep.go":       "package sample\n",
		"gcc.target/keep.go":      "package sample\n",
		"target/generated.go":     "package sample\n",
	})
	src := []string{"src/lib/deep.go", "src/main.go", "src/target-old/keep.go", "src/target/generated.go"}
	for _, tc := range []struct {
		name  string
		opts  gocloc.Options
		paths []string
		want  []string
	}{
		{name: "include subtree", opts: gocloc.Options{MatchDir: "src/"}, want: src},
		{name: "anchored include subtree", opts: gocloc.Options{MatchDir: "/src/"}, want: src},
		{
			name: "nested directory name", opts: gocloc.Options{MatchDir: "lib"},
			want: []string{"src/lib/deep.go", "tests/lib/deep.go"},
		},
		{
			name: "nested directory path remains reachable", opts: gocloc.Options{MatchDir: "src/lib/"},
			want: []string{"src/lib/deep.go"},
		},
		{
			name: "globstar directories", opts: gocloc.Options{MatchDir: "**/lib/"},
			want: []string{"src/lib/deep.go", "tests/lib/deep.go"},
		},
		{
			name: "directory and file inclusions intersect",
			opts: gocloc.Options{MatchDir: "{src,tests}/", Match: "main.go"},
			want: []string{"src/main.go", "tests/main.go"},
		},
		{
			name: "directory exclusion keeps partial names",
			opts: gocloc.Options{MatchDir: "src/", NotMatchDir: "target/"},
			want: []string{"src/lib/deep.go", "src/main.go", "src/target-old/keep.go"},
		},
		{
			name: "combined exclusions win",
			opts: gocloc.Options{
				MatchDir: "src/", Exclude: []string{"lib/"},
				NotMatchDir: "target/", NotMatch: "keep.go",
			},
			want: []string{"src/main.go"},
		},
		{
			name: "exclude wins over directory inclusion",
			opts: gocloc.Options{MatchDir: "src/", Exclude: []string{"src/"}}, want: []string{},
		},
		{
			name: "input directory name included", opts: gocloc.Options{MatchDir: "src/"},
			paths: []string{filepath.Join(dir, "src")}, want: src,
		},
		{
			name: "anchored rule does not match input directory", opts: gocloc.Options{MatchDir: "/src/"},
			paths: []string{filepath.Join(dir, "src")}, want: []string{},
		},
		{
			name: "input directory name excluded", opts: gocloc.Options{NotMatchDir: "target"},
			paths: []string{filepath.Join(dir, "target")}, want: []string{},
		},
		{
			name: "anchored exclusion per root", opts: gocloc.Options{NotMatchDir: "/target/"},
			paths: []string{filepath.Join(dir, "src")},
			want:  []string{"src/lib/deep.go", "src/main.go", "src/target-old/keep.go"},
		},
		{
			name: "outside include ancestor ignored", opts: gocloc.Options{MatchDir: "src/"},
			paths: []string{filepath.Join(dir, "src/lib")}, want: []string{},
		},
		{
			name: "outside exclude ancestor ignored", opts: gocloc.Options{NotMatchDir: "src/"},
			paths: []string{filepath.Join(dir, "src/lib")}, want: []string{"src/lib/deep.go"},
		},
		{
			name: "explicit file parent included", opts: gocloc.Options{MatchDir: "src/", Match: "/main.go"},
			paths: []string{filepath.Join(dir, "src/main.go")}, want: []string{"src/main.go"},
		},
		{
			name: "explicit file parent excluded", opts: gocloc.Options{NotMatchDir: "target/"},
			paths: []string{filepath.Join(dir, "target/generated.go")}, want: []string{},
		},
		{
			name: "explicit file outside ancestor ignored", opts: gocloc.Options{MatchDir: "src/"},
			paths: []string{filepath.Join(dir, "src/lib/deep.go")}, want: []string{},
		},
		{
			name: "independent file pattern roots", opts: gocloc.Options{Match: "lib/*.go"},
			paths: []string{filepath.Join(dir, "src"), filepath.Join(dir, "tests")},
			want:  []string{"src/lib/deep.go", "tests/lib/deep.go"},
		},
		{
			name: "plain directory pattern does not match substrings", opts: gocloc.Options{MatchDir: "target"},
			want: []string{"src/target/generated.go", "target/generated.go"},
		},
		{
			name: "regex anchors are literal", opts: gocloc.Options{Match: "^main.go$"}, want: []string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := tc.paths
			if paths == nil {
				paths = []string{dir}
			}
			for _, workers := range []int{1, 4} {
				t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
					var calls atomic.Int32
					opts := tc.opts
					opts.Workers = workers
					opts.OnCode = func(string) { calls.Add(1) }
					processorOpts := gocloc.NewClocOptions()
					processorOpts.Workers = workers
					processorOpts.Exclude = opts.Exclude
					processorOpts.Match = opts.Match
					processorOpts.NotMatch = opts.NotMatch
					processorOpts.MatchDir = opts.MatchDir
					processorOpts.NotMatchDir = opts.NotMatchDir
					processorOpts.OnCode = opts.OnCode
					for _, analyze := range []func() (*gocloc.Result, error){
						func() (*gocloc.Result, error) { return gocloc.Analyze(paths, &opts) },
						func() (*gocloc.Result, error) {
							return gocloc.NewProcessor(gocloc.NewDefinedLanguages(), processorOpts).Analyze(paths)
						},
					} {
						calls.Store(0)
						result, err := analyze()
						if err != nil {
							t.Fatal(err)
						}
						if got := apiRelativeFiles(t, dir, result); !reflect.DeepEqual(got, tc.want) {
							t.Fatalf("files=%q, want %q", got, tc.want)
						}
						if calls.Load() != int32(len(tc.want)) {
							t.Fatalf("callbacks=%d, want %d", calls.Load(), len(tc.want))
						}
					}
				})
			}
		})
	}
}

func TestGlobValidationPrecedesScanning(t *testing.T) {
	t.Parallel()
	for _, pattern := range []string{"", "/", "[", "{dist,target", "../target", "src/../target", `C:\target`} {
		t.Run(pattern, func(t *testing.T) {
			for _, emptyPaths := range []bool{false, true} {
				paths := []string{filepath.Join(t.TempDir(), "missing")}
				if emptyPaths {
					paths = []string{}
				}
				var diagnostics bytes.Buffer
				var calls atomic.Int32
				legacy := gocloc.NewClocOptions()
				legacy.Exclude = []string{"valid/", pattern}
				legacy.Diagnostics = &diagnostics
				legacy.OnCode = func(string) { calls.Add(1) }
				analyze := func() (*gocloc.Result, error) {
					return gocloc.Analyze(paths, &gocloc.Options{
						Exclude: legacy.Exclude, Diagnostics: legacy.Diagnostics, OnCode: legacy.OnCode,
					})
				}
				for _, run := range []func() (*gocloc.Result, error){
					analyze,
					func() (*gocloc.Result, error) {
						return gocloc.NewProcessor(gocloc.NewDefinedLanguages(), legacy).Analyze(paths)
					},
				} {
					result, err := run()
					var optionErr *gocloc.OptionError
					if !errors.As(err, &optionErr) || optionErr.Field != "Exclude" {
						t.Fatalf("error=%v, want OptionError for Exclude", err)
					}
					if result != nil || diagnostics.Len() != 0 || calls.Load() != 0 {
						t.Fatal("invalid glob triggered scanning or callbacks")
					}
					if !strings.Contains(err.Error(), "pattern 2") {
						t.Fatalf("error lacks pattern index: %v", err)
					}
				}
			}
		})
	}
}

func apiRelativeFiles(t *testing.T, root string, result *gocloc.Result) []string {
	t.Helper()
	files := make([]string, 0, len(result.Files))
	for path := range result.Files {
		absolute, err := filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(root, absolute)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, filepath.ToSlash(rel))
	}
	sort.Strings(files)
	return files
}

func TestReplacedExtensionsAffectPublicAnalysis(t *testing.T) {
	// Exts is a mutable compatibility variable. This top-level test does not
	// run in parallel because it temporarily replaces the package mapping.
	preexistingLanguages := gocloc.NewDefinedLanguages()
	original := gocloc.Exts
	modified := make(map[string]string, len(original)+1)
	for ext, language := range original {
		modified[ext] = language
	}
	modified["gox"] = "Go"
	gocloc.Exts = modified
	t.Cleanup(func() { gocloc.Exts = original })
	if !strings.Contains(gocloc.NewDefinedLanguages().GetFormattedString(), "gox") {
		t.Fatal("language listing ignored the replaced extension mapping")
	}
	if !strings.Contains(preexistingLanguages.GetFormattedString(), "gox") {
		t.Fatal("existing language listing ignored the replaced extension mapping")
	}

	dir := writeAPIFixture(t, map[string]string{"sample.gox": "package sample\n"})
	for _, tc := range []struct {
		name    string
		analyze func() (*gocloc.Result, error)
	}{
		{name: "Analyze", analyze: func() (*gocloc.Result, error) {
			return gocloc.Analyze([]string{dir}, nil)
		}},
		{name: "Processor", analyze: func() (*gocloc.Result, error) {
			return gocloc.NewProcessor(gocloc.NewDefinedLanguages(), gocloc.NewClocOptions()).Analyze([]string{dir})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyze()
			if err != nil {
				t.Fatal(err)
			}
			if result.Total.Total != 1 || result.Total.Code != 1 || result.Languages["Go"] == nil {
				t.Fatalf("replacement extension mapping was ignored: %+v", result.Total)
			}
		})
	}

	excluded, err := gocloc.Analyze([]string{dir}, &gocloc.Options{ExcludeExts: []string{"gox"}})
	if err != nil {
		t.Fatal(err)
	}
	if excluded.Total.Total != 0 {
		t.Fatalf("custom extension filter was ignored: %+v", excluded.Total)
	}
}

func TestAnalyzeFilters(t *testing.T) {
	t.Parallel()
	dir := writeAPIFixture(t, map[string]string{
		"src/main.go":      "package main\n",
		"src/main_test.go": "package main\n",
		"scripts/main.py":  "print(1)\n",
		"docs/alias.text":  "more plain text\n",
		"scripts/tool.go":  "#!/usr/bin/env python\nprint(3)\n",
		"docs/readme.txt":  "plain text\n",
		"vendor/lib.go":    "package lib\n",
	})
	all := []string{
		"docs/alias.text", "docs/readme.txt", "scripts/main.py", "scripts/tool.go",
		"src/main.go", "src/main_test.go", "vendor/lib.go",
	}
	goFiles := []string{"src/main.go", "src/main_test.go", "vendor/lib.go"}
	for _, tc := range []struct {
		name  string
		opts  gocloc.Options
		paths []string
		want  []string
	}{
		{name: "no filters", opts: gocloc.Options{}, want: all},
		{
			name: "glob combined with language filters",
			opts: gocloc.Options{
				Exclude: []string{"src/"}, Match: "*.go", NotMatch: "lib.go",
				IncludeLangs: []string{"Go", "Python"}, ExcludeExts: []string{"txt"},
			},
			want: []string{"scripts/tool.go"},
		},
		{name: "include language", opts: gocloc.Options{IncludeLangs: []string{"Go"}}, want: goFiles},
		{
			name: "ignore unknown include names",
			opts: gocloc.Options{IncludeLangs: []string{"Go", "unknown", "Go"}}, want: goFiles,
		},
		{name: "only unknown include names", opts: gocloc.Options{IncludeLangs: []string{"unknown"}}, want: all},
		{
			name: "extension maps to detected language including aliases and shebang",
			opts: gocloc.Options{ExcludeExts: []string{"py", "txt"}}, want: goFiles,
		},
		{name: "unknown exclude extension", opts: gocloc.Options{ExcludeExts: []string{"unknown"}}, want: all},
		{
			name: "exclude wins over include",
			opts: gocloc.Options{IncludeLangs: []string{"Go"}, ExcludeExts: []string{"go"}}, want: []string{},
		},
		{
			name: "file glob",
			opts: gocloc.Options{Match: "*.go", NotMatch: "*_test.go"},
			want: []string{"scripts/tool.go", "src/main.go", "vendor/lib.go"},
		},
		{
			name: "directory glob",
			opts: gocloc.Options{MatchDir: "{src,vendor}/", NotMatchDir: "vendor/"},
			want: []string{"src/main.go", "src/main_test.go"},
		},
		{
			name: "relative path glob",
			opts: gocloc.Options{Match: "src/**/*.go", NotMatch: "*_test.go"},
			want: []string{"src/main.go"},
		},
		{
			name: "whole filename without substring matching",
			opts: gocloc.Options{Match: "main"}, want: []string{},
		},
		{
			name: "multiple file and directory roots", opts: gocloc.Options{},
			paths: []string{filepath.Join(dir, "src/main.go"), filepath.Join(dir, "docs")},
			want:  []string{"docs/alias.text", "docs/readme.txt", "src/main.go"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := tc.paths
			if paths == nil {
				paths = []string{dir}
			}
			result, err := gocloc.Analyze(paths, &tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			got := []string{}
			for path := range result.Files {
				relative, err := filepath.Rel(dir, path)
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, filepath.ToSlash(relative))
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("files=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestAnalyzeValidatesBeforeScanning(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		field  string
		opts   gocloc.Options
		syntax bool
	}{
		{name: "workers", field: "Workers", opts: gocloc.Options{Workers: -1}},
		{name: "file include", field: "Match", opts: gocloc.Options{Match: "["}, syntax: true},
		{name: "file exclude", field: "NotMatch", opts: gocloc.Options{NotMatch: "["}, syntax: true},
		{name: "directory include", field: "MatchDir", opts: gocloc.Options{MatchDir: "["}, syntax: true},
		{name: "directory exclude", field: "NotMatchDir", opts: gocloc.Options{NotMatchDir: "["}, syntax: true},
		{name: "file include directory suffix", field: "Match", opts: gocloc.Options{Match: "src/"}},
		{name: "file exclude directory suffix", field: "NotMatch", opts: gocloc.Options{NotMatch: "src/"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diagnostics bytes.Buffer
			var calls atomic.Int32
			opts := tc.opts
			opts.Diagnostics = &diagnostics
			opts.OnCode = func(string) { calls.Add(1) }
			paths := []string{filepath.Join(t.TempDir(), "missing")}
			runners := []struct {
				name string
				run  func() (*gocloc.Result, error)
			}{
				{name: "Analyze", run: func() (*gocloc.Result, error) {
					return gocloc.Analyze(paths, &opts)
				}},
			}
			if tc.field != "Workers" {
				processorOpts := gocloc.NewClocOptions()
				processorOpts.Match = opts.Match
				processorOpts.NotMatch = opts.NotMatch
				processorOpts.MatchDir = opts.MatchDir
				processorOpts.NotMatchDir = opts.NotMatchDir
				processorOpts.Diagnostics = opts.Diagnostics
				processorOpts.OnCode = opts.OnCode
				runners = append(runners, struct {
					name string
					run  func() (*gocloc.Result, error)
				}{name: "Processor", run: func() (*gocloc.Result, error) {
					return gocloc.NewProcessor(gocloc.NewDefinedLanguages(), processorOpts).Analyze(paths)
				}})
			}
			for _, runner := range runners {
				t.Run(runner.name, func(t *testing.T) {
					result, err := runner.run()
					var optionErr *gocloc.OptionError
					if !errors.As(err, &optionErr) {
						t.Fatalf("error=%v, want OptionError", err)
					}
					if result != nil || optionErr.Field != tc.field {
						t.Fatalf(
							"result=%+v error=%v, want OptionError for %s",
							result,
							err,
							tc.field,
						)
					}
					if diagnostics.Len() != 0 || calls.Load() != 0 {
						t.Fatal("invalid options triggered scanning or callbacks")
					}
					if errors.Is(err, doublestar.ErrBadPattern) != tc.syntax {
						t.Fatalf("unexpected wrapped glob syntax error: %v", err)
					}
				})
			}
		})
	}
}

func TestAnalyzeCallbacksFollowDedup(t *testing.T) {
	t.Parallel()
	dir := writeAPIFixture(t, map[string]string{
		"a.go": "package a\n// comment\n\n", "b.go": "package a\n// comment\n\n",
	})
	for _, workers := range []int{0, 1, 4} {
		for _, dedup := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers=%d/dedup=%t", workers, dedup), func(t *testing.T) {
				var code, comment, blank atomic.Int32
				var diagnostics bytes.Buffer
				opts := &gocloc.Options{
					Workers: workers, Dedup: dedup, Debug: true, Diagnostics: &diagnostics,
					OnCode:    func(string) { code.Add(1) },
					OnComment: func(string) { comment.Add(1) },
					OnBlank:   func(string) { blank.Add(1) },
				}
				result, err := gocloc.Analyze([]string{dir}, opts)
				if err != nil {
					t.Fatal(err)
				}
				want := int32(2)
				if dedup {
					want = 1
				}
				counts := []int32{result.Total.Total, code.Load(), comment.Load(), blank.Load()}
				if !reflect.DeepEqual(counts, []int32{want, want, want, want}) {
					t.Fatalf("files and callbacks=%v, want %d each", counts, want)
				}
				if !strings.Contains(diagnostics.String(), "[FILE]") {
					t.Fatal("debug output did not use the supplied writer")
				}
			})
		}
	}
}

func TestAnalyzeAutomaticCallbacksOverlapAndWait(t *testing.T) {
	previous := runtime.GOMAXPROCS(2)
	t.Cleanup(func() { runtime.GOMAXPROCS(previous) })
	dir := writeAPIFixture(t, map[string]string{"a.go": "package a\n", "b.go": "package b\n"})
	for _, dedup := range []bool{false, true} {
		t.Run(fmt.Sprintf("dedup=%t", dedup), func(t *testing.T) {
			entered := make(chan struct{}, 2)
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			var finished atomic.Int32
			opts := &gocloc.Options{
				Dedup: dedup,
				OnCode: func(string) {
					entered <- struct{}{}
					<-release
					finished.Add(1)
				},
			}
			done := make(chan error, 1)
			go func() {
				_, err := gocloc.Analyze([]string{dir}, opts)
				done <- err
				close(done)
			}()
			t.Cleanup(func() {
				unblock()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("Analyze did not finish after releasing callbacks")
				}
			})
			timeout := time.NewTimer(5 * time.Second)
			defer timeout.Stop()
			for range 2 {
				select {
				case <-entered:
				case err := <-done:
					t.Fatalf("Analyze returned with blocked callbacks: %v", err)
				case <-timeout.C:
					t.Fatal("automatic workers did not run callbacks concurrently")
				}
			}
			select {
			case err := <-done:
				t.Fatalf("Analyze returned before callbacks finished: %v", err)
			default:
			}
			unblock()
			select {
			case err := <-done:
				if err != nil || finished.Load() != 2 {
					t.Fatalf("error=%v finished=%d, want both callbacks complete", err, finished.Load())
				}
			case <-timeout.C:
				t.Fatal("Analyze failed to return after callbacks finished")
			}
		})
	}
}

func TestAnalyzeOptionsCanBeReusedConcurrently(t *testing.T) {
	t.Parallel()
	dir := writeAPIFixture(t, map[string]string{"main.go": "package main\n", "main.py": "print(1)\n"})
	opts := &gocloc.Options{
		IncludeLangs: []string{"Go"}, ExcludeExts: []string{"txt"}, Match: "*.go",
	}
	want := *opts
	want.IncludeLangs = append([]string{}, opts.IncludeLangs...)
	want.ExcludeExts = append([]string{}, opts.ExcludeExts...)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := gocloc.Analyze([]string{dir}, opts)
			if err != nil {
				t.Error(err)
				return
			}
			if result.Total.Total != 1 || result.Languages["Go"] == nil {
				t.Errorf("unexpected result: %+v", result)
			}
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(*opts, want) {
		t.Fatalf("caller options mutated: %+v", opts)
	}
}

func TestAnalyzeDiagnosticFailure(t *testing.T) {
	t.Parallel()
	_, err := gocloc.Analyze([]string{filepath.Join(t.TempDir(), "missing")}, &gocloc.Options{
		Diagnostics: failingWriter{},
	})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("error=%v, want diagnostic writer error", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func writeAPIFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
