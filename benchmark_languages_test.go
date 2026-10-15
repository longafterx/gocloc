package gocloc

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/longafterx/gocloc/internal/lexertest"
)

const languageFixtureVersion = "v2"

type languageFixture struct {
	root    string
	digest  string
	bytes   int64
	sources []lexertest.Case
}

func languageFixtureGroups() [][]lexertest.Case {
	groups := [][]lexertest.Case{}
	for _, source := range lexertest.BenchmarkCases() {
		groups = append(groups, []lexertest.Case{source})
	}
	return append(groups, lexertest.BenchmarkCases())
}

func languageFixtureName(sources []lexertest.Case) string {
	if len(sources) == 1 {
		return sources[0].Name
	}
	return "mixed"
}

func writeLanguageFixture(tb testing.TB, sources []lexertest.Case) languageFixture {
	tb.Helper()
	fixture := languageFixture{root: tb.TempDir(), sources: sources}
	digest := sha256.New()
	for _, source := range sources {
		for i := range 8 {
			marker, closer := "//", ""
			switch source.Language {
			case "Python", "BASH":
				marker = "#"
			case "SQL", "Lua":
				marker = "--"
			case "XML":
				marker, closer = "<!--", " -->"
			}
			// Pairs have identical contents so both dedup modes have fixed,
			// independently asserted counts and first-discovered filenames.
			content := []byte(strings.Repeat(source.Source, 128) + fmt.Sprintf("%s fixture %02d%s\n", marker, i%4, closer))
			name := fmt.Sprintf("%s/file%02d.%s", source.Name, i, source.Extension)
			path := filepath.Join(fixture.root, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				tb.Fatal(err)
			}
			if err := os.WriteFile(path, content, 0600); err != nil {
				tb.Fatal(err)
			}
			fmt.Fprintf(digest, "%s\x00%d\x00", name, len(content))
			digest.Write(content)
			fixture.bytes += int64(len(content))
		}
	}
	fixture.digest = fmt.Sprintf("%x", digest.Sum(nil))
	return fixture
}

func (fixture languageFixture) verify(tb testing.TB, result *Result, dedup bool) {
	tb.Helper()
	files := 8
	if dedup {
		files = 4
	}
	if result == nil || result.Total == nil || len(result.Languages) != len(fixture.sources) {
		tb.Fatalf("unexpected result: %+v", result)
	}
	var code, comments, blanks int32
	for _, source := range fixture.sources {
		wantCode := int32(files * 128 * strings.Count(source.Kinds, "C"))
		wantComments := int32(files * (128*strings.Count(source.Kinds, "M") + 1))
		wantBlanks := int32(files * 128 * strings.Count(source.Kinds, "B"))
		language := result.Languages[source.Language]
		if language == nil || len(language.Files) != files || language.Code != wantCode ||
			language.Comments != wantComments || language.Blanks != wantBlanks {
			tb.Fatalf("%s counts = %+v, want %d files and %d/%d/%d lines",
				source.Language, language, files, wantCode, wantComments, wantBlanks)
		}
		for i := range files {
			name := fmt.Sprintf("%s/file%02d.%s", source.Name, i, source.Extension)
			file := result.Files[filepath.Join(fixture.root, filepath.FromSlash(name))]
			if file == nil || file.Lang != source.Language {
				tb.Fatalf("missing retained file %s", name)
			}
		}
		code += wantCode
		comments += wantComments
		blanks += wantBlanks
	}
	if result.Total.Code != code || result.Total.Comments != comments || result.Total.Blanks != blanks ||
		result.Total.Total != int32(files*len(fixture.sources)) || len(result.Files) != files*len(fixture.sources) {
		tb.Fatalf("incorrect aggregate: %+v", result.Total)
	}
}

func TestBenchmarkLanguages(t *testing.T) {
	for _, sources := range languageFixtureGroups() {
		t.Run(languageFixtureName(sources), func(t *testing.T) {
			fixture := writeLanguageFixture(t, sources)
			copy := writeLanguageFixture(t, sources)
			if fixture.digest != copy.digest || fixture.bytes != copy.bytes {
				t.Fatal("language fixture depends on the temporary directory")
			}
			t.Logf("fixture=languages/%s/%s bytes=%d sha256=%s",
				languageFixtureVersion, languageFixtureName(sources), fixture.bytes, fixture.digest)
			for _, workers := range []int{1, 8} {
				for _, dedup := range []bool{false, true} {
					t.Run(fmt.Sprintf("workers=%d/dedup=%t", workers, dedup), func(t *testing.T) {
						var code, comments, blanks atomic.Int32
						result, err := Analyze([]string{fixture.root}, &Options{
							Workers: workers, Dedup: dedup,
							OnCode: func(string) { code.Add(1) }, OnComment: func(string) { comments.Add(1) },
							OnBlank: func(string) { blanks.Add(1) },
						})
						if err != nil {
							t.Fatal(err)
						}
						fixture.verify(t, result, dedup)
						if code.Load() != result.Total.Code || comments.Load() != result.Total.Comments ||
							blanks.Load() != result.Total.Blanks {
							t.Fatalf("callbacks = %d/%d/%d, result = %+v", code.Load(), comments.Load(), blanks.Load(), result.Total)
						}
					})
				}
			}
		})
	}
}

func BenchmarkProcessorLanguages(b *testing.B) {
	for _, sources := range languageFixtureGroups() {
		b.Run(languageFixtureVersion+"/"+languageFixtureName(sources), func(b *testing.B) {
			fixture := writeLanguageFixture(b, sources)
			b.Logf("bytes=%d sha256=%s", fixture.bytes, fixture.digest)
			for _, workers := range []int{1, 8} {
				for _, dedup := range []bool{false, true} {
					b.Run(fmt.Sprintf("workers=%d/dedup=%t", workers, dedup), func(b *testing.B) {
						paths := []string{fixture.root}
						opts := &Options{Workers: workers, Dedup: dedup}
						result, err := Analyze(paths, opts)
						if err != nil {
							b.Fatal(err)
						}
						fixture.verify(b, result, dedup)
						b.ReportAllocs()
						b.SetBytes(fixture.bytes)
						b.ResetTimer()
						for range b.N {
							result, err = Analyze(paths, opts)
							if err != nil {
								b.Fatal(err)
							}
						}
						b.StopTimer()
						fixture.verify(b, result, dedup)
					})
				}
			}
		})
	}
}
