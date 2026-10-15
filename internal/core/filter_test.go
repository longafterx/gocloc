package core

import (
	"path"
	"testing"
)

func TestGlobExclusionSemantics(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		pattern  string
		relative string
		dir      bool
		want     bool
	}{
		{name: "directory", pattern: "target/", relative: "target", dir: true, want: true},
		{name: "nested directory", pattern: "target/", relative: "src/target", dir: true, want: true},
		{name: "directory only", pattern: "target/", relative: "target"},
		{name: "bare name file", pattern: "target", relative: "src/target", want: true},
		{name: "bare name directory", pattern: "target", relative: "src/target", dir: true, want: true},
		{name: "whole name", pattern: "target/", relative: "gcc.target", dir: true},
		{name: "case sensitive", pattern: "target/", relative: "Target", dir: true},
		{name: "extension case", pattern: "*.c", relative: "test.C"},
		{
			name: "braces", pattern: "{dist,node_modules,target}/",
			relative: "src/node_modules", dir: true, want: true,
		},
		{
			name: "braces whole name", pattern: "{dist,node_modules,target}/",
			relative: "distribution", dir: true,
		},
		{name: "anchored root", pattern: "/target/", relative: "target", dir: true, want: true},
		{name: "anchored nested", pattern: "/target/", relative: "src/target", dir: true},
		{name: "anchored filename", pattern: "/README.md", relative: "README.md", want: true},
		{name: "anchored filename nested", pattern: "/README.md", relative: "docs/README.md"},
		{name: "filename wildcard", pattern: "*.min.js", relative: "web/app.min.js", want: true},
		{name: "relative path", pattern: "src/generated/", relative: "src/generated", dir: true, want: true},
		{name: "relative path anchored", pattern: "src/generated/", relative: "other/src/generated", dir: true},
		{
			name: "star one segment", pattern: "src/*/generated/",
			relative: "src/a/generated", dir: true, want: true,
		},
		{name: "star cannot cross slash", pattern: "src/*/generated/", relative: "src/a/b/generated", dir: true},
		{
			name: "globstar zero segments", pattern: "src/**/generated/",
			relative: "src/generated", dir: true, want: true,
		},
		{
			name: "globstar many segments", pattern: "src/**/generated/",
			relative: "src/a/b/generated", dir: true, want: true,
		},
		{name: "leading globstar zero", pattern: "**/generated/", relative: "generated", dir: true, want: true},
		{name: "leading globstar nested", pattern: "**/generated/", relative: "src/generated", dir: true, want: true},
		{name: "trailing globstar directory", pattern: "generated/**", relative: "generated", dir: true, want: true},
		{name: "trailing globstar file", pattern: "generated/**", relative: "generated/src/a.go", want: true},
		{
			name: "brace paths", pattern: "{src,test}/generated/",
			relative: "test/generated", dir: true, want: true,
		},
		{name: "question mark", pattern: "test?.go", relative: "src/test1.go", want: true},
		{name: "character class", pattern: "test[0-9].go", relative: "test2.go", want: true},
		{name: "negative class", pattern: "test[!0-9].go", relative: "testa.go", want: true},
		{name: "escaped wildcard", pattern: `test\*.go`, relative: "test*.go", want: true},
		{name: "escaped wildcard literal", pattern: `test\*.go`, relative: "test1.go"},
		{name: "spaces preserved", pattern: " target ", relative: " target ", want: true},
		{name: "spaces not trimmed", pattern: " target ", relative: "target"},
		{name: "dot prefix", pattern: "./target/", relative: "target", dir: true, want: true},
		{name: "hidden filename", pattern: "*.go", relative: ".hidden.go", want: true},
		{name: "pipe literal", pattern: "dist|target", relative: "dist|target", want: true},
		{name: "pipe no alternation", pattern: "dist|target", relative: "target"},
		{name: "exclamation literal", pattern: "!target", relative: "!target", want: true},
		{name: "hash literal", pattern: "#target", relative: "#target", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filter, err := prepareExcludes([]string{tc.pattern})
			if err != nil {
				t.Fatal(err)
			}
			if got := filter.excluded(path.Base(tc.relative), tc.relative, tc.dir); got != tc.want {
				t.Fatalf(
					"%q matches %q: %v, want %v",
					tc.pattern,
					tc.relative,
					got,
					tc.want,
				)
			}
		})
	}
}
