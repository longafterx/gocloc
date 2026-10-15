package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/longafterx/gocloc"
)

// OutputTypeDefault is cloc's text output format for --output-type option
const OutputTypeDefault string = "default"

// OutputTypeClocXML is Cloc's XML output format for --output-type option
const OutputTypeClocXML string = "cloc-xml"

// OutputTypeSloccount is Sloccount output format for --output-type option
const OutputTypeSloccount string = "sloccount"

// OutputTypeJSON is JSON output format for --output-type option
const OutputTypeJSON string = "json"

// OutputTypeMarkdown is Markdown output format for --output-type option
const OutputTypeMarkdown string = "markdown"

const (
	fileHeader             string = "File"
	languageHeader         string = "Language"
	commonHeader           string = "files          blank        comment           code"
	defaultOutputSeparator string = "-------------------------------------------------------------------------" +
		"-------------------------------------------------------------------------" +
		"-------------------------------------------------------------------------"
)

type outputBuilder struct {
	opts   *CmdOptions
	result *gocloc.Result
	out    io.Writer
	rowLen int
	err    error
}

func newOutputBuilder(result *gocloc.Result, opts *CmdOptions, out io.Writer) *outputBuilder {
	rowLen := 79
	if opts.ByFile {
		rowLen = result.MaxPathLength + len(commonHeader) + 2
	}
	return &outputBuilder{opts: opts, result: result, out: out, rowLen: rowLen}
}

// Formatting stops after the first failure, which WriteResult returns to Cobra.
func (o *outputBuilder) Write(data []byte) (int, error) {
	if o.err != nil {
		return 0, o.err
	}
	n, err := o.out.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	o.err = err
	return n, err
}

func (o *outputBuilder) printf(format string, args ...any) {
	if o.err == nil {
		_, o.err = fmt.Fprintf(o, format, args...)
	}
}

func (o *outputBuilder) println(args ...any) {
	if o.err == nil {
		_, o.err = fmt.Fprintln(o, args...)
	}
}

func (o *outputBuilder) writeJSON(value any) {
	if o.err != nil {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		o.err = err
		return
	}
	_, o.err = o.Write(data)
}

func (o *outputBuilder) WriteHeader() {
	maxPathLen := o.result.MaxPathLength
	headerLen := 28
	header := languageHeader

	if o.opts.ByFile {
		headerLen = maxPathLen + 1
		header = fileHeader
	}

	if o.opts.OutputType == OutputTypeDefault {
		o.printf("%.[2]*[1]s\n", defaultOutputSeparator, o.rowLen)
		o.printf("%-[2]*[1]s %[3]s\n", header, headerLen, commonHeader)
		o.printf("%.[2]*[1]s\n", defaultOutputSeparator, o.rowLen)
	}

	if o.opts.OutputType == OutputTypeMarkdown {
		allHeaders := fmt.Sprintf("%s%s%s", header, strings.Repeat(" ", headerLen), commonHeader)
		headerString := "| " + gocloc.InsertPipesInTheMiddle(allHeaders)
		o.println(headerString)

		for i := 0; i < len(headerString); i++ {
			if headerString[i] == '|' {
				o.printf("|")
			} else {
				if i == 1 {
					// Align the first column to the left
					o.printf(":")
				} else {
					// Align the other columns to the right
					if headerString[i+1] == '|' && i > headerLen {
						o.printf(":")
					} else {
						o.printf("-")
					}
				}
			}
		}

		o.println()
	}
}

func (o *outputBuilder) WriteFooter() {
	total := o.result.Total
	maxPathLen := o.result.MaxPathLength

	if o.opts.OutputType == OutputTypeDefault {
		o.printf("%.[2]*[1]s\n", defaultOutputSeparator, o.rowLen)
		if o.opts.ByFile {
			o.printf("%-[1]*[2]v %6[3]v %14[4]v %14[5]v %14[6]v\n",
				maxPathLen, "TOTAL", total.Total, total.Blanks, total.Comments, total.Code)
		} else {
			o.printf("%-27v %6v %14v %14v %14v\n",
				"TOTAL", total.Total, total.Blanks, total.Comments, total.Code)
		}
		o.printf("%.[2]*[1]s\n", defaultOutputSeparator, o.rowLen)
	}

	if o.opts.OutputType == OutputTypeMarkdown {
		if o.opts.ByFile {
			o.printf("| %-[1]*[2]v |%10v|%12v|%14v|%8v |\n", maxPathLen, "", "", "", "", "")
			o.printf("| %-[1]*[2]v |%9v |%11v |%13v |%8v |\n", maxPathLen, "TOTAL", total.Total, total.Blanks, total.Comments, total.Code)
		} else {
			o.printf("| %21v|%22v|%12v|%14v|%8v |\n", "", "", "", "", "")
			o.printf("| %20v |%21v |%11v |%13v |%8v |\n", "TOTAL", total.Total, total.Blanks, total.Comments, total.Code)
		}
	}
}

