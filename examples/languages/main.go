package main

import (
	"fmt"
	"os"

	"github.com/longafterx/gocloc"
)

func main() {
	result, err := gocloc.Analyze([]string{"."}, &gocloc.Options{
		IncludeLangs: []string{"Go", "Python"},
		NotMatchDir:  `(^|[/\\])(dist|node_modules|target)([/\\]|$)`,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "gocloc: %v\n", err)
		os.Exit(1)
	}

	for _, lang := range result.Languages {
		fmt.Println(lang)
	}
	fmt.Println(result.Total)
	fmt.Printf("%+v", result)
}
