package core

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveWorkerCount(t *testing.T) {
	tests := []struct {
		name string
		opts *ClocOptions
		want int
	}{
		{name: "nil options", want: 1},
		{name: "zero value", opts: &ClocOptions{}, want: 1},
		{name: "negative count", opts: &ClocOptions{Workers: -1}, want: 1},
		{name: "one worker", opts: &ClocOptions{Workers: 1}, want: 1},
		{name: "configured count", opts: &ClocOptions{Workers: 3}, want: 3},
		{name: "maximum count", opts: &ClocOptions{Workers: MaxWorkers}, want: MaxWorkers},
		{name: "capped count", opts: &ClocOptions{Workers: MaxWorkers + 1}, want: MaxWorkers},
		{name: "debug does not change count", opts: &ClocOptions{Workers: 2, Debug: true}, want: 2},
		{
			name: "code callback does not change count",
			opts: &ClocOptions{Workers: 2, OnCode: func(string) {}}, want: 2,
		},
		{
			name: "blank callback does not change count",
			opts: &ClocOptions{Workers: 2, OnBlank: func(string) {}}, want: 2,
		},
		{
			name: "comment callback does not change count",
			opts: &ClocOptions{Workers: 2, OnComment: func(string) {}}, want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveWorkerCount(tt.opts); got != tt.want {
				t.Fatalf("resolveWorkerCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestProcessorAnalyzeEmptyInput(t *testing.T) {
	for _, workers := range []int{0, 1, 2, 8} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			t.Parallel()
			opts := &ClocOptions{Workers: workers}
			result, err := NewProcessor(NewDefinedLanguages(), opts).Analyze(nil)
			if err != nil {
				t.Fatal(err)
			}
			hasFiles := len(result.Files) != 0 || len(result.Languages) != 0
			if result.Total.Total != 0 || hasFiles {
				t.Fatalf("expected empty result: %+v", result)
			}
		})
	}
}

func TestProcessorAnalyzeWorkersProducesEquivalentResults(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	pythonFile := filepath.Join(dir, "script.py")
	if err := os.WriteFile(goFile, []byte("package sample\n// comment\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pythonFile, []byte("# comment\nprint(\"x\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, workers := range []int{0, 1, 2, 8} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			result := analyzeDirectory(t, dir, &ClocOptions{Workers: workers})
			assertAnalysisCounts(t, result.Languages, result.Files)
		})
	}
}

func TestProcessorAnalyzeWorkersPreservesDuplicateBehavior(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("nil options count both files", func(t *testing.T) {
		result := analyzeDirectory(t, dir, nil)
		validTotal := result.Total.Total == 2 && result.Total.Code == 2
		if !validTotal || len(result.Files) != 2 {
			t.Fatalf("nil options should count both files: %+v", result.Total)
		}
	})

	for _, skip := range []bool{false, true} {
		wantPaths := []string{filepath.Join(dir, "a.go")}
		if skip {
			wantPaths = append(wantPaths, filepath.Join(dir, "b.go"))
		}
		for _, workers := range []int{0, 1, 2, 8} {
			t.Run(fmt.Sprintf("workers=%d/skip=%t", workers, skip), func(t *testing.T) {
				result := analyzeDirectory(t, dir, &ClocOptions{Workers: workers, SkipDuplicated: skip})
				wantCount := int32(len(wantPaths))
				if result.Total.Total != wantCount || result.Total.Code != wantCount {
					t.Fatalf("total=%+v, want %d files and code lines", result.Total, wantCount)
				}
				if got := result.Languages["Go"].Files; !reflect.DeepEqual(got, wantPaths) {
					t.Fatalf("retained files=%q, want %q", got, wantPaths)
				}
				if len(result.Files) != len(wantPaths) {
					t.Fatalf("analyzed file count=%d, want %d", len(result.Files), len(wantPaths))
				}
				for _, path := range wantPaths {
					if result.Files[path] == nil || result.Files[path].Code != 1 {
						t.Fatalf("file %q was not counted correctly: %+v", path, result.Files[path])
					}
				}
			})
		}
	}
}

