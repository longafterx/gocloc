# gocloc

[English](README.md) | [简体中文](README.zh-CN.md)

[![GoDoc](https://godoc.org/github.com/rustyllh/gocloc?status.svg)](https://godoc.org/github.com/rustyllh/gocloc)
[![ci](https://github.com/rustyllh/gocloc/workflows/Go/badge.svg)](https://github.com/rustyllh/gocloc/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/rustyllh/gocloc)](https://goreportcard.com/report/github.com/rustyllh/gocloc)
[![Docker Pulls](https://img.shields.io/docker/pulls/rustyllh/gocloc)](https://hub.docker.com/r/rustyllh/gocloc)
[![Docker Image Size](https://img.shields.io/docker/image-size/rustyllh/gocloc/latest)](https://hub.docker.com/r/rustyllh/gocloc)

一个用 Go 编写的高性能、并行源码行数统计工具，基于 [hhatto/gocloc](https://github.com/hhatto/gocloc)。
灵感来自 [tokei](https://github.com/Aaronepower/tokei)，在我们最新的 Go 源码仓库基准测试中，平均耗时更低。

## 安装

### macOS / Linux

安装最新稳定版，无需 Go 环境，自动校验 SHA-256：

```sh
curl --proto '=https' --tlsv1.2 -fsSL https://raw.githubusercontent.com/rustyllh/gocloc/master/install.sh | sh
```

需要 `curl`、`tar` 以及 `sha256sum` 或 `shasum`。默认安装到 `$HOME/.local/bin`，请按需将其加入 `PATH`。

### Windows

在 PowerShell 5.1+ 中执行：

```powershell
& ([scriptblock]::Create((Invoke-RestMethod https://raw.githubusercontent.com/rustyllh/gocloc/master/install.ps1 -ErrorAction Stop)))
```

默认安装到 `%LOCALAPPDATA%\gocloc\bin`，请将其加入用户 `Path`。支持 x86-64 和 i386，不支持 Windows ARM64。

这两种方式会执行下载的脚本，可先查看 [install.sh](install.sh) 或 [install.ps1](install.ps1)。
再次运行安装命令即可升级，使用 `gocloc --version` 确认版本。
也可以从 [GitHub Releases](https://github.com/rustyllh/gocloc/releases) 手动下载二进制文件及校验文件。

### Docker

以只读方式挂载并统计当前目录：

```sh
docker run --rm --read-only --mount "type=bind,source=$(pwd),target=/workdir,readonly" rustyllh/gocloc:latest .
```

支持 Linux `amd64` 和 `arm64`，自动选择匹配的架构。`:vX.Y.Z` 用于固定版本，`:latest` 跟随稳定版更新。
命令行选项放在镜像名称之后，例如 `--dedup -o json .`。

## 性能

* 测试日期：2026-10-15
* CPU Apple M3 8 核 / 16 GiB / macOS 27.0.1 arm64
* 优化版 gocloc：[38bad47](https://github.com/rustyllh/gocloc/commit/38bad4752b755918759c2d2b4703b458125b01f4)（源码构建）
* [tokei 14.0.0](https://github.com/XAMPPRocky/tokei/releases/tag/v14.0.0)，编译时启用了 JSON 序列化支持
* 上游 gocloc：[679b457](https://github.com/hhatto/gocloc/commit/679b457182dcf852d90e52f132403d6b7de0e33d)（源码构建；[发行版](https://github.com/hhatto/gocloc/releases)）
* [cloc 2.04](https://github.com/AlDanial/cloc/releases/tag/v2.04)
* 测试目标：[golang/go](https://github.com/golang/go) 仓库，`master` 分支，提交 [6b3800e](https://github.com/golang/go/commit/6b3800e1dd90925b4184acd6391f68bffd16b6a5)

两个 gocloc 版本均使用 Go 1.27.2 编译。所有工具均以默认选项扫描上述仓库，不覆盖运行时或线程配置。每个工具先预热一次，计时运行的输出重定向到 `/dev/null`。三个快速工具交替顺序运行，随后测量 cloc，保留全部样本。下方命令输出取自预热运行，`time` 行显示计时运行的平均值。

| 工具 | 运行次数 | 平均耗时（秒） | 中位数（秒） | 耗时范围（秒） |
| --- | ---: | ---: | ---: | ---: |
| 优化版 gocloc | 30 | 0.285 | 0.212 | 0.196–0.746 |
| tokei | 10 | 0.299 | 0.221 | 0.204–0.691 |
| 上游 gocloc | 10 | 1.731 | 1.435 | 1.262–3.317 |
| cloc | 3 | 28.577 | 29.016 | 27.025–29.690 |

测量期间有桌面后台任务运行，耗时波动如上表所示。统计结果遵循各工具的默认口径；cloc 每次运行有 2–3 个文件超时。

### 优化版 gocloc

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

### 上游 gocloc（https://github.com/hhatto/gocloc）

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

## 使用

```sh
# 统计一个或多个目录
gocloc .
gocloc src tests

# 排除依赖和构建目录
gocloc --exclude '{dist,node_modules,target}/' .

# 筛选语言或排除扩展名
gocloc -l Go,Python .
gocloc -e txt,md .

# 按文件展示，并按代码行数排序
gocloc -f -s code .

# 将 JSON 导出到扫描目录之外
gocloc -o json . > ../counts.json

# 内容相同的文件只统计一次（默认关闭）
gocloc --dedup .

# 列出支持的语言 / 查看全部选项
gocloc -L
gocloc -h
```

默认自动选择 worker 数量，也可使用 `-w 1` 至 `-w 64` 手动指定。
`-o` 支持 `default`、`json`、`markdown`、`cloc-xml` 和 `sloccount`。
长选项与短选项可以混用，完整列表见 `gocloc -h`。

文件和目录过滤统一使用 glob。使用可重复的 `--exclude` 排除文件或目录，多个规则取并集：

```sh
gocloc --exclude 'target/' --exclude '*.min.js' .
gocloc --exclude 'src/generated/' --exclude '**/vendor/' .
gocloc --match '*.go' --not-match '*_test.go' .
gocloc --match-d 'src/' --not-match-d '{dist,node_modules,target}/' .
gocloc --match 'src/**/*.{go,py}' .
```

| 排除模式 | 含义 |
| --- | --- |
| `target/` | 任意层级下完整名字为 target 的目录及其内容，保留 gcc.target |
| `{dist,node_modules,target}/` | 任意层级下这三个完整目录名 |
| `*.min.js` | 任意层级下匹配的文件名或目录名 |
| `/target/` | 每个扫描根目录直接包含的 target 目录 |
| `src/generated/` | 相对于每个扫描根目录的完整目录路径 |
| `**/generated/` | 任意层级的 generated 目录，包含根目录直接包含的同名目录 |

模式必须加引号，避免 shell 提前展开。匹配区分大小写，`*` 不跨目录，`**` 可以匹配零层或多层目录；
末尾 `/` 将规则限定为目录，没有末尾 `/` 时 `--exclude` 也可匹配文件。排除目录后直接停止遍历其内容。
多个输入分别作为匹配根目录；不锚定的名字规则也检查输入目录自身的名字。
显式文件使用其父目录作为匹配根目录，并检查父目录自身是否被名字规则排除。
根目录之外的祖先不参与 glob 匹配。模式在所有平台统一使用 `/`，反斜杠用于转义。
非法模式在扫描前报错。排除规则优先于包含规则；`!` 不表示重新包含，也不会自动读取 `.gitignore`。

`--match`、`--not-match` 只过滤文件，不允许以 `/` 结尾。
`--match-d`、`--not-match-d` 过滤目录及其所有后代，末尾 `/` 可省略。
包含目录时会继续遍历尚未匹配的父目录，以便找到深层匹配。
同时设置文件和目录包含规则时，文件必须满足两者。
这四个参数分别接受一个 glob，可用 `{a,b}` 表达多个备选；空值表示不启用该过滤。
`--exclude` 可重复传入，不允许空值。

这些参数不再支持原来的正则语法。例如，`--match '\.go$'` 改为 `--match '*.go'`，
`--not-match-d 'dist|node_modules|target'` 改为 `--not-match-d '{dist,node_modules,target}/'`。
`|`、`^`、`$` 在 glob 中是普通字符。已移除 `--fullpath`，直接使用 `--match 'src/**'` 等相对路径模式。
`-e/--exclude-ext` 继续用于扩展名过滤。
生成的报告应放在扫描目录之外，或显式排除。`>` 会覆盖目标文件。
旧版本默认开启去重；如需保留旧版统计行为，请添加 `--dedup`。

`--debug` 将实际分析过程写入 stderr，不会关闭 worker 并发。
逐行日志包含文件路径和行号，不同文件的日志允许交错。
开启 `--dedup` 时，重复文件可能先输出分析日志，再通过 `[SKIP]` 日志标记为不计入统计。

### Go 库调用

使用内置语言规则时，无需手动创建处理器：

```go
result, err := gocloc.Analyze([]string{"."}, nil)
```

需要过滤时，直接传入语言列表和 glob 模式：

```go
result, err := gocloc.Analyze([]string{"src", "tests"}, &gocloc.Options{
    IncludeLangs: []string{"Go", "Python"},
    ExcludeExts:  []string{"txt"},
    Exclude:      []string{"{dist,node_modules,target}/", "*_test.go"},
    Dedup:        true, // 可选：内容相同的文件只统计一次。
})
```

使用 `result` 前应检查 `err`。`nil` 和 `&gocloc.Options{}` 均默认不去重，
worker 数量为 `runtime.GOMAXPROCS(0)`，最多 64 个。`Workers: 1` 使用单 worker；
显式指定的数量必须在 1～64 之间，零值表示自动选择。
路径列表为空时返回空结果，不会自动扫描当前目录。

过滤配置每次调用只准备一次。配置不合法时，在扫描前返回 `*gocloc.OptionError`。
`Options.Exclude`、`Match`、`NotMatch`、`MatchDir`、`NotMatchDir` 与 CLI 使用相同的 glob 规则，
非法 glob 的错误会标明对应字段。`Processor.Analyze` 同样支持 `ClocOptions` 中的这些字段。
`AnalyzeFile` 和 `AnalyzeReader` 不执行路径过滤。
原 `ClocOptions.ReMatch`、`ReNotMatch`、`ReMatchDir`、`ReNotMatchDir` 已替换为去掉 `Re` 前缀的字符串 glob 字段。
两种选项类型都已移除 `Fullpath`；包含 `/` 的模式自动相对各输入根目录匹配路径。
与 CLI 一致，`ExcludeExts` 排除的是扩展名对应的识别语言，包括该语言的其他扩展名。
`IncludeLangs` 会忽略未知语言名；如果传入的名称都无法识别，则不限制统计语言。

`NewProcessor` 和 `ClocOptions` 继续供自定义语言规则和已有调用方使用，默认行为不变：
`Workers <= 1` 使用单 worker，`ClocOptions{}` 启用去重，`NewClocOptions()` 不去重。
完整程序见 [examples/files](examples/files/main.go) 和 [examples/languages](examples/languages/main.go)。

### 库回调

`Analyze` 和 `Processor.Analyze` 在 worker 中执行 `OnCode`、`OnComment` 和 `OnBlank`。
使用多个 worker 时，不同文件的回调可能并发执行，调用方需要保护共享状态。
例如，将以下配置传给 `gocloc.Analyze`（计数器使用 `sync/atomic`）：

```go
var codeLines atomic.Int64
options := &gocloc.Options{
    Dedup:  true, // 可选：内容相同的文件只统计一次。
    OnCode: func(string) { codeLines.Add(1) },
}
```

同一文件内按行顺序调用，文件之间不保证调用顺序。`Workers` 同时限制文件分析和回调执行的并发度。
设置 `Workers = 1` 可保证单次 `Analyze` 内回调不重叠，但不保证运行在调用方的 goroutine 上。
回调必须返回，`Analyze` 才能结束；即使发生错误，`Analyze` 也会等待全部回调执行完毕再返回。
`AnalyzeFile` 和 `AnalyzeReader` 仍然同步调用回调。
这改变了旧版库在调用方 goroutine 中执行处理器回调的约定，已有调用方需要保护共享状态或显式使用单 worker。

回调遵循去重设置。不去重时，每份文件都会在分析过程中触发回调，后续读取失败不能撤销已经执行的调用。
开启去重时，worker 单次读取源文件，同时计算哈希、统计行数并记录回调事件；按发现顺序去重后，
只有首次保留副本的事件会交给回调 worker 执行。源文件读取失败或被去重排除时，不触发回调。
debug 日志仍然反映实际分析过程，包括被排除的副本。

每次扫描的延迟事件缓存共享 16 MiB 内存缓冲容量预算（不是进程总内存上限）。
超过额度的缓存写入系统临时目录的私有文件，回放或排除后删除；排队中的临时文件不占用打开的文件描述符。
解析缓冲、单条回调字符串以及调用方保留的内存不计入该预算。
临时缓存写入失败时，给出诊断并排除相应文件；回放或清理失败时，等待 worker 完成后返回错误。
已经执行的回调无法回滚。

## Jenkins

在 Jenkins 中安装 [SLOCCount 插件](https://plugins.jenkins.io/sloccount/)，并确保执行节点的 `PATH` 中有 `gocloc`。
在已检出源码、使用 Unix 节点的声明式 Pipeline 中添加以下阶段：

```groovy
stage('Count lines') {
    steps {
        sh 'mkdir -p reports && gocloc -f -o sloccount --exclude=reports/ . > reports/sloccount.sc'
        sloccountPublish pattern: 'reports/sloccount.sc', encoding: 'UTF-8'
    }
}
```

报告目录已从统计中排除。对于 Freestyle 任务，执行同一条 shell 命令，再添加
**Publish SLOCCount analysis results** 构建后操作，将文件匹配模式设为 `reports/sloccount.sc`。

## 许可证

MIT
