package core

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"sort"
	"unicode"
	"unicode/utf8"

	"github.com/longafterx/gocloc/internal/lexer"
)

// ClocFile is collecting to line count result.
type ClocFile struct {
	Code     int32  `xml:"code,attr" json:"code"`
	Comments int32  `xml:"comment,attr" json:"comment"`
	Blanks   int32  `xml:"blank,attr" json:"blank"`
	Name     string `xml:"name,attr" json:"name"`
	Lang     string `xml:"language,attr" json:"language"`
}

// ClocFiles is gocloc result set.
type ClocFiles []ClocFile

func (cf ClocFiles) SortByName() {
	sortFunc := func(i, j int) bool {
		return cf[i].Name < cf[j].Name
	}
	sort.Slice(cf, sortFunc)
}

func (cf ClocFiles) SortByComments() {
	sortFunc := func(i, j int) bool {
		if cf[i].Comments == cf[j].Comments {
			return cf[i].Code > cf[j].Code
		}
		return cf[i].Comments > cf[j].Comments
	}
	sort.Slice(cf, sortFunc)
}

func (cf ClocFiles) SortByBlanks() {
	sortFunc := func(i, j int) bool {
		if cf[i].Blanks == cf[j].Blanks {
			return cf[i].Code > cf[j].Code
		}
		return cf[i].Blanks > cf[j].Blanks
	}
	sort.Slice(cf, sortFunc)
}

func (cf ClocFiles) SortByCode() {
	sortFunc := func(i, j int) bool {
		return cf[i].Code > cf[j].Code
	}
	sort.Slice(cf, sortFunc)
}

// AnalyzeFile analyzes a file and reports failures through opts.Diagnostics.
// Its signature is retained for callers; Processor excludes failed files.
func AnalyzeFile(filename string, language *Language, opts *ClocOptions) *ClocFile {
	opts = opts.withDiagnostics()
	file, err := analyzeFile(filename, language, opts)
	opts.warn(err)
	if file == nil {
		return &ClocFile{Name: filename}
	}
	return file
}

func analyzeFile(filename string, language *Language, opts *ClocOptions) (*ClocFile, error) {
	fp, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	file, readErr := analyzeReader(filename, language, fp, opts)
	return file, errors.Join(readErr, fp.Close())
}

type hashingReader struct {
	reader io.Reader
	hash   hash.Hash
}

// detectAndAnalyzeFile reuses the detection prefix for line analysis and, when
// deduplication is enabled, hashes physical reads once. Workers bound open files.
func detectAndAnalyzeFile(
	candidate fileCandidate,
	languages *DefinedLanguages,
	opts *ClocOptions,
	reader *lineReader,
) (result fileAnalysisResult) {
	result = fileAnalysisResult{fileCandidate: candidate, ignored: true}
	file, err := os.Open(candidate.path)
	if err != nil {
		result.err = err
		return result
	}
	defer func() {
		result.err = errors.Join(result.err, file.Close())
	}()

	var source io.Reader = file
	var hashed *hashingReader
	if !opts.SkipDuplicated {
		hashed = &hashingReader{reader: file, hash: md5.New()}
		source = hashed
	}
	reader.reset(source)
	defer reader.release()
	var prefix []byte
	var detectionErr error
	ext, ok := detectFileType(candidate.path, opts, func(all bool) ([]byte, error) {
		var readErr error
		if all {
			prefix, readErr = io.ReadAll(reader.reader)
		} else {
			prefix, readErr = reader.next()
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			detectionErr = fmt.Errorf("read %q for language detection: %w", candidate.path, readErr)
		}
		return prefix, readErr
	})
	if detectionErr != nil {
		result.err = detectionErr
		return result
	}
	if !ok {
		return result
	}
	exts := opts.exts
	if !opts.extsSet {
		exts = Exts
	}
	languageKey, ok := exts[ext]
	if !ok {
		return result
	}
	if !includeLanguage(languageKey, opts) {
		return result
	}
	result.languageKey = languageKey
	// Detection's bytes are consumed before the next physical read; replaying
	// them requires neither another buffered reader nor another hash update.
	reader.prefix = prefix
	result.clocFile, result.err = analyzeLines(
		candidate.path,
		languages.Langs[languageKey],
		reader,
		opts,
	)
	if result.err != nil {
		return result
	}
	if hashed != nil {
		copy(result.digest[:], hashed.hash.Sum(nil))
	}
	result.ignored = false
	return result
}

