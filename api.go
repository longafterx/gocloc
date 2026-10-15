package gocloc

import (
	"io"

	"github.com/longafterx/gocloc/internal/core"
)

// Public types remain aliases so existing callers can use the same fields,
// methods, and function signatures while implementation lives in internal/core.
type (
	ClocFile            = core.ClocFile
	ClocFiles           = core.ClocFiles
	ClocLanguage        = core.ClocLanguage
	Language            = core.Language
	Languages           = core.Languages
	DefinedLanguages    = core.DefinedLanguages
	ClocOptions         = core.ClocOptions
	Options             = core.Options
	OptionError         = core.OptionError
	Result              = core.Result
	JSONLanguagesResult = core.JSONLanguagesResult
	JSONFilesResult     = core.JSONFilesResult
	XMLResultType       = core.XMLResultType
	XMLTotalLanguages   = core.XMLTotalLanguages
	XMLResultLanguages  = core.XMLResultLanguages
	XMLTotalFiles       = core.XMLTotalFiles
	XMLResultFiles      = core.XMLResultFiles
	XMLResult           = core.XMLResult
)

const (
	MaxWorkers         = core.MaxWorkers
	XMLResultWithLangs = core.XMLResultWithLangs
	XMLResultWithFiles = core.XMLResultWithFiles
)

// Exts remains mutable for compatibility with callers that add or replace
// language-extension mappings. Public analysis calls use its current value.
var Exts = core.Exts

// Processor retains the historical constructor and analysis interface.
type Processor struct {
	langs *DefinedLanguages
	opts  *ClocOptions
}

func Analyze(paths []string, opts *Options) (*Result, error) {
	return core.AnalyzeWithExts(paths, opts, Exts)
}

func NewProcessor(langs *DefinedLanguages, opts *ClocOptions) *Processor {
	return &Processor{langs: langs, opts: opts}
}

func (p *Processor) Analyze(paths []string) (*Result, error) {
	return core.NewProcessorWithExts(p.langs, p.opts, Exts).Analyze(paths)
}

func NewClocOptions() *ClocOptions {
	return core.NewClocOptions()
}

func NewLanguage(name string, lineComments []string, multiLines [][]string) *Language {
	return core.NewLanguage(name, lineComments, multiLines)
}

func NewDefinedLanguages() *DefinedLanguages {
	return core.NewDefinedLanguagesWithExtensionSource(&Exts)
}

func AnalyzeFile(filename string, language *Language, opts *ClocOptions) *ClocFile {
	return core.AnalyzeFile(filename, language, opts)
}

func AnalyzeReader(filename string, language *Language, reader io.Reader, opts *ClocOptions) *ClocFile {
	return core.AnalyzeReader(filename, language, reader, opts)
}

func NewJSONLanguagesResultFromCloc(total *Language, languages Languages) JSONLanguagesResult {
	return core.NewJSONLanguagesResultFromCloc(total, languages)
}

func NewJSONFilesResultFromCloc(total *Language, files ClocFiles) JSONFilesResult {
	return core.NewJSONFilesResultFromCloc(total, files)
}

func NewXMLResultFromCloc(total *Language, languages Languages, kind XMLResultType) *XMLResult {
	return core.NewXMLResultFromCloc(total, languages, kind)
}

func InsertPipesInTheMiddle(input string) string {
	return core.InsertPipesInTheMiddle(input)
}
