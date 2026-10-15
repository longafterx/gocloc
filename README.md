# gocloc

[English](README.md) | [简体中文](README.zh-CN.md)

[![GoDoc](https://godoc.org/github.com/rustyllh/gocloc?status.svg)](https://godoc.org/github.com/rustyllh/gocloc)
[![ci](https://github.com/rustyllh/gocloc/workflows/Go/badge.svg)](https://github.com/rustyllh/gocloc/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/rustyllh/gocloc)](https://goreportcard.com/report/github.com/rustyllh/gocloc)
[![Docker Pulls](https://img.shields.io/docker/pulls/rustyllh/gocloc)](https://hub.docker.com/r/rustyllh/gocloc)
[![Docker Image Size](https://img.shields.io/docker/image-size/rustyllh/gocloc/latest)](https://hub.docker.com/r/rustyllh/gocloc)

A high-performance, parallel source code line counter written in Go, based on [hhatto/gocloc](https://github.com/hhatto/gocloc).
Inspired by [tokei](https://github.com/Aaronepower/tokei) and faster in our latest Go repository benchmarks.

## Installation

### macOS / Linux

Install the latest stable release (no Go required; SHA-256 verified):

```sh
curl --proto '=https' --tlsv1.2 -fsSL https://raw.githubusercontent.com/rustyllh/gocloc/master/install.sh | sh
```

Requires `curl`, `tar`, and `sha256sum` or `shasum`. Installs to `$HOME/.local/bin`; add it to your `PATH` if needed.

### Windows

Run in PowerShell 5.1+:

```powershell
& ([scriptblock]::Create((Invoke-RestMethod https://raw.githubusercontent.com/rustyllh/gocloc/master/install.ps1 -ErrorAction Stop)))
```

Installs to `%LOCALAPPDATA%\gocloc\bin`; add it to your user `Path`. Supports x86-64 and i386, not Windows ARM64.

Both installers execute downloaded code; review [install.sh](install.sh) or [install.ps1](install.ps1) first if needed.
Run the installer again to upgrade, then check `gocloc --version`.
Alternatively, download a binary and its checksums from [GitHub Releases](https://github.com/rustyllh/gocloc/releases).

### Docker

Run against the current directory with a read-only mount:

```sh
docker run --rm --read-only --mount "type=bind,source=$(pwd),target=/workdir,readonly" rustyllh/gocloc:latest .
```

Supports Linux `amd64` and `arm64`, selected automatically. Use `:vX.Y.Z` to pin a release; `:latest` tracks stable releases.
Append CLI options after the image name, for example `--dedup -o json .`.

## Performance

* measured on 2026-10-15
* CPU Apple M3 8-core / 16 GiB / macOS 27.0.1 arm64
* optimized gocloc: [38bad47](https://github.com/rustyllh/gocloc/commit/38bad4752b755918759c2d2b4703b458125b01f4) (source build)
* [tokei 14.0.0](https://github.com/XAMPPRocky/tokei/releases/tag/v14.0.0) compiled with serialization support: json
* upstream gocloc [679b457](https://github.com/hhatto/gocloc/commit/679b457182dcf852d90e52f132403d6b7de0e33d) (source build; [releases](https://github.com/hhatto/gocloc/releases))
* [cloc 2.04](https://github.com/AlDanial/cloc/releases/tag/v2.04)
* target repository is [golang/go](https://github.com/golang/go), branch `master`, commit [6b3800e](https://github.com/golang/go/commit/6b3800e1dd90925b4184acd6391f68bffd16b6a5)

Both gocloc binaries were built with Go 1.27.2. All tools scanned the repository above using default options, with no runtime or thread overrides. Each tool had one untimed warm-up; timed output went to `/dev/null`. The fast tools ran in alternating order, followed by cloc. All samples were retained. The command output below is from the warm-up; the `time` lines show averages over the timed runs.

| Tool | Runs | Mean (s) | Median (s) | Range (s) |
| --- | ---: | ---: | ---: | ---: |
| optimized gocloc | 30 | 0.285 | 0.212 | 0.196–0.746 |
| tokei | 10 | 0.299 | 0.221 | 0.204–0.691 |
| upstream gocloc | 10 | 1.731 | 1.435 | 1.262–3.317 |
| cloc | 3 | 28.577 | 29.016 | 27.025–29.690 |

Background desktop tasks were active during measurement, and timings varied as shown above. Counts reflect each tool's default behavior; cloc reported 2–3 file timeouts per run.

### optimized gocloc

```
$ time gocloc .
-------------------------------------------------------------------------------
Language                     files          blank        comment           code
-------------------------------------------------------------------------------
Go                           11746         277212         451413        2574521
Plain Text                    1476          14813              0         233179
Assembly                       659          16207          24208         149514
HTML                            15           2098            180          19987
JSON                            41            124              0          14186
YAML                            59            326            331           6126
C                              117            976            854           5601
Markdown                        68           1440             36           4837
BASH                            31            362            592           2780
JavaScript                       9            301            331           1706
Perl                             9            163            163           1058
C Header                        28            154            371            801
Python                           2            155            196            416
CSS                              4              5             13            360
Batch                            5             35             57            120
Plan9 Shell                      4             23             44            102
Dockerfile                       2             15             18             61
Bourne Shell                     4             23             18             49
C++                              2             11             14             24
Makefile                         6              9             34             22
Objective-C                      2              3              3             15
FORTRAN Modern                   2              1              3              8
Awk                              1              1              6              7
-------------------------------------------------------------------------------
TOTAL                        14292         314457         478885        3015480
-------------------------------------------------------------------------------
gocloc .  0.288s user 0.804s system 448.9% cpu 0.285 total
```

### tokei

```
$ time tokei .
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Language              Files        Lines         Code     Comments       Blanks
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 AWK                       1           14            7            6            1
 Alex                      2          117          101            0           16
 GNU Style Assembly      659       189929       154061        19670        16198
 Autoconf                  9          283          274            0            9
 BASH                     16          876          502          264          110
 Batch                     5          212          120           57           35
 C                       117         7431         5657          856          918
 C Header                 28         1326          801          371          154
 C++                       2           49           24           14           11
 CSS                       4          378          360           13            5
 Dockerfile                2           94           61           18           15
 Forge Config              3            3            3            0            0
 FORTRAN Modern            2           12            8            3            1
 Go                    11728      3302392      2575946       455985       270461
 JavaScript                9         2338         1705          332          301
 JSON                     38        14287        14163            0          124
 Makefile                  6           65           22           34            9
 Objective-C               2           21           15            3            3
 Perl                      9         1384         1049          172          163
 Python                    2          767          482          142          143
 Shell                    19         2948         2371          380          197
 Snakemake                28        21223        19878            0         1345
 Templ                     8           20           20            0            0
 Plain Text             1478       248043            0       233221        14822
 YAML                     59         6783         6136          331          316
─────────────────────────────────────────────────────────────────────────────────
 HTML                     15        12849        12465           37          347
 |- CSS                    3         2060         1841           10          209
 |- HTML                   1          219          212            0            7
 |- JavaScript             8         7143         7026           91           26
 (Total)                            22271        21544          138          589
─────────────────────────────────────────────────────────────────────────────────
 Markdown                 68         5751            0         4311         1440
 |- Go                     2          562          559            3            0
 (Total)                             6313          559         4314         1440
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Total                 14319      3829579      2805869       716324       307386
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
tokei .  0.555s user 0.665s system 478.5% cpu 0.299 total
```

### upstream gocloc (https://github.com/hhatto/gocloc)

```
$ time gocloc-upstream .
-------------------------------------------------------------------------------
Language                     files          blank        comment           code
-------------------------------------------------------------------------------
Go                           11534         275074         485096        2500003
Plain Text                    1468          14795              0         233087
Assembly                       656          16198          24181         149512
HTML                            15           2098            180          19987
JSON                            40            124              0          14186
YAML                            59            326            361           6096
C                              113            968            846           5546
Markdown                        62           1436             36           4830
BASH                            31            362           1144           2228
JavaScript                       9            301            332           1705
C Header                        27            153            368            798
Perl                             9            163            581            640
Python                           2            155            187            425
CSS                              4              5             13            360
Batch                            5             35              0            177
Plan9 Shell                      4             23             47             99
Dockerfile                       2             15             18             61
Bourne Shell                     4             23             18             49
C++                              2             11             14             24
Makefile                         6              9             34             22
Objective-C                      2              3              3             15
FORTRAN Modern                   2              1              3              8
Awk                              1              1              6              7
-------------------------------------------------------------------------------
TOTAL                        14057         312279         513468        2939865
-------------------------------------------------------------------------------
gocloc-upstream .  0.633s user 0.901s system 94.1% cpu 1.731 total
```

### cloc

```
$ time cloc .
3 errors:
Line count, exceeded timeout:  ./src/cmd/dist/build.go
Line count, exceeded timeout:  ./src/vendor/golang.org/x/net/idna/tables15.0.0.go
Line count, exceeded timeout:  ./src/vendor/golang.org/x/net/idna/tables17.0.0.go
github.com/AlDanial/cloc v 2.04  T=33.35 s (422.5 files/s, 113621.7 lines/s)
-----------------------------------------------------------------------------------
Language                         files          blank        comment           code
-----------------------------------------------------------------------------------
Go                               11519         274964         460948        2523575
Text                              1473          14804              0         233134
Assembly                           656          16184          24182         149525
HTML                                15           2098             50          20117
Snakemake                           28           2202              0          19021
JSON                                40            124              0          14186
YAML                                68            361            364           6564
C                                  113            968            845           5547
Markdown                            64           1443             36           4840
CSV                                  1              0              0           2118
Bourne Shell                        19            273            914           1761
JavaScript                           9            301            331           1706
Perl                                 9            163            163           1058
C/C++ Header                        27            153            368            798
Bourne Again Shell                  16            112            248            516
Python                               2            155            187            425
CSS                                  4              5             13            360
Windows Resource File                4             23              0            146
DOS Batch                            5             35             57            120
Logos                                2             16              0            101
Dockerfile                           2             15             18             61
diff                                 1              7             20             36
C++                                  2             11             14             24
make                                 6              9             34             22
Objective-C                          1              2              3             11
Fortran 90                           2              1              3              8
awk                                  1              1              6              7
MATLAB                               1              1              0              4
-----------------------------------------------------------------------------------
SUM:                             14090         314431         488804        2985791
-----------------------------------------------------------------------------------

One or more files took longer to process than expected.
Try rerunning without timeout guards by adding  --timeout 0
to your command line arguments.  See the documentation on
the --timeout switch for more information.
cloc .  22.923s user 1.960s system 86.7% cpu 28.577 total
```

## Usage

```sh
# Count one or more directories
gocloc .
gocloc src tests

# Exclude dependency and build directories
gocloc --exclude '{dist,node_modules,target}/' .

# Filter languages or exclude extensions
gocloc -l Go,Python .
gocloc -e txt,md .

# Report each file, sorted by code lines
gocloc -f -s code .

# Export JSON outside the scanned directory
gocloc -o json . > ../counts.json

# Count identical contents only once (off by default)
gocloc --dedup .

# List supported languages / show all options
gocloc -L
gocloc -h
```

Workers are selected automatically; use `-w 1` through `-w 64` to override.
`-o` supports `default`, `json`, `markdown`, `cloc-xml`, and `sloccount`.
Short and long options can be mixed; see `gocloc -h` for the full list.

All file and directory filters use globs. Use repeatable `--exclude` flags to exclude files or directories;
any matching rule excludes the entry:

```sh
gocloc --exclude 'target/' --exclude '*.min.js' .
gocloc --exclude 'src/generated/' --exclude '**/vendor/' .
gocloc --match '*.go' --not-match '*_test.go' .
gocloc --match-d 'src/' --not-match-d '{dist,node_modules,target}/' .
gocloc --match 'src/**/*.{go,py}' .
```

| Exclusion pattern | Meaning |
| --- | --- |
| `target/` | A directory named exactly target at any depth, including its contents; keeps gcc.target |
| `{dist,node_modules,target}/` | Any of these three complete directory names at any depth |
| `*.min.js` | A matching file or directory name at any depth |
| `/target/` | A target directory directly inside each scan root |
| `src/generated/` | This exact directory path relative to each scan root |
| `**/generated/` | A generated directory at any depth, including directly inside the root |

Quote patterns to prevent shell expansion. Matching is case-sensitive: `*` does not cross directories,
while `**` matches zero or more directory levels. A trailing `/` restricts a rule to directories;
without it, `--exclude` can also match files. Excluded directories are pruned before visiting their contents.
Each input has its own matching root. Unanchored name rules also check the input directory's own name.
Explicit file inputs use their parent directory as the matching root and check that parent's name rules too.
Ancestors outside the matching root are ignored. Use `/` in patterns on every platform;
backslashes escape glob characters. Invalid patterns fail before scanning.
Exclusions take precedence over inclusions. `!` does not re-include entries,
and filters do not automatically read `.gitignore` files.

`--match` and `--not-match` filter files only; a trailing `/` is rejected.
`--match-d` and `--not-match-d` filter directories and all their descendants; a trailing `/` is optional.
Directory inclusion keeps traversing unmatched parents to find nested matches.
When both file and directory inclusion filters are set, files must satisfy both.
These four flags each accept one glob; use `{a,b}` for alternatives. Empty values disable the filter.
`--exclude` is repeatable and rejects empty values.

This replaces the former regex syntax. For example, `--match '\.go$'` becomes `--match '*.go'`,
and `--not-match-d 'dist|node_modules|target'` becomes `--not-match-d '{dist,node_modules,target}/'`.
`|`, `^` and `$` are ordinary characters in globs. `--fullpath` has been removed;
use a relative path glob such as `--match 'src/**'` directly.
`-e/--exclude-ext` continues to filter extensions.
Keep generated reports outside the scanned tree or explicitly exclude them. `>` overwrites the destination.
Older releases enabled deduplication by default; use `--dedup` to retain that behavior.

`--debug` writes analysis activity to stderr without disabling parallel workers.
Line records include the file path and line number; records from different files may interleave.
With `--dedup`, a duplicate can produce analysis logs before a `[SKIP]` record excludes it from the totals.

### Go library

Use the built-in languages without constructing a processor:

```go
result, err := gocloc.Analyze([]string{"."}, nil)
```

Pass language lists and glob patterns directly when filtering:

```go
result, err := gocloc.Analyze([]string{"src", "tests"}, &gocloc.Options{
    IncludeLangs: []string{"Go", "Python"},
    ExcludeExts:  []string{"txt"},
    Exclude:      []string{"{dist,node_modules,target}/", "*_test.go"},
    Dedup:        true, // Optional: count identical content only once.
})
```

Check `err` before using `result`. `nil` and `&gocloc.Options{}` both default to no
deduplication and `runtime.GOMAXPROCS(0)` workers, capped at 64. `Workers: 1` selects
one worker; explicit values outside 1–64 are rejected, while zero selects automatically.
An empty path list returns an empty result, not a scan of the current directory.

Filters are prepared once per call. Invalid options return `*gocloc.OptionError` before
scanning. `Options.Exclude`, `Match`, `NotMatch`, `MatchDir` and `NotMatchDir` use the same
glob rules as the CLI; errors identify the corresponding field.
`Processor.Analyze` supports the same fields on `ClocOptions`.
`AnalyzeFile` and `AnalyzeReader` do not apply path filters.
The former `ClocOptions.ReMatch`, `ReNotMatch`, `ReMatchDir` and `ReNotMatchDir` fields
have been replaced by string glob fields without the `Re` prefix. `Fullpath` has been removed
from both option types; patterns containing `/` already match paths relative to each input root.
Like the CLI, `ExcludeExts` excludes the detected language associated
with each extension, including aliases. `IncludeLangs` ignores unknown names; if none of
the supplied names are recognized, no language inclusion filter is applied.

`NewProcessor` and `ClocOptions` remain available for custom language rules and existing
callers, with their defaults unchanged: `Workers <= 1` uses one worker, `ClocOptions{}`
enables deduplication, and `NewClocOptions()` disables it.
See [examples/files](examples/files/main.go) and [examples/languages](examples/languages/main.go)
for complete programs.

### Library callbacks

`Analyze` and `Processor.Analyze` run `OnCode`, `OnComment`, and `OnBlank` in workers. With multiple
workers, callbacks from different files may run concurrently; callers must protect shared state.
For example, pass these options to `gocloc.Analyze` (the counter uses `sync/atomic`):

```go
var codeLines atomic.Int64
options := &gocloc.Options{
    Dedup:  true, // Optional: count identical content only once.
    OnCode: func(string) { codeLines.Add(1) },
}
```

Calls within one file follow line order; order across files is unspecified. `Workers` limits
concurrent callback execution as well as file analysis. Set `Workers = 1` for non-overlapping
callbacks within one `Analyze` call; this does not put callbacks on the caller's goroutine.
Callbacks must return for `Analyze` to finish, and `Analyze` waits for all callbacks before
returning, including on errors. `AnalyzeFile` and `AnalyzeReader` remain synchronous.
This changes the earlier library contract that kept processor callbacks on the caller's goroutine.
Existing callers must synchronize shared state or explicitly use one worker.

Callbacks follow the deduplication setting. Without deduplication, every copy triggers callbacks
during analysis; later read errors cannot undo calls already made. With deduplication, workers
read each source file once while hashing, counting, and recording callback events. Results are
deduplicated in discovery order, then only the first retained copy's events are replayed in
callback workers. Failed source reads and discarded copies do not trigger callbacks. Debug
logs still show actual analysis, including discarded copies.

Deferred events share a 16 MiB in-memory buffer-capacity budget per scan (not a total process
memory limit). Large buffers spill to private files in the OS temporary directory and are
removed after replay or exclusion. Queued spools do not hold open file descriptors. Parser
buffers, individual callback strings, and memory retained by callers are outside this budget.
Spool write failures exclude the affected file with a diagnostic; replay or cleanup failures
are returned after workers finish. Already delivered callbacks cannot be rolled back.

## Jenkins

Install the [SLOCCount plugin](https://plugins.jenkins.io/sloccount/) on Jenkins and put `gocloc` on the agent's `PATH`.
Add this stage to a Declarative Pipeline running on a Unix agent with the source checked out:

```groovy
stage('Count lines') {
    steps {
        sh 'mkdir -p reports && gocloc -f -o sloccount --exclude=reports/ . > reports/sloccount.sc'
        sloccountPublish pattern: 'reports/sloccount.sc', encoding: 'UTF-8'
    }
}
```

The report directory is excluded from counting. For Freestyle jobs, run the same shell command and configure
**Publish SLOCCount analysis results** with the pattern `reports/sloccount.sc`.

## License

MIT