func TestProcessorIndividualCallbacksPreserveDeduplication(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.go")
	duplicate := filepath.Join(dir, "b.go")
	for _, path := range []string{first, duplicate} {
		if err := os.WriteFile(path, []byte("package sample\n// comment\n\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name string
		want string
		set  func(*ClocOptions, func(string))
	}{
		{
			name: "code", want: "package sample",
			set: func(opts *ClocOptions, callback func(string)) { opts.OnCode = callback },
		},
		{
			name: "comment", want: "// comment",
			set: func(opts *ClocOptions, callback func(string)) { opts.OnComment = callback },
		},
		{
			name: "blank", want: "",
			set: func(opts *ClocOptions, callback func(string)) { opts.OnBlank = callback },
		},
		{
			name: "debug with callback", want: "package sample",
			set: func(opts *ClocOptions, callback func(string)) {
				opts.Debug = true
				opts.OnCode = callback
			},
		},
	}
	for _, tt := range tests {
		for _, workers := range []int{1, 8} {
			t.Run(fmt.Sprintf("%s/workers=%d", tt.name, workers), func(t *testing.T) {
				t.Parallel()
				var diagnostics bytes.Buffer
				lines := []string{}
				opts := &ClocOptions{Workers: workers, Diagnostics: &diagnostics}
				tt.set(opts, func(line string) { lines = append(lines, line) })
				result := analyzeDirectory(t, dir, opts)
				if result.Total.Total != 1 || result.Files[duplicate] != nil {
					t.Fatalf("duplicate was retained: %+v", result.Files)
				}
				if !reflect.DeepEqual(lines, []string{tt.want}) {
					t.Fatalf("callbacks=%q, want [%q]", lines, tt.want)
				}
				if !opts.Debug {
					return
				}
				log := diagnostics.String()
				ignoreAt := strings.Index(log, fmt.Sprintf("[SKIP] file=%q reason=\"duplicate content\"\n", duplicate))
				analyzeAt := strings.Index(log, fmt.Sprintf("[FILE] file=%q\n", first))
				invalidOrder := analyzeAt < 0 || ignoreAt <= analyzeAt
				analyzedDuplicate := strings.Contains(log, fmt.Sprintf("[FILE] file=%q\n", duplicate))
				if invalidOrder || !analyzedDuplicate {
					t.Fatalf("debug must log analysis before duplicate exclusion: %s", log)
				}
			})
		}
	}
}

func TestProcessorSingleWorkerCallbacksPreserveOrderAndDeduplication(t *testing.T) {
	dir := t.TempDir()
	first := "package first\n// first\n\n"
	second := "package second\n// second\n\n"
	for name, content := range map[string]string{"a.go": first, "b.go": first, "c.go": second} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, skip := range []bool{false, true} {
		for _, debug := range []bool{false, true} {
			t.Run(fmt.Sprintf("skip=%t/debug=%t", skip, debug), func(t *testing.T) {
				lines := []string{}
				opts := &ClocOptions{
					Workers: 1, SkipDuplicated: skip, Debug: debug, Diagnostics: io.Discard,
					OnCode:    func(line string) { lines = append(lines, "code:"+line) },
					OnComment: func(line string) { lines = append(lines, "comment:"+line) },
					OnBlank:   func(line string) { lines = append(lines, "blank:"+line) },
				}
				result := analyzeDirectory(t, dir, opts)
				want := []string{"code:package first", "comment:// first", "blank:"}
				files := int32(2)
				if skip {
					want = append(want, want...)
					files = 3
				}
				want = append(want, "code:package second", "comment:// second", "blank:")
				if !reflect.DeepEqual(lines, want) || result.Total.Total != files {
					t.Fatalf("callbacks=%q total=%+v, want %q (%d files)", lines, result.Total, want, files)
				}
			})
		}
	}
}

func TestProcessorCallbacksFollowDeduplication(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"a.go":        "package sample\nvar s = \"/*\" // /*\n// sample\n\n",
		"b.py":        "package sample\nvar s = \"/*\" // /*\n// sample\n\n",
		"c.go":        "package sample\nvar s = \"/*\" // /*\n// sample\n\n",
		"d.go":        "package other\n// other\n\n",
		"ignored.xyz": "package other\n// other\n\n",
		"excluded.go": "package excluded\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, skip := range []bool{false, true} {
		for _, debug := range []bool{false, true} {
			for _, workers := range []int{0, 1, 2, 8} {
				name := fmt.Sprintf(
					"skip=%t/debug=%t/workers=%d",
					skip,
					debug,
					workers,
				)
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					var mu sync.Mutex
					calls := make(map[string]int)
					record := func(kind, line string) {
						mu.Lock()
						defer mu.Unlock()
						calls[kind+":"+line]++
					}
					var diagnostics bytes.Buffer
					opts := &ClocOptions{
						Workers: workers, SkipDuplicated: skip,
						Debug: debug, Diagnostics: &diagnostics, NotMatch: "excluded*",
						OnCode:    func(line string) { record("code", line) },
						OnComment: func(line string) { record("comment", line) },
						OnBlank:   func(line string) { record("blank", line) },
					}
					result := analyzeDirectory(t, dir, opts)
					want := map[string]int{
						"code:package sample": 1, "comment:// sample": 1,
						"code:var s = \"/*\" // /*": 1,
						"code:package other":        1, "comment:// other": 1, "blank:": 2,
					}
					wantFiles := int32(2)
					if skip {
						want["code:package sample"] = 3
						want["code:var s = \"/*\" // /*"] = 3
						want["comment:// sample"] = 2
						want["code:// sample"] = 1
						want["blank:"] = 4
						wantFiles = 4
					}
					if !reflect.DeepEqual(calls, want) || result.Total.Total != wantFiles {
						t.Fatalf(
							"calls=%v, files=%d; want %v, %d",
							calls,
							result.Total.Total,
							want,
							wantFiles,
						)
					}
					// Callback mode must not change language selection, retained paths or counts.
					plain := *opts
					plain.OnCode, plain.OnComment, plain.OnBlank = nil, nil, nil
					plain.Debug = false
					withoutCallbacks := analyzeDirectory(t, dir, &plain)
					if !reflect.DeepEqual(result, withoutCallbacks) {
						t.Fatal("callbacks changed the analysis result")
					}
					if opts.diagnostics != nil || opts.Workers != workers {
						t.Fatal("Analyze mutated caller options")
					}
				})
			}
		}
	}
}

