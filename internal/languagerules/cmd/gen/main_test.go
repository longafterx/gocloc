package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/longafterx/gocloc/internal/core"
	"github.com/longafterx/gocloc/internal/lexer"
)

func TestGenerateArtifacts(t *testing.T) {
	t.Parallel()
	content, err := os.ReadFile(filepath.Join("../..", sourcePath))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(content)); got != sourceSHA256 {
		t.Fatalf("snapshot checksum = %s", got)
	}
	var upstream upstreamRules
	if err := decodeJSON(content, &upstream); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(filepath.Join("../..", mappingPath))
	if err != nil {
		t.Fatal(err)
	}
	mappings := map[string]languageMapping{}
	if err := decodeJSON(content, &mappings); err != nil {
		t.Fatal(err)
	}
	if len(upstream.Languages) != 333 {
		t.Fatalf("catalog has %d entries, want 333", len(upstream.Languages))
	}
	definitions := core.NewDefinedLanguages()
	if len(mappings) != len(definitions.Langs) {
		t.Fatalf("mapping size=%d, built-in definitions=%d", len(mappings), len(definitions.Langs))
	}
	for name := range definitions.Langs {
		mapping, ok := mappings[name]
		if !ok {
			t.Fatalf("missing explicit mapping for %q", name)
		}
		if active := lexer.BuiltIn(name) != nil; active != (mapping.KeepLegacy == "") {
			t.Fatalf("activation for %q ignores its mapping", name)
		}
	}
	code, report, err := generateArtifacts(upstream.Languages, mappings)
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []struct {
		name string
		path string
		data []byte
	}{
		{name: "rules", path: outputPath, data: code},
		{name: "coverage", path: coveragePath, data: report},
	} {
		t.Run(artifact.name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join("../..", artifact.path))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(artifact.data, want) {
				t.Fatal("generated artifact is stale; run go generate ./...")
			}
		})
	}
	again, againReport, err := generateArtifacts(upstream.Languages, mappings)
	if err != nil || !bytes.Equal(again, code) || !bytes.Equal(againReport, report) {
		t.Fatalf("generation depends on map iteration order: %v", err)
	}
}

func TestValidateMappings(t *testing.T) {
	t.Parallel()
	upstream := map[string]upstreamRule{"C": {}, "Html": {}}
	for _, test := range []struct {
		name    string
		mapping languageMapping
		valid   bool
	}{
		{name: "exact source", mapping: languageMapping{Source: "C"}, valid: true},
		{name: "explicit legacy", mapping: languageMapping{KeepLegacy: "not in snapshot"}, valid: true},
		{name: "deferred source", mapping: languageMapping{Source: "Html", KeepLegacy: "deferred"}, valid: true},
		{name: "missing source"},
		{name: "unknown source", mapping: languageMapping{Source: "missing"}},
		{name: "embedding accidentally activated", mapping: languageMapping{Source: "Html"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateMappings(upstream, map[string]languageMapping{"name": test.mapping})
			if (err == nil) != test.valid {
				t.Fatalf("validation = %v, want valid=%t", err, test.valid)
			}
		})
	}
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, source string
		valid        bool
	}{
		{name: "known fields", source: "{\"languages\":{\"L\":{\"quotes\":[[\"a\",\"b\"]],\"literate\":true}}}", valid: true},
		{name: "unknown rule field", source: "{\"languages\":{\"L\":{\"unknown\":true}}}"},
		{name: "trailing document", source: "{\"languages\":{}} {}"},
		{name: "trailing garbage", source: "{\"languages\":{}} garbage"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var rules upstreamRules
			if err := decodeJSON([]byte(test.source), &rules); (err == nil) != test.valid {
				t.Fatalf("decode = %v, want valid=%t", err, test.valid)
			}
		})
	}
}

func TestQuotePair(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		pair  []string
		open  string
		close string
		valid bool
	}{
		{name: "escaped quote", pair: []string{"\\\"", "\\\""}, open: "\"", close: "\"", valid: true},
		{name: "triple quote", pair: []string{"'''", "'''"}, open: "'''", close: "'''", valid: true},
		{name: "literal backslash", pair: []string{"\\", "/"}, open: "\\", close: "/", valid: true},
		{name: "missing closer", pair: []string{"open"}},
		{name: "empty opener", pair: []string{"", "close"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			open, close, err := quotePair(test.pair)
			if (err == nil) != test.valid || open != test.open || close != test.close {
				t.Fatalf("quotePair = %q/%q/%v", open, close, err)
			}
		})
	}
}
