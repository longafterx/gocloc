package main

import (
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/rustyllh/gocloc"
	"github.com/spf13/cobra"
)

// CmdOptions is gocloc command options.
type CmdOptions struct {
	ByFile         bool
	SortTag        string
	OutputType     string
	ExcludeExt     string
	Exclude        []string
	IncludeLang    string
	Match          string
	NotMatch       string
	MatchDir       string
	NotMatchDir    string
	Fullpath       bool
	Debug          bool
	Workers        *int
	Dedup          bool
	SkipDuplicated bool
	ShowLang       bool
	ShowVersion    bool
}

func configureWorkerOptions(opts CmdOptions, analysisOpts *gocloc.Options) error {
	if opts.Workers == nil {
		analysisOpts.Workers = 0
		return nil
	}
	if *opts.Workers < 0 {
		return fmt.Errorf("--workers must be greater than or equal to zero")
	}
	if *opts.Workers == 0 {
		return fmt.Errorf("--workers must be between 1 and %d", gocloc.MaxWorkers)
	}
	if *opts.Workers > gocloc.MaxWorkers {
		return fmt.Errorf("--workers must not exceed %d", gocloc.MaxWorkers)
	}
	analysisOpts.Workers = *opts.Workers
	return nil
}

func newRootCommand() *cobra.Command {
	var opts CmdOptions
	var workerCount int
	command := &cobra.Command{
		Use:   "gocloc [OPTIONS] PATH[...]",
		Short: "A fast, parallel source code line counter",
		Example: `  # Count the current directory
  gocloc .

  # Count multiple directories
  gocloc src tests

  # Exclude dependency and build directories by name
  gocloc --exclude '{dist,node_modules,target}/' .

  # Count only Go and Python files
  gocloc -l Go,Python .

  # Report each file, sorted by code lines
  gocloc -f -s code .

  # Export JSON outside the scanned directory
  gocloc -o json . > ../counts.json

  # Count identical contents only once
  gocloc --dedup .`,
		Args:               cobra.ArbitraryArgs,
		SilenceErrors:      true,
		SilenceUsage:       true,
		DisableSuggestions: true,
		CompletionOptions:  cobra.CompletionOptions{DisableDefaultCmd: true},
		RunE: func(cmd *cobra.Command, paths []string) error {
			if cmd.Flags().Changed("workers") {
				opts.Workers = &workerCount
			}
			if cmd.Flags().Changed("skip-duplicated") {
				opts.Dedup = !opts.SkipDuplicated
			}
			analysisOpts := &gocloc.Options{}
			if err := configureWorkerOptions(opts, analysisOpts); err != nil {
				return err
			}
			if opts.ShowVersion {
				info, _ := debug.ReadBuildInfo()
				_, err := fmt.Fprintln(cmd.OutOrStdout(), versionString(Version, GitCommit, info))
				return err
			}
			if opts.ShowLang {
				languages := gocloc.NewDefinedLanguages()
				_, err := fmt.Fprintln(cmd.OutOrStdout(), languages.GetFormattedString())
				return err
			}
			if len(paths) == 0 {
				return cmd.Help()
			}
			analysisOpts.Diagnostics = cmd.ErrOrStderr()
			return runAnalysis(
				paths,
				opts,
				analysisOpts,
				cmd.OutOrStdout(),
			)
		},
	}
	command.DisableFlagsInUseLine = true
	command.SetUsageTemplate(`Usage:
  {{.UseLine}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}
`)
	flags := command.Flags()
	flags.SetInterspersed(true)
	flags.BoolVarP(
		&opts.ByFile,
		"by-file",
		"f",
		false,
		"report results for every encountered source file",
	)
	opts.SortTag = "code"
	flags.VarP(
		(*sortTagValue)(&opts.SortTag),
		"sort",
		"s",
		"sort based on a certain column [name,files,blank,comment,code]",
	)
	flags.StringVarP(
		&opts.OutputType,
		"output-type",
		"o",
		OutputTypeDefault,
		"output type [values: default,markdown,cloc-xml,sloccount,json]",
	)
	flags.StringVarP(
		&opts.ExcludeExt,
		"exclude-ext",
		"e",
		"",
		"exclude file name extensions (separated commas)",
	)
	flags.StringArrayVar(
		&opts.Exclude,
		"exclude",
		[]string{},
		"exclude files or directories matching a glob (repeatable; trailing / matches directories only)",
	)
	flags.StringVarP(
		&opts.IncludeLang,
		"include-lang",
		"l",
		"",
		"include language names (separated commas)",
	)
	flags.StringVar(
		&opts.Match,
		"match",
		"",
		"include file name (regex)",
	)
	flags.StringVar(
		&opts.NotMatch,
		"not-match",
		"",
		"exclude file name (regex)",
	)
	flags.StringVar(
		&opts.MatchDir,
		"match-d",
		"",
		"include directory paths matching a regex (substring match)",
	)
	flags.StringVar(
		&opts.NotMatchDir,
		"not-match-d",
		"",
		"exclude directory paths matching a regex (substring match)",
	)
	flags.BoolVar(
		&opts.Fullpath,
		"fullpath",
		false,
		"apply match/not-match options to full file paths instead of base names",
	)
	flags.BoolVar(
		&opts.Debug,
		"debug",
		false,
		"log analysis activity to stderr (files may interleave)",
	)
	flags.VarP(
		(*workerCountValue)(&workerCount),
		"workers",
		"w",
		"number of file analysis workers (1-64; default: automatic)",
	)
	flags.BoolVar(
		&opts.Dedup,
		"dedup",
		false,
		"count files with identical contents only once",
	)
	flags.BoolVar(
		&opts.SkipDuplicated,
		"skip-duplicated",
		false,
		"compatibility option: skip duplicate detection; =false enables it (prefer --dedup)",
	)
	command.MarkFlagsMutuallyExclusive("dedup", "skip-duplicated")
	flags.BoolVarP(
		&opts.ShowLang,
		"show-lang",
		"L",
		false,
		"print about all languages and extensions",
	)
	flags.BoolVarP(
		&opts.ShowVersion,
		"version",
		"V",
		false,
		"print version info",
	)
	return command
}