func TestProcessorConcurrentCallbacksPreserveLineOrder(t *testing.T) {
	dir := t.TempDir()
	content := "package first\n// first\n\npackage second\n// second"
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, skip := range []bool{false, true} {
		t.Run(fmt.Sprintf("skip=%t", skip), func(t *testing.T) {
			t.Parallel()
			lines := []string{}
			opts := &ClocOptions{
				Workers: 8, SkipDuplicated: skip,
				OnCode:    func(line string) { lines = append(lines, "code:"+line) },
				OnComment: func(line string) { lines = append(lines, "comment:"+line) },
				OnBlank:   func(line string) { lines = append(lines, "blank:"+line) },
			}
			analyzeDirectory(t, dir, opts)
			want := []string{"code:package first", "comment:// first", "blank:", "code:package second", "comment:// second"}
			if !reflect.DeepEqual(lines, want) {
				t.Fatalf("callbacks=%q, want %q", lines, want)
			}
		})
	}
}

func TestProcessorConcurrentCallbacksOverlapAndWait(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"a.go": "package a\n", "b.go": "package b\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, skip := range []bool{false, true} {
		for _, debug := range []bool{false, true} {
			t.Run(fmt.Sprintf("skip=%t/debug=%t", skip, debug), func(t *testing.T) {
				t.Parallel()
				entered := make(chan string, 2)
				release := make(chan struct{})
				unblock := sync.OnceFunc(func() { close(release) })
				var finished atomic.Int32
				opts := &ClocOptions{
					Workers: 2, SkipDuplicated: skip, Debug: debug,
					Diagnostics: io.Discard,
					OnCode: func(line string) {
						entered <- line
						<-release
						finished.Add(1)
					},
				}
				done := make(chan struct{})
				var result *Result
				var err error
				go func() {
					defer close(done)
					result, err = NewProcessor(NewDefinedLanguages(), opts).Analyze([]string{dir})
				}()
				// Always release blocked callbacks, even if the overlap assertion fails.
				t.Cleanup(func() {
					unblock()
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						panic("Analyze did not finish after releasing callbacks")
					}
				})
				seen := make(map[string]bool)
				timeout := time.NewTimer(5 * time.Second)
				defer timeout.Stop()
				for range 2 {
					select {
					case line := <-entered:
						seen[line] = true
					case <-done:
						t.Fatal("Analyze returned while callbacks were blocked")
					case <-timeout.C:
						t.Fatal("callbacks from two files did not overlap")
					}
				}
				if !seen["package a"] || !seen["package b"] {
					t.Fatalf("unexpected callbacks: %v", seen)
				}
				select {
				case <-done:
					t.Fatal("Analyze returned before callbacks finished")
				default:
				}
				unblock()
				select {
				case <-done:
				case <-timeout.C:
					t.Fatal("Analyze did not wait and finish")
				}
				if err != nil || result.Total.Total != 2 || finished.Load() != 2 {
					t.Fatalf(
						"result=%+v error=%v completed callbacks=%d",
						result,
						err,
						finished.Load(),
					)
				}
			})
		}
	}
}

