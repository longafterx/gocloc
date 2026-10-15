package core

import (
	"io"
	"regexp"
)

// MaxWorkers limits fixed worker configurations to a bounded resource footprint.
const MaxWorkers = 64

// ClocOptions configures Processor, AnalyzeFile and AnalyzeReader.
// Its historical defaults are preserved: Workers <= 1 selects one worker and
// the zero value enables deduplication. NewClocOptions disables deduplication.
// For automatic concurrency and consistent zero-value defaults, use Options with Analyze.
type ClocOptions struct {
	// Debug logs analysis activity, not just files included in the result.
	// Records from different files may interleave when using multiple workers.
	Debug bool
	// SkipDuplicated disables content-based duplicate detection when true.
	// NewClocOptions sets this to true; false enables deduplication.
	SkipDuplicated bool
	ExcludeExts    map[string]struct{}
	IncludeLangs   map[string]struct{}
	// Exclude configures glob exclusions for Processor.Analyze, with the same
	// semantics as Options.Exclude. AnalyzeFile and AnalyzeReader do not apply it.
	Exclude       []string
	ReNotMatch    *regexp.Regexp
	ReMatch       *regexp.Regexp
	ReNotMatchDir *regexp.Regexp
	ReMatchDir    *regexp.Regexp
	Fullpath      bool

	// Workers limits concurrent file analysis and concurrent callback execution.
	// Values <= 1 use one worker; values above MaxWorkers are capped.
	Workers int

	// Diagnostics receives warnings and debug logs, never statistical output.
	// Nil uses os.Stderr; io.Discard silences diagnostics. Processor.Analyze
	// serializes writes within a scan, including writes from traversal and workers.
	Diagnostics io.Writer
	diagnostics *diagnosticSink
	exts        map[string]string
	extsSet     bool
	excluder    *pathExcluder

	// OnCode is triggered for each line of code.
	// Processor.Analyze invokes callbacks in workers, never guaranteeing the
	// caller's goroutine. Calls within one file follow line order; different files
	// may overlap. Callers must synchronize shared state and let callbacks return.
	// Analyze waits for all callbacks. Workers=1 prevents overlapping calls within
	// one Analyze invocation. AnalyzeFile and AnalyzeReader call synchronously.
	//
	// With deduplication, events are buffered and only retained, successfully read
	// files trigger callbacks. The event cache has a bounded memory budget and
	// spills to temporary files, removed after replay or exclusion. Without
	// deduplication, callbacks stream during analysis; later read errors cannot
	// undo them. Replay errors likewise cannot undo already delivered events.
	OnCode func(line string)
	// OnBlank is triggered for each blank line, with the same contract as OnCode.
	OnBlank func(line string)
	// OnComment is triggered for each comment line, with the same contract as OnCode.
	OnComment func(line string)
}

// NewClocOptions creates options that count identical files separately by default.
func NewClocOptions() *ClocOptions {
	return &ClocOptions{
		Debug:          false,
		SkipDuplicated: true,
		ExcludeExts:    make(map[string]struct{}),
		IncludeLangs:   make(map[string]struct{}),
	}
}
