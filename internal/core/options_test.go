package core

import (
	"errors"
	"io"
	"reflect"
	"regexp/syntax"
	"runtime"
	"strconv"
	"testing"
)

func TestOptionsPrepareAutomaticWorkers(t *testing.T) {
	languages := NewDefinedLanguages()
	previous := runtime.GOMAXPROCS(0)
	t.Cleanup(func() { runtime.GOMAXPROCS(previous) })
	for _, limit := range []int{1, 3, MaxWorkers + 1} {
		t.Run("GOMAXPROCS="+strconv.Itoa(limit), func(t *testing.T) {
			runtime.GOMAXPROCS(limit)
			for _, opts := range []*Options{nil, {}} {
				prepared, err := opts.prepare(languages)
				if err != nil {
					t.Fatal(err)
				}
				if prepared.Workers != min(limit, MaxWorkers) || !prepared.SkipDuplicated {
					t.Fatalf("prepared=%+v, want automatic workers without deduplication", prepared)
				}
				if opts != nil && opts.Workers != 0 {
					t.Fatal("automatic selection mutated caller options")
				}
			}
		})
	}
}

func TestOptionsPrepareFixedWorkers(t *testing.T) {
	t.Parallel()
	for _, workers := range []int{1, 8, MaxWorkers} {
		t.Run(strconv.Itoa(workers), func(t *testing.T) {
			opts := &Options{Workers: workers}
			prepared, err := opts.prepare(NewDefinedLanguages())
			if err != nil {
				t.Fatal(err)
			}
			if prepared.Workers != workers {
				t.Fatalf("workers=%d, want %d", prepared.Workers, workers)
			}
		})
	}
}

func TestOptionsPrepareInvalidFields(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		opts  Options
		field string
	}{
		{name: "negative workers", opts: Options{Workers: -1}, field: "Workers"},
		{name: "too many workers", opts: Options{Workers: MaxWorkers + 1}, field: "Workers"},
		{name: "match", opts: Options{Match: "["}, field: "Match"},
		{name: "not match", opts: Options{NotMatch: "["}, field: "NotMatch"},
		{name: "match directory", opts: Options{MatchDir: "["}, field: "MatchDir"},
		{name: "not match directory", opts: Options{NotMatchDir: "["}, field: "NotMatchDir"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prepared, err := tc.opts.prepare(NewDefinedLanguages())
			var optionErr *OptionError
			if !errors.As(err, &optionErr) {
				t.Fatalf("error=%v, want OptionError", err)
			}
			if prepared != nil || optionErr.Field != tc.field {
				t.Fatalf(
					"prepared=%+v error=%v, want OptionError for %s",
					prepared,
					err,
					tc.field,
				)
			}
			if tc.field != "Workers" {
				var syntaxErr *syntax.Error
				if !errors.As(err, &syntaxErr) || syntaxErr.Expr != "[" {
					t.Fatalf("error=%v, want wrapped regex syntax error", err)
				}
			}
		})
	}
}

func TestOptionsPrepareFiltersAndOwnership(t *testing.T) {
	t.Parallel()
	opts := &Options{
		Workers: 4, Dedup: true, Debug: true, Fullpath: true, Diagnostics: io.Discard,
		ExcludeExts:  []string{"py", "py", "txt", "unknown"},
		IncludeLangs: []string{"Python", "Go", "Go", "unknown"},
		Exclude:      []string{"{dist,target}/", "*.min.js"},
		Match:        `\.go$`,
		NotMatch:     `_test\.go$`,
		MatchDir:     "src",
		NotMatchDir:  "vendor",
	}
	before := *opts
	before.ExcludeExts = append([]string{}, opts.ExcludeExts...)
	before.IncludeLangs = append([]string{}, opts.IncludeLangs...)
	before.Exclude = append([]string{}, opts.Exclude...)
	prepared, err := opts.prepare(NewDefinedLanguages())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*opts, before) {
		t.Fatal("preparation mutated caller options")
	}
	flagsForwarded := !prepared.SkipDuplicated && prepared.Debug && prepared.Fullpath
	if !flagsForwarded || prepared.Diagnostics != io.Discard {
		t.Fatalf("options not forwarded: %+v", prepared)
	}
	wantExcluded := map[string]struct{}{"Python": {}, "Plain Text": {}, "unknown": {}}
	wantIncluded := map[string]struct{}{"Go": {}, "Python": {}}
	if !reflect.DeepEqual(prepared.ExcludeExts, wantExcluded) {
		t.Fatalf("excluded=%v, want %v", prepared.ExcludeExts, wantExcluded)
	}
	if !reflect.DeepEqual(prepared.IncludeLangs, wantIncluded) {
		t.Fatalf("included=%v, want %v", prepared.IncludeLangs, wantIncluded)
	}
	gotPatterns := []string{
		prepared.ReMatch.String(), prepared.ReNotMatch.String(),
		prepared.ReMatchDir.String(), prepared.ReNotMatchDir.String(),
	}
	wantPatterns := []string{opts.Match, opts.NotMatch, opts.MatchDir, opts.NotMatchDir}
	if !reflect.DeepEqual(gotPatterns, wantPatterns) {
		t.Fatalf("patterns=%q, want %q", gotPatterns, wantPatterns)
	}
	// Each call owns its sets and compiled filters, not caller slices or shared state.
	opts.ExcludeExts[0] = "go"
	opts.IncludeLangs[0] = "JSON"
	opts.Exclude[0] = "different/"
	if !reflect.DeepEqual(prepared.Exclude, before.Exclude) {
		t.Fatal("prepared glob filters alias caller slices")
	}
	if !reflect.DeepEqual(prepared.ExcludeExts, wantExcluded) || !reflect.DeepEqual(prepared.IncludeLangs, wantIncluded) {
		t.Fatal("prepared filters alias caller slices")
	}
}
