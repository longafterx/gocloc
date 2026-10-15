package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/longafterx/gocloc"
)

func TestCommandOutputUsesConfiguredWriter(t *testing.T) {
	dir := outputFixture(t)
	for _, format := range []string{"default", "json", "cloc-xml", "markdown", "sloccount"} {
		for _, byFile := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/by-file=%t", format, byFile), func(t *testing.T) {
				t.Parallel()
				var stdout, stderr bytes.Buffer
				command := newRootCommand()
				command.SetOut(&stdout)
				command.SetErr(&stderr)
				args := []string{"--output-type=" + format, "--sort=name", "--workers=2", dir}
				if byFile {
					args = append(args, "--by-file")
				}
				command.SetArgs(args)
				if err := command.Execute(); err != nil {
					t.Fatal(err)
				}
				if stdout.Len() == 0 || stderr.Len() != 0 {
					t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
				if format == "json" && !json.Valid(stdout.Bytes()) {
					t.Fatalf("invalid JSON: %s", stdout.String())
				}
			})
		}
	}
}

func TestCommandDiagnosticsDoNotCorruptJSON(t *testing.T) {
	dir := outputFixture(t)
	missing := filepath.Join(dir, "missing")
	for _, workers := range []string{"1", "8"} {
		for _, debug := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers=%s/debug=%t", workers, debug), func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				command := newRootCommand()
				command.SetOut(&stdout)
				command.SetErr(&stderr)
				command.SetArgs([]string{
					"--output-type=json", "--workers=" + workers, fmt.Sprintf("--debug=%t", debug), missing, dir,
				})
				if err := command.Execute(); err != nil {
					t.Fatal(err)
				}
				var result gocloc.JSONLanguagesResult
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
					t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
				}
				if result.Total.FilesCount != 1 || result.Total.Code != 2 {
					t.Fatalf("unexpected total: %+v", result.Total)
				}
				if !strings.Contains(stderr.String(), "warning:") || !strings.Contains(stderr.String(), missing) {
					t.Fatalf("missing warning: %s", stderr.String())
				}
				if strings.Contains(stderr.String(), "[FILE] file=") != debug {
					t.Fatalf("debug=%t stderr=%s", debug, stderr.String())
				}
			})
		}
	}
}

func TestCommandDebugPreservesSortedOutput(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"a.go": "package sample\n// comment\n\n",
		"b.go": "package sample\n// comment\n\n",
		"c.py": "# comment\nprint(1)\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, format := range []string{"default", "json", "cloc-xml", "markdown", "sloccount"} {
		for _, byFile := range []bool{false, true} {
			for _, dedup := range []bool{false, true} {
				name := fmt.Sprintf("%s/by-file=%t/dedup=%t", format, byFile, dedup)
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					var withoutDebug string
					for _, debug := range []bool{false, true} {
						var stdout, stderr bytes.Buffer
						command := newRootCommand()
						command.SetOut(&stdout)
						command.SetErr(&stderr)
						command.SetArgs([]string{
							"--workers=8", "--sort=name", "--output-type=" + format,
							fmt.Sprintf("--by-file=%t", byFile), fmt.Sprintf("--dedup=%t", dedup),
							fmt.Sprintf("--debug=%t", debug), dir,
						})
						if err := command.Execute(); err != nil {
							t.Fatal(err)
						}
						if !debug {
							withoutDebug = stdout.String()
							if stderr.Len() != 0 {
								t.Fatalf("unexpected diagnostics: %s", stderr.String())
							}
							continue
						}
						if stdout.String() != withoutDebug {
							t.Fatalf("debug changed sorted output:\n%s\nwant:\n%s", stdout.String(), withoutDebug)
						}
						if !strings.Contains(stderr.String(), "[CODE] file=") {
							t.Fatalf("missing analysis logs: %s", stderr.String())
						}
					}
				})
			}
		}
	}
}

type outputFailureWriter struct {
	err error
}

func (w outputFailureWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	return len(p) / 2, nil
}

func TestCommandPropagatesOutputFailure(t *testing.T) {
	dir := outputFixture(t)
	failure := errors.New("output is closed")
	for _, format := range []string{"default", "json", "cloc-xml", "markdown", "sloccount"} {
		for _, tc := range []struct {
			name   string
			writer io.Writer
			want   error
		}{
			{name: "write error", writer: outputFailureWriter{err: failure}, want: failure},
			{name: "short write", writer: outputFailureWriter{}, want: io.ErrShortWrite},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				command := newRootCommand()
				command.SetOut(tc.writer)
				command.SetErr(io.Discard)
				command.SetArgs([]string{"--by-file", "--output-type=" + format, dir})
				if err := command.Execute(); !errors.Is(err, tc.want) {
					t.Fatalf("error=%v, want %v", err, tc.want)
				}
			})
		}
	}
}

func TestOutputWidthDoesNotLeakBetweenCommands(t *testing.T) {
	dir := outputFixture(t)
	for _, byFile := range []bool{true, false, true, false} {
		var stdout bytes.Buffer
		command := newRootCommand()
		command.SetOut(&stdout)
		command.SetErr(io.Discard)
		command.SetArgs([]string{fmt.Sprintf("--by-file=%t", byFile), dir})
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
		if !byFile {
			line, _, _ := strings.Cut(stdout.String(), "\n")
			if len(line) != 79 {
				t.Fatalf("summary separator width=%d, want 79", len(line))
			}
		}
	}
}

func outputFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte("package main\n// comment\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}
