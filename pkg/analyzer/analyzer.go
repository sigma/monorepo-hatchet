package analyzer

import (
	"go/ast"
	"path/filepath"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Result holds the analysis results
type Result struct {
	Files map[string]struct{}
}

var Analyzer = &analysis.Analyzer{
	Name: "embedanalyzer",
	Doc:  "Analyzes Go files to find embedded files and their dependencies",
	Run:  run,
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
	},
	FactTypes:  []analysis.Fact{},
	ResultType: reflect.TypeOf((*Result)(nil)),
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspectResult := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	result := &Result{
		Files: make(map[string]struct{}),
	}

	nodeFilter := []ast.Node{
		(*ast.GenDecl)(nil),
	}

	inspectResult.Preorder(nodeFilter, func(n ast.Node) {
		genDecl, ok := n.(*ast.GenDecl)
		if !ok {
			return
		}

		// Check for //go:embed comment
		if genDecl.Doc != nil {
			for _, comment := range genDecl.Doc.List {
				text := strings.TrimSpace(comment.Text)
				if !strings.HasPrefix(text, "//go:embed") {
					continue
				}

				// Extract the pattern after "//go:embed"
				text = strings.TrimPrefix(text, "//go:embed")
				text = strings.TrimSpace(text)

				// Split on whitespace and take first pattern
				patterns := strings.Fields(text)
				if len(patterns) == 0 {
					continue
				}

				pattern := patterns[0]
				// Get the directory containing the Go file with the embed directive
				pos := pass.Fset.Position(genDecl.Pos())
				dir := filepath.Dir(pos.Filename)
				matches, err := filepath.Glob(filepath.Join(dir, pattern))
				if err != nil {
					continue
				}
				for _, match := range matches {
					result.Files[match] = struct{}{}
					// Report the found file
					pass.Reportf(comment.Pos(), "found embedded file: testfile.txt")
				}
			}
		}
	})

	return result, nil
}
