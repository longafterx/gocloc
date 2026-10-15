//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/rustyllh/gocloc"
)

func TestCLIIntegration(t *testing.T) {
	binary := integrationBinary(t)
	dir := t.TempDir()
	for name, content := range map[string]string{
		"src/main.go":         "package sample\n// comment\n\nfunc main() {}\n",
		"src/copy.go":         "package sample\n// comment\n\nfunc main() {}\n",
		"scripts/run.py":      "#!/usr/bin/env python\n# comment\n# another\n\n\nprint(1)\nprint(2)\nprint(3)\n",
		"target/generated.go": "package generated\n",
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("statistics", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			args  []string
			files int32
			code  int32
		}{
			{name: "default", args: []string{"."}, files: 4, code: 9},
			{name: "deduplication", args: []string{"--dedup", "."}, files: 3, code: 7},
			{name: "explicit no deduplication", args: []string{"--dedup=false", "."}, files: 4, code: 9},
			{name: "legacy no deduplication", args: []string{"--skip-duplicated", "."}, files: 4, code: 9},
			{name: "legacy deduplication", args: []string{"--skip-duplicated=false", "."}, files: 3, code: 7},
			{name: "one worker", args: []string{"--workers=1", "."}, files: 4, code: 9},
			{name: "multiple paths", args: []string{"src", "scripts"}, files: 3, code: 8},
			{name: "interspersed", args: []string{"src", "--workers", "2", "scripts"}, files: 3, code: 8},
			{name: "exclude directory", args: []string{"--not-match-d=target", "."}, files: 3, code: 8},
			{name: "include directory", args: []string{"--match-d=scripts", "."}, files: 1, code: 4},
			{name: "include filename", args: []string{"--match=*.py", "."}, files: 1, code: 4},
			{name: "exclude filename", args: []string{"--not-match=*.py", "."}, files: 3, code: 5},
			{name: "relative path glob", args: []string{"--match=scripts/**/*.py", "."}, files: 1, code: 4},
			{name: "single level path", args: []string{"--match=src/*.go", "."}, files: 2, code: 4},
			{name: "brace alternatives", args: []string{"--match=*.{go,py}", "."}, files: 4, code: 9},
			{name: "directory braces", args: []string{"--match-d={src,scripts}/", "."}, files: 3, code: 8},
			{name: "anchored directory", args: []string{"--match-d=/src/", "."}, files: 2, code: 4},
			{name: "include input directory", args: []string{"--match-d=src/", "src"}, files: 2, code: 4},
			{
				name: "include explicit file parent",
				args: []string{"--match-d=scripts/", "--match=/run.py", "scripts/run.py"}, files: 1, code: 4,
			},
			{
				name: "exclude explicit file parent",
				args: []string{"--not-match-d=target/", "target/generated.go"},
			},
			{name: "directory whole name", args: []string{"--not-match-d=arget", "."}, files: 4, code: 9},
			{name: "file whole name", args: []string{"--match=run", "."}},
			{name: "regex no longer interpreted", args: []string{"--match=^run.py$", "."}},
			{name: "exclude extension", args: []string{"--exclude-ext=go,txt", "."}, files: 1, code: 4},
			{name: "include languages", args: []string{"--include-lang=Python,JSON", "."}, files: 1, code: 4},
		} {
			t.Run(tc.name, func(t *testing.T) {
				args := append([]string{"--output-type=json"}, tc.args...)
				stdout, stderr, err := runCLI(binary, dir, args)
				if err != nil || stderr != "" {
					t.Fatalf("run CLI: %v, stderr: %s", err, stderr)
				}
				var result gocloc.JSONLanguagesResult
				if err := json.Unmarshal([]byte(stdout), &result); err != nil {
					t.Fatal(err)
				}
				if result.Total.FilesCount != tc.files || result.Total.Code != tc.code {
					t.Fatalf("total = %+v, want %d files, %d code", result.Total, tc.files, tc.code)
				}
			})
		}
	})

	t.Run("directory exclusion example", func(t *testing.T) {
		root := t.TempDir()
		for _, name := range []string{
			"main.go", "src/keep.go", "src/dist-other/keep.go", "src/build-target/keep.go",
			"dist/generated.go", "dist/sub/nested.go", "src/node_modules/dependency.go",
			"src/target/sub/generated.go",
			"gcc.target/keep.go", "distribution/keep.go", "src/skip_test.go",
		} {
			path := filepath.Join(root, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		for _, tc := range []struct {
			name string
			args []string
		}{
			{
				name: "glob braces and repeated flags",
				args: []string{"--exclude={dist,node_modules,target}/", "--exclude=*_test.go"},
			},
			{
				name: "directory and filename glob flags",
				args: []string{
					"--not-match-d={dist,node_modules,target}/",
					"--not-match=*_test.go",
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				args := append([]string{"-f", "-o", "json", "."}, tc.args...)
				stdout, stderr, err := runCLI(binary, root, args)
				if err != nil || stderr != "" {
					t.Fatalf("run CLI: %v, stderr: %s", err, stderr)
				}
				var result gocloc.JSONFilesResult
				if err := json.Unmarshal([]byte(stdout), &result); err != nil {
					t.Fatal(err)
				}
				got := make([]string, 0, len(result.Files))
				for _, file := range result.Files {
					got = append(got, filepath.ToSlash(file.Name))
				}
				sort.Strings(got)
				want := []string{
					"distribution/keep.go", "gcc.target/keep.go", "main.go",
					"src/build-target/keep.go", "src/dist-other/keep.go", "src/keep.go",
				}
				if strings.Join(got, "\n") != strings.Join(want, "\n") {
					t.Fatalf("counted files = %v, want %v", got, want)
				}
			})
		}
	})

	t.Run("short and long options are equivalent", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			long  []string
			short []string
		}{
			{
				name:  "file output and sorting",
				long:  []string{"--workers=8", "--by-file", "--sort=name", "--output-type=json", "."},
				short: []string{"-w", "8", "-f", "-s", "name", "-o", "json", "."},
			},
			{
				name:  "attached values and combined flags",
				long:  []string{"--workers=8", "--by-file", "--sort=name", "--output-type=json", "."},
				short: []string{"-w08", "-fsname", "-ojson", "."},
			},
			{
				name:  "include and exclude",
				long:  []string{"--include-lang=Go,Python", "--exclude-ext=go,txt", "--output-type=json", "."},
				short: []string{"-l", "Go,Python", "-e", "go,txt", "-o=json", "."},
			},
			{
				name:  "mixed forms and paths",
				long:  []string{"--workers=2", "--sort=name", "--output-type=json", "src", "scripts"},
				short: []string{"src", "-w2", "--sort=name", "-ojson", "scripts"},
			},
			{name: "version", long: []string{"--version"}, short: []string{"-V"}},
			{name: "languages", long: []string{"--show-lang"}, short: []string{"-L"}},
			{name: "help", long: []string{"--help"}, short: []string{"-h"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				longOut, longErrOut, longErr := runCLI(binary, dir, tc.long)
				shortOut, shortErrOut, shortErr := runCLI(binary, dir, tc.short)
				if longErr != nil || shortErr != nil {
					t.Fatalf("long: %v %s; short: %v %s", longErr, longErrOut, shortErr, shortErrOut)
				}
				if tc.name == "languages" {
					longOut = normalizeLanguageExtensions(longOut)
					shortOut = normalizeLanguageExtensions(shortOut)
				}
				if shortOut != longOut || shortErrOut != longErrOut {
					t.Fatalf("long and short outputs differ:\n%s\n%s", longOut, shortOut)
				}
			})
		}
	})

	t.Run("errors", func(t *testing.T) {
		for _, args := range [][]string{
			{"--unknown"}, {"--workers"}, {"--workers=0"}, {"--workers=-1", "--version"},
			{"--sort=unknown", "."}, {"--by-file", "--sort=files", "."}, {"--match=[", "."},
			{"-w0", "-V"}, {"-w65", "-L"}, {"-sunknown", "."}, {"-f", "-sfiles", "."},
			{"--dedup", "--skip-duplicated", "."}, {"--dedup=invalid", "."},
			{"--exclude=", "."}, {"--exclude=[", "."}, {"--fullpath", "."},
			{"--match=src/", "."}, {"--not-match=src/", "."},
			{"--not-match=[", "."}, {"--match-d=[", "."}, {"--not-match-d=[", "."},
		} {
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				stdout, stderr, err := runCLI(binary, dir, args)
				if err == nil || stdout != "" || strings.TrimSpace(stderr) == "" {
					t.Fatalf("err = %v, stdout = %q, stderr = %q", err, stdout, stderr)
				}
				if strings.Contains(stderr, "panic:") || strings.Contains(stderr, "Usage:") {
					t.Fatalf("unexpected panic or usage: %s", stderr)
				}
				if strings.Count(strings.TrimSpace(stderr), "\n") != 0 {
					t.Fatalf("expected one error line, got %q", stderr)
				}
			})
		}
	})

	t.Run("paths resembling commands", func(t *testing.T) {
		for _, name := range []string{"help", "completion", "--version"} {
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, name)
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "main.go"), []byte("package main\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				stdout, stderr, err := runCLI(binary, root, []string{"--output-type=json", "--", name})
				if err != nil || stderr != "" {
					t.Fatalf("run CLI: %v, stderr: %s", err, stderr)
				}
				var result gocloc.JSONLanguagesResult
				if err := json.Unmarshal([]byte(stdout), &result); err != nil {
					t.Fatal(err)
				}
				if result.Total.FilesCount != 1 {
					t.Fatalf("expected one file, got %+v", result.Total)
				}
			})
		}
	})

	t.Run("output compatibility", func(t *testing.T) {
		// Optional comparison against a CLI built before the migration.
		baseline := os.Getenv("GOCLOC_CLI_BASELINE")
		cases := []struct {
			name string
			args []string
		}{
			{name: "defaults", args: []string{"."}},
			{name: "all languages", args: []string{"--show-lang"}},
			{name: "debug", args: []string{"--debug", "src/copy.go"}},
			// These filtered totals tie on code lines, so use a deterministic order
			// for byte-for-byte comparisons. Default tie order is unspecified.
			{
				name: "filters",
				args: []string{"--not-match-d=target", "--include-lang=Go,Python", "--sort=name", "."},
			},
			{name: "no deduplication", args: []string{"--skip-duplicated", "--sort=name", "."}},
			{name: "deduplication", args: []string{"--skip-duplicated=false", "--sort=name", "."}},
			{name: "serial", args: []string{"--workers=1", "."}},
			{name: "parallel", args: []string{"--workers=8", "."}},
			{name: "decimal leading zero", args: []string{"--workers=08", "."}},
			{name: "interspersed", args: []string{"src", "--workers", "2", "scripts", "--sort=name"}},
			{name: "exclude extensions", args: []string{"--exclude-ext=py,txt", "."}},
			{name: "include directory", args: []string{"--match-d=src", "."}},
			{name: "repeated option", args: []string{"--include-lang=Go", "--include-lang=Python", "."}},
		}
		for _, format := range []string{"default", "json", "cloc-xml", "markdown", "sloccount"} {
			for _, byFile := range []bool{false, true} {
				args := []string{"--sort=name", "--output-type=" + format, "."}
				name := format
				if byFile {
					args = append([]string{"--by-file"}, args...)
					name += " by file"
				}
				cases = append(cases, struct {
					name string
					args []string
				}{name: name, args: args})
			}
		}
		for _, sort := range []string{"name", "files", "blank", "comment", "code"} {
			cases = append(cases, struct {
				name string
				args []string
			}{name: "sort " + sort, args: []string{"--sort=" + sort, "."}})
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				after, newStderr, newErr := runCLI(binary, dir, tc.args)
				if newErr != nil || after == "" {
					t.Fatalf("run CLI: %v, stdout: %q, stderr: %q", newErr, after, newStderr)
				}
				if tc.name == "debug" {
					hasDebug := strings.Contains(newStderr, "[FILE] file=")
					if !hasDebug || strings.Contains(after, "[FILE] file=") {
						t.Fatalf("debug logs must be on stderr: stdout=%q stderr=%q", after, newStderr)
					}
				} else if newStderr != "" {
					t.Fatalf("unexpected diagnostics: %s", newStderr)
				}
				// Debug record format and cross-file order intentionally differ
				// from older releases. Statistical parity is tested separately.
				if baseline == "" || tc.name == "debug" {
					return
				}
				// Older CLIs enabled deduplication by default. Align the counting
				// mode while allowing an explicit legacy flag below to override it.
				baselineArgs := append([]string{"--skip-duplicated=true"}, tc.args...)
				before, oldStderr, oldErr := runCLI(baseline, dir, baselineArgs)
				if oldErr != nil || newErr != nil {
					t.Fatalf("before: %v %s; after: %v %s", oldErr, oldStderr, newErr, newStderr)
				}
				if tc.name == "all languages" {
					// Extension order comes from a map and varies even between legacy runs.
					before = normalizeLanguageExtensions(before)
					after = normalizeLanguageExtensions(after)
				}
				if before != after || oldStderr != newStderr {
					t.Fatalf("output changed\nbefore:\n%s\n%s\nafter:\n%s\n%s", before, oldStderr, after, newStderr)
				}
			})
		}
	})
}

func normalizeLanguageExtensions(output string) string {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		name, extensions, ok := strings.Cut(line, "(")
		if !ok {
			continue
		}
		items := strings.Split(strings.TrimSuffix(extensions, ")"), ", ")
		sort.Strings(items)
		lines[i] = name + "(" + strings.Join(items, ", ") + ")"
	}
	return strings.Join(lines, "\n")
}

func runCLI(binary, dir string, args []string) (string, string, error) {
	command := exec.Command(binary, args...)
	command.Dir = dir
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}