func (o *outputBuilder) writeByFile() {
	opts, result := o.opts, o.result
	clocFiles := result.Files
	total := result.Total
	maxPathLen := result.MaxPathLength

	var sortedFiles gocloc.ClocFiles
	for _, file := range clocFiles {
		sortedFiles = append(sortedFiles, *file)
	}
	switch opts.SortTag {
	case "name":
		sortedFiles.SortByName()
	case "comment":
		sortedFiles.SortByComments()
	case "blank":
		sortedFiles.SortByBlanks()
	default:
		sortedFiles.SortByCode()
	}

	switch opts.OutputType {
	case OutputTypeClocXML:
		t := gocloc.XMLTotalFiles{
			Code:    total.Code,
			Comment: total.Comments,
			Blank:   total.Blanks,
		}
		f := &gocloc.XMLResultFiles{
			Files: sortedFiles,
			Total: t,
		}
		xmlResult := gocloc.XMLResult{
			XMLFiles: f,
		}
		o.err = xmlResult.EncodeTo(o)
	case OutputTypeSloccount:
		for _, file := range sortedFiles {
			p := ""
			if strings.HasPrefix(file.Name, "./") || string(file.Name[0]) == "/" {
				splitPaths := strings.Split(file.Name, string(os.PathSeparator))
				if len(splitPaths) >= 3 {
					p = splitPaths[1]
				}
			}
			o.printf("%v\t%v\t%v\t%v\n",
				file.Code, file.Lang, p, file.Name)
		}
	case OutputTypeJSON:
		jsonResult := gocloc.NewJSONFilesResultFromCloc(total, sortedFiles)
		o.writeJSON(jsonResult)
	case OutputTypeMarkdown:
		for _, file := range sortedFiles {
			clocFile := file
			o.printf("| %-[1]*[2]s |%8[3]v  |%11[4]v |%13[5]v |%8[6]v |\n",
				maxPathLen, file.Name, 1, clocFile.Blanks, clocFile.Comments, clocFile.Code)
		}

	default:
		for _, file := range sortedFiles {
			clocFile := file
			o.printf("%-[1]*[2]s %21[3]v %14[4]v %14[5]v\n",
				maxPathLen, file.Name, clocFile.Blanks, clocFile.Comments, clocFile.Code)
		}
	}
}

func (o *outputBuilder) WriteResult() error {
	// write header
	o.WriteHeader()

	clocLangs := o.result.Languages
	total := o.result.Total

	if o.opts.ByFile {
		o.writeByFile()
	} else {
		var sortedLanguages gocloc.Languages
		for _, language := range clocLangs {
			if len(language.Files) != 0 {
				sortedLanguages = append(sortedLanguages, *language)
			}
		}
		switch o.opts.SortTag {
		case "name":
			sortedLanguages.SortByName()
		case "files":
			sortedLanguages.SortByFiles()
		case "comment":
			sortedLanguages.SortByComments()
		case "blank":
			sortedLanguages.SortByBlanks()
		default:
			sortedLanguages.SortByCode()
		}

		switch o.opts.OutputType {
		case OutputTypeClocXML:
			xmlResult := gocloc.NewXMLResultFromCloc(total, sortedLanguages, gocloc.XMLResultWithLangs)
			o.err = xmlResult.EncodeTo(o)
		case OutputTypeJSON:
			jsonResult := gocloc.NewJSONLanguagesResultFromCloc(total, sortedLanguages)
			o.writeJSON(jsonResult)
		case OutputTypeMarkdown:
			for _, language := range sortedLanguages {
				o.printf("| %-20v |%21v |%11v |%13v |%8v |\n",
					language.Name, len(language.Files), language.Blanks, language.Comments, language.Code)
			}
		default:
			for _, language := range sortedLanguages {
				o.printf("%-27v %6v %14v %14v %14v\n",
					language.Name, len(language.Files), language.Blanks, language.Comments, language.Code)
			}
		}
	}

	// write footer
	o.WriteFooter()
	if o.err != nil {
		return fmt.Errorf("write result: %w", o.err)
	}
	return nil
}
