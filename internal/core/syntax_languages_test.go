package core

import (
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/longafterx/gocloc/internal/lexertest"
)

func TestLanguageCorpus(t *testing.T) {
	t.Parallel()
	for _, fixture := range append(lexertest.Cases(), lexertest.BenchmarkCases()...) {
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()
			language := NewDefinedLanguages().Langs[fixture.Language]
			if language == nil || syntaxForLanguage(language) == nil {
				t.Fatalf("missing lexical rules for %s", fixture.Language)
			}
			lines := physicalLines(fixture.Source)
			if len(lines) != len(fixture.Kinds) {
				t.Fatalf("invalid fixture: %d physical lines, %d expectations", len(lines), len(fixture.Kinds))
			}
			for _, mode := range []struct {
				name   string
				reader func() io.Reader
				debug  bool
			}{
				{name: "buffered", reader: func() io.Reader { return strings.NewReader(fixture.Source) }},
				{name: "fragmented debug", debug: true, reader: func() io.Reader {
					return iotest.HalfReader(strings.NewReader(fixture.Source))
				}},
			} {
				t.Run(mode.name, func(t *testing.T) {
					var kinds strings.Builder
					gotLines := []string{}
					callback := func(kind byte) func(string) {
						return func(line string) {
							kinds.WriteByte(kind)
							gotLines = append(gotLines, line)
						}
					}
					file, err := analyzeReader("sample."+fixture.Extension, language, mode.reader(), (&ClocOptions{
						Debug: mode.debug, Diagnostics: io.Discard,
						OnCode: callback('C'), OnComment: callback('M'), OnBlank: callback('B'),
					}).withDiagnostics())
					if err != nil {
						t.Fatal(err)
					}
					if kinds.String() != fixture.Kinds {
						t.Fatalf("line kinds = %s, want %s", &kinds, fixture.Kinds)
					}
					for i, line := range lines {
						lines[i] = strings.TrimSpace(line)
						if i == 0 {
							lines[i] = strings.TrimPrefix(lines[i], "\xef\xbb\xbf")
						}
					}
					if !reflect.DeepEqual(gotLines, lines) {
						t.Fatalf("callback lines differ: got %q, want %q", gotLines, lines)
					}
					if file.Code != int32(strings.Count(fixture.Kinds, "C")) ||
						file.Comments != int32(strings.Count(fixture.Kinds, "M")) ||
						file.Blanks != int32(strings.Count(fixture.Kinds, "B")) {
						t.Fatalf("counts disagree with line classifications: %+v", file)
					}
				})
			}
		})
	}
}

func physicalLines(source string) []string {
	if source == "" {
		return []string{}
	}
	return strings.Split(strings.TrimSuffix(source, "\n"), "\n")
}

func FuzzLanguageReader(f *testing.F) {
	fixtures := lexertest.BenchmarkCases()
	for i, fixture := range fixtures {
		f.Add(fixture.Source, uint8(i))
	}
	languages := NewDefinedLanguages()
	f.Fuzz(func(t *testing.T, source string, index uint8) {
		if len(source) > 1024*1024 {
			t.Skip()
		}
		language := languages.Langs[fixtures[int(index)%len(fixtures)].Language]
		want, wantErr := analyzeReader("sample", language, strings.NewReader(source), NewClocOptions())
		got, gotErr := analyzeReader("sample", language, iotest.HalfReader(strings.NewReader(source)), NewClocOptions())
		if wantErr != nil || gotErr != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("fragmentation changed analysis: %+v/%v versus %+v/%v", got, gotErr, want, wantErr)
		}
		if got.Code+got.Comments+got.Blanks != int32(len(physicalLines(source))) {
			t.Fatalf("physical lines were lost or counted twice: %+v", got)
		}
	})
}