func (r *hashingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		if _, hashErr := r.hash.Write(p[:n]); hashErr != nil {
			return n, hashErr
		}
	}
	return n, err
}

// AnalyzeReader counts lines and reports read failures through opts.Diagnostics.
// Partial counts are returned for compatibility; Processor excludes failed files.
func AnalyzeReader(filename string, language *Language, file io.Reader, opts *ClocOptions) *ClocFile {
	opts = opts.withDiagnostics()
	result, err := analyzeReader(filename, language, file, opts)
	opts.warn(err)
	return result
}

func analyzeReader(filename string, language *Language, file io.Reader, opts *ClocOptions) (*ClocFile, error) {
	return analyzeLines(
		filename,
		language,
		newLineReader(file),
		opts,
	)
}

func analyzeLines(filename string, language *Language, reader *lineReader, opts *ClocOptions) (*ClocFile, error) {
	if opts.Debug {
		opts.diagnosticf("[FILE] file=%q\n", filename)
	}

	clocFile := &ClocFile{
		Name: filename,
		Lang: language.Name,
	}

	isFirstLine := true
	rules := syntaxForLanguage(language)
	useSyntax := rules != nil
	var scanner lexer.Scanner
	if useSyntax {
		scanner = lexer.NewScanner(rules)
	}
	inComments := [][2]string{}
	// Reuse flags for every multiline-comment row instead of allocating per row.
	codeFlags := make([]bool, len(language.multiLines))

scannerloop:
	for {
		lineOrg, err := reader.next()
		if err != nil && !errors.Is(err, io.EOF) {
			return clocFile, fmt.Errorf("read %q: %w", filename, err)
		}

		// prevent infinite loop
		if len(lineOrg) == 0 && err == io.EOF {
			break
		}

		line := bytes.TrimSpace(lineOrg)
		if useSyntax {
			input := lineOrg
			if isFirstLine {
				input = bytes.TrimPrefix(input, []byte("\xef\xbb\xbf"))
				line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
			}
			isShebang := isFirstLine && bytes.HasPrefix(line, []byte("#!"))
			isFirstLine = false
			if isShebang {
				onCode(
					clocFile,
					opts,
					scanner.InComment(),
					line,
					lineOrg,
				)
				continue
			}
			isCode := scanner.ScanLine(input, line)
			switch {
			case len(line) == 0:
				onBlank(
					clocFile,
					opts,
					scanner.InComment(),
					line,
					lineOrg,
				)
			case isCode:
				onCode(
					clocFile,
					opts,
					scanner.InComment(),
					line,
					lineOrg,
				)
			default:
				onComment(
					clocFile,
					opts,
					scanner.InComment(),
					line,
					lineOrg,
				)
			}
			continue
		}

		if len(line) == 0 {
			onBlank(
				clocFile,
				opts,
				len(inComments) > 0,
				line,
				lineOrg,
			)
			continue
		}

		// shebang line is 'code'
		if isFirstLine && bytes.HasPrefix(line, []byte("#!")) {
			onCode(
				clocFile,
				opts,
				len(inComments) > 0,
				line,
				lineOrg,
			)
			isFirstLine = false
			continue
		}

		if len(inComments) == 0 {
			if isFirstLine {
				line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
			}

			if len(language.regexLineComments) > 0 {
			singleloopRegex:
				for _, singleCommentRegex := range language.regexLineComments {
					if singleCommentRegex.Match(line) {
						// check if single comment is a prefix of multi comment
						for _, ml := range language.multiLines {
							if ml[0] != "" && bytes.HasPrefix(line, []byte(ml[0])) {
								break singleloopRegex
							}
						}
						onComment(
							clocFile,
							opts,
							len(inComments) > 0,
							line,
							lineOrg,
						)
						continue scannerloop
					}
				}
			} else {
			singleloop:
				for _, singleComment := range language.lineComments {
					if bytes.HasPrefix(line, []byte(singleComment)) {
						// check if single comment is a prefix of multi comment
						for _, ml := range language.multiLines {
							if ml[0] != "" && bytes.HasPrefix(line, []byte(ml[0])) {
								break singleloop
							}
						}
						onComment(
							clocFile,
							opts,
							len(inComments) > 0,
							line,
							lineOrg,
						)
						continue scannerloop
					}
				}
			}

			if len(language.multiLines) == 0 {
				onCode(
					clocFile,
					opts,
					len(inComments) > 0,
					line,
					lineOrg,
				)
				continue scannerloop
			}
		}

		if len(inComments) == 0 && !containsCommentBytes(line, language.multiLines) {
			onCode(
				clocFile,
				opts,
				len(inComments) > 0,
				line,
				lineOrg,
			)
			continue scannerloop
		}

		lenLine := len(line)
		singleDelimiter := len(language.multiLines) == 1 && len(language.multiLines[0]) == 2
		if singleDelimiter && language.multiLines[0][0] == "" {
			onCode(
				clocFile,
				opts,
				len(inComments) > 0,
				line,
				lineOrg,
			)
			continue
		}
		clear(codeFlags)
		for pos := 0; pos < lenLine; {
			for idx, ml := range language.multiLines {
				begin, end := ml[0], ml[1]
				lenBegin := len(begin)

				hasBegin := pos+lenBegin <= lenLine && bytes.HasPrefix(line[pos:], []byte(begin))
				if hasBegin && (begin != end || len(inComments) == 0) {
					pos += lenBegin
					inComments = append(inComments, [2]string{begin, end})
					continue
				}

				if n := len(inComments); n > 0 {
					last := inComments[n-1]
					if pos+len(last[1]) <= lenLine && bytes.HasPrefix(line[pos:], []byte(last[1])) {
						inComments = inComments[:n-1]
						pos += len(last[1])
					}
				} else if pos < lenLine {
					r, _ := utf8.DecodeRune(line[pos:])
					if !unicode.IsSpace(r) {
						codeFlags[idx] = true
					}
				}
			}
			pos++
		}

		isCode := true
		for _, b := range codeFlags {
			if !b {
				isCode = false
			}
		}

		if isCode {
			onCode(
				clocFile,
				opts,
				len(inComments) > 0,
				line,
				lineOrg,
			)
		} else {
			onComment(
				clocFile,
				opts,
				len(inComments) > 0,
				line,
				lineOrg,
			)
		}

		if err == io.EOF {
			break
		}
	}

	return clocFile, nil
}

// Materialize strings only for observers. Callback strings must outlive the
// borrowed line buffer, which the next read (or next file) may overwrite.
func onBlank(clocFile *ClocFile, opts *ClocOptions, isInComments bool, line, lineOrg []byte) {
	clocFile.Blanks++
	if opts.OnBlank != nil {
		opts.OnBlank(string(line))
	}

	if opts.Debug {
		opts.debugLine(
			"BLNK",
			clocFile,
			isInComments,
			lineOrg,
		)
	}
}

func onComment(clocFile *ClocFile, opts *ClocOptions, isInComments bool, line, lineOrg []byte) {
	clocFile.Comments++
	if opts.OnComment != nil {
		opts.OnComment(string(line))
	}

	if opts.Debug {
		opts.debugLine(
			"COMM",
			clocFile,
			isInComments,
			lineOrg,
		)
	}
}

func onCode(clocFile *ClocFile, opts *ClocOptions, isInComments bool, line, lineOrg []byte) {
	clocFile.Code++
	if opts.OnCode != nil {
		opts.OnCode(string(line))
	}

	if opts.Debug {
		opts.debugLine(
			"CODE",
			clocFile,
			isInComments,
			lineOrg,
		)
	}
}