// go-flags parsed integers in base ten; pflag's IntVar instead uses base zero.
// Keep values such as --workers=08 decimal and reject hexadecimal notation.
type workerCountValue int

func (v *workerCountValue) String() string { return strconv.Itoa(int(*v)) }
func (v *workerCountValue) Type() string   { return "int" }

func (v *workerCountValue) Set(value string) error {
	count, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("invalid worker count: %w", err)
	}
	*v = workerCountValue(count)
	return nil
}

// Validate each occurrence during parsing, including before informational exits.
type sortTagValue string

func (v *sortTagValue) String() string { return string(*v) }
func (v *sortTagValue) Type() string   { return "string" }

func (v *sortTagValue) Set(value string) error {
	switch value {
	case "name", "files", "blank", "comment", "code":
		*v = sortTagValue(value)
		return nil
	default:
		return fmt.Errorf("must be one of name, files, blank, comment or code")
	}
}

func runAnalysis(paths []string, opts CmdOptions, analysisOpts *gocloc.Options, out io.Writer) error {
	// check sort tag option with other options
	if opts.ByFile && opts.SortTag == "files" {
		return fmt.Errorf("`--sort files` option cannot be used in conjunction with the `--by-file` option")
	}
	analysisOpts.ExcludeExts = strings.Split(opts.ExcludeExt, ",")
	analysisOpts.Exclude = opts.Exclude
	analysisOpts.IncludeLangs = strings.Split(opts.IncludeLang, ",")
	analysisOpts.Match = opts.Match
	analysisOpts.NotMatch = opts.NotMatch
	analysisOpts.MatchDir = opts.MatchDir
	analysisOpts.NotMatchDir = opts.NotMatchDir
	analysisOpts.Debug = opts.Debug
	analysisOpts.Dedup = opts.Dedup
	analysisOpts.Fullpath = opts.Fullpath

	result, err := gocloc.Analyze(paths, analysisOpts)
	if err != nil {
		var optionErr *gocloc.OptionError
		if errors.As(err, &optionErr) {
			flag := map[string]string{
				"Exclude": "exclude",
				"Match":   "match", "NotMatch": "not-match",
				"MatchDir": "match-d", "NotMatchDir": "not-match-d",
			}[optionErr.Field]
			if flag != "" {
				return fmt.Errorf("invalid --%s: %w", flag, optionErr.Err)
			}
		}
		return fmt.Errorf("fail gocloc analyze: %w", err)
	}

	return newOutputBuilder(result, &opts, out).WriteResult()
}
