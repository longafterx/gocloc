package core

import (
	"fmt"
	"io"
	"regexp"
	"runtime"
)

// Options configures Analyze. Its zero value uses built-in languages, automatic
// concurrency and no deduplication, just like a nil *Options.
// Analyze does not modify Options or its slices. Do not modify them during a call.
type Options struct {
	// Workers bounds concurrent file analysis and callback execution. Zero uses
	// runtime.GOMAXPROCS(0), capped at MaxWorkers. Explicit values must be 1-MaxWorkers.
	// One worker prevents overlapping callbacks within one Analyze call, but does
	// not run them on the caller's goroutine.
	Workers int
	// Dedup counts identical contents only once, retaining the first discovered file.
	Dedup bool

	// ExcludeExts lists case-sensitive extensions without leading dots, such as
	// "go" or "txt". Like --exclude-ext, these exclude the associated detected
	// language, including its other extensions, rather than only a filename suffix.
	ExcludeExts []string
	// IncludeLangs lists exact built-in language names, such as "Go" or "Python".
	// Unknown names are ignored, matching the CLI. If no recognized names remain,
	// all languages are included, subject to other filters.
	IncludeLangs []string
	// Exclude lists case-sensitive globs for Analyze. After removing a trailing
	// slash, unanchored patterns without / match names at any depth; others match
	// paths relative to each input root. A leading slash anchors to that root,
	// and a trailing slash restricts matching to directories. Supports ** and
	// {a,b}; Fullpath has no effect. See README for scan-root and escape semantics.
	Exclude []string

	// Match and NotMatch are inclusion and exclusion regexes for file base names,
	// or full paths when Fullpath is true. Empty patterns disable the filter.
	Match    string
	NotMatch string
	// MatchDir and NotMatchDir match directory paths, not just directory base names.
	// Patterns are regular expressions, not globs; empty patterns disable the filter.
	MatchDir    string
	NotMatchDir string
	Fullpath    bool

	// Debug logs analysis activity, including copies later excluded by Dedup.
	// Records from different files may interleave.
	Debug bool
	// Diagnostics receives warnings and debug logs, never statistical output.
	// Nil uses os.Stderr; io.Discard silences diagnostics. Writes are serialized
	// within each Analyze call. A writer shared across calls must be concurrency-safe.
	Diagnostics io.Writer

	// OnCode receives each code line as normalized by the parser (not raw source;
	// surrounding whitespace and newlines are stripped). Callbacks within one
	// file follow line order; different files may run concurrently. Callers must
	// synchronize shared state. Analyze waits for all callbacks, including on errors.
	// With Dedup, only retained, successfully read files trigger callbacks. Without
	// Dedup, callbacks stream during analysis; later read errors cannot undo them.
	// See ClocOptions.OnCode for buffering, temporary files and replay error semantics.
	OnCode func(line string)
	// OnBlank receives each blank line, with the same contract as OnCode.
	OnBlank func(line string)
	// OnComment receives each comment line, with the same contract as OnCode.
	OnComment func(line string)
}

// OptionError identifies an invalid Options field. Validation happens before
// scanning files or invoking callbacks. Unwrap exposes the underlying error,
// including *syntax.Error for invalid regular expressions.
type OptionError struct {
	Field string
	Err   error
}

func (e *OptionError) Error() string { return fmt.Sprintf("invalid %s: %v", e.Field, e.Err) }
func (e *OptionError) Unwrap() error { return e.Err }

func (opts *Options) prepare(languages *DefinedLanguages) (*ClocOptions, error) {
	return opts.prepareWithExts(languages, Exts)
}

func (opts *Options) prepareWithExts(languages *DefinedLanguages, exts map[string]string) (*ClocOptions, error) {
	if opts == nil {
		opts = &Options{}
	}
	if opts.Workers < 0 || opts.Workers > MaxWorkers {
		return nil, &OptionError{
			Field: "Workers",
			Err:   fmt.Errorf("must be zero (automatic) or between 1 and %d", MaxWorkers),
		}
	}
	workers := opts.Workers
	if workers == 0 {
		workers = min(runtime.GOMAXPROCS(0), MaxWorkers)
	}
	prepared := &ClocOptions{
		Exclude:        append([]string{}, opts.Exclude...),
		Workers:        workers,
		SkipDuplicated: !opts.Dedup,
		ExcludeExts:    make(map[string]struct{}, len(opts.ExcludeExts)),
		IncludeLangs:   make(map[string]struct{}, len(opts.IncludeLangs)),
		Fullpath:       opts.Fullpath,
		Debug:          opts.Debug,
		Diagnostics:    opts.Diagnostics,
		OnCode:         opts.OnCode,
		OnBlank:        opts.OnBlank,
		OnComment:      opts.OnComment,
	}
	for _, filter := range []struct {
		field   string
		pattern string
		target  **regexp.Regexp
	}{
		{field: "Match", pattern: opts.Match, target: &prepared.ReMatch},
		{field: "NotMatch", pattern: opts.NotMatch, target: &prepared.ReNotMatch},
		{field: "MatchDir", pattern: opts.MatchDir, target: &prepared.ReMatchDir},
		{field: "NotMatchDir", pattern: opts.NotMatchDir, target: &prepared.ReNotMatchDir},
	} {
		if filter.pattern == "" {
			continue
		}
		compiled, err := regexp.Compile(filter.pattern)
		if err != nil {
			return nil, &OptionError{Field: filter.field, Err: err}
		}
		*filter.target = compiled
	}
	for _, ext := range opts.ExcludeExts {
		if language, ok := exts[ext]; ok {
			prepared.ExcludeExts[language] = struct{}{}
			continue
		}
		prepared.ExcludeExts[ext] = struct{}{}
	}
	for _, language := range opts.IncludeLangs {
		if _, ok := languages.Langs[language]; ok {
			prepared.IncludeLangs[language] = struct{}{}
		}
	}
	return prepared, nil
}