func TestProcessorConcurrentCallbacksEmptyInput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "unknown.xyz"), []byte("unknown\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{name: "empty", paths: []string{}},
		{name: "excluded", paths: []string{dir}},
	} {
		for _, skip := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/skip=%t", tc.name, skip), func(t *testing.T) {
				t.Parallel()
				var calls atomic.Int32
				opts := &ClocOptions{
					Workers: 8, SkipDuplicated: skip,
					OnCode: func(string) { calls.Add(1) },
				}
				result, err := NewProcessor(NewDefinedLanguages(), opts).Analyze(tc.paths)
				if err != nil {
					t.Fatal(err)
				}
				if result.Total.Total != 0 || calls.Load() != 0 {
					t.Fatalf("total=%+v, callbacks=%d", result.Total, calls.Load())
				}
			})
		}
	}
}

func TestProcessorDeduplicatedCallbacksSpillAndCleanup(t *testing.T) {
	dir := t.TempDir()
	spoolDir := t.TempDir()
	setCallbackTempDir(t, spoolDir)
	content := "package sample\n" + strings.Repeat("// comment\n\nvar x = 1\n", 8192)
	for _, name := range []string{"a.go", "b.py", "c.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name        string
		diagnostics io.Writer
		wantErr     bool
	}{
		{name: "normal", diagnostics: io.Discard},
		{name: "diagnostic failure", diagnostics: diagnosticFailureWriter{err: io.ErrClosedPipe}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := []string{}
			var observedSpill bool
			var inspect sync.Once
			record := func(kind, line string) {
				inspect.Do(func() {
					entries, err := os.ReadDir(spoolDir)
					if err != nil {
						t.Error(err)
					}
					observedSpill = len(entries) > 0
				})
				// Only the first file is retained, so its events cannot overlap.
				lines = append(lines, kind+":"+line)
			}
			opts := &ClocOptions{
				Workers: 8, Debug: tc.wantErr, Diagnostics: tc.diagnostics,
				OnCode:    func(line string) { record("code", line) },
				OnComment: func(line string) { record("comment", line) },
				OnBlank:   func(line string) { record("blank", line) },
			}
			result, err := NewProcessor(NewDefinedLanguages(), opts).Analyze([]string{dir})
			if (err != nil) != tc.wantErr {
				t.Fatalf("Analyze error=%v, want error=%t", err, tc.wantErr)
			}
			if !tc.wantErr && (result.Total.Total != 1 || result.Files[filepath.Join(dir, "a.go")] == nil) {
				t.Fatalf("wrong retained file: %+v", result)
			}
			want := []string{"code:package sample"}
			for range 8192 {
				want = append(want, "comment:// comment", "blank:", "code:var x = 1")
			}
			if !reflect.DeepEqual(lines, want) || !observedSpill {
				t.Fatalf("callbacks changed contents/order or failed to spill: lines=%d, spill=%t", len(lines), observedSpill)
			}
			entries, err := os.ReadDir(spoolDir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("spools survived Analyze: %v, %v", entries, err)
			}
		})
	}
}

func TestProcessorCallbackStorageFailureExcludesFileAndContinues(t *testing.T) {
	dir := t.TempDir()
	setCallbackTempDir(t, filepath.Join(t.TempDir(), "missing"))
	big := "package big\n" + strings.Repeat("var x = 1\n", 8192)
	for name, content := range map[string]string{"a.go": big, "b.go": big, "c.go": "package good\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var diagnostics bytes.Buffer
	lines := []string{}
	opts := &ClocOptions{
		Workers: 8, Diagnostics: &diagnostics,
		OnCode: func(line string) { lines = append(lines, line) },
	}
	result := analyzeDirectory(t, dir, opts)
	if result.Total.Total != 1 || !reflect.DeepEqual(lines, []string{"package good"}) {
		t.Fatalf("failed files triggered callbacks: total=%+v, lines=%q", result.Total, lines)
	}
	if strings.Count(diagnostics.String(), "buffer callbacks for") != 2 {
		t.Fatalf("missing storage failure diagnostics: %s", diagnostics.String())
	}
}

func TestProcessorOnlySpoolsWhenDeduplicatingWithCallbacks(t *testing.T) {
	dir := t.TempDir()
	setCallbackTempDir(t, filepath.Join(t.TempDir(), "missing"))
	content := "var value = \"" + strings.Repeat("x", 128<<10) + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "large.go"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		skip      bool
		callbacks bool
		wantFiles int32
	}{
		{name: "plain", skip: true, wantFiles: 1},
		{name: "dedup only", wantFiles: 1},
		{name: "callbacks only", skip: true, callbacks: true, wantFiles: 1},
		{name: "dedup and callbacks need spool", callbacks: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			var diagnostics bytes.Buffer
			opts := &ClocOptions{Workers: 8, SkipDuplicated: tc.skip, Diagnostics: &diagnostics}
			if tc.callbacks {
				opts.OnCode = func(string) { calls.Add(1) }
			}
			result := analyzeDirectory(t, dir, opts)
			if result.Total.Total != tc.wantFiles || (tc.callbacks && calls.Load() != tc.wantFiles) {
				t.Fatalf("files=%d, callbacks=%d, want %d", result.Total.Total, calls.Load(), tc.wantFiles)
			}
			if warned := diagnostics.Len() != 0; warned != (tc.wantFiles == 0) {
				t.Fatalf("unexpected diagnostics: %s", diagnostics.String())
			}
		})
	}
}

func TestProcessorCallbacksBeyondResultWindow(t *testing.T) {
	dir := t.TempDir()
	count := parallelResultWindowSize(4) + 16
	for i := range count {
		for _, ext := range []string{"go", "unknown"} {
			path := filepath.Join(dir, fmt.Sprintf("file%04d.%s", i, ext))
			if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, workers := range []int{1, 4} {
		for _, skip := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers=%d/skip=%t", workers, skip), func(t *testing.T) {
				t.Parallel()
				var calls atomic.Int32
				opts := &ClocOptions{
					Workers: workers, SkipDuplicated: skip,
					OnCode: func(string) { calls.Add(1) },
				}
				result := analyzeDirectory(t, dir, opts)
				want := int32(1)
				if skip {
					want = int32(count)
				}
				if result.Total.Total != want || calls.Load() != want {
					t.Fatalf("files=%d callbacks=%d, want %d", result.Total.Total, calls.Load(), want)
				}
			})
		}
	}
}

func assertAnalysisCounts(t *testing.T, languages map[string]*Language, files map[string]*ClocFile) {
	t.Helper()
	if len(files) != 2 {
		t.Fatalf("analyzed file count = %d, want 2", len(files))
	}
	if got := languages["Go"]; got.Code != 2 || got.Comments != 1 || got.Blanks != 1 {
		t.Fatalf("Go counts = (%d, %d, %d), want (2, 1, 1)", got.Code, got.Comments, got.Blanks)
	}
	if got := languages["Python"]; got.Code != 1 || got.Comments != 1 || got.Blanks != 0 {
		t.Fatalf("Python counts = (%d, %d, %d), want (1, 1, 0)", got.Code, got.Comments, got.Blanks)
	}
}

func analyzeDirectory(t *testing.T, dir string, opts *ClocOptions) *Result {
	t.Helper()
	result, err := NewProcessor(NewDefinedLanguages(), opts).Analyze([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
