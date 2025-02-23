package analyzer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum-optimism/monorepo-hatchet/pkg/analyzer"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	// Create testdata directory if it doesn't exist
	testdata := analysistest.TestData()
	if err := os.MkdirAll(filepath.Join(testdata, "src", "a"), 0755); err != nil {
		t.Fatal(err)
	}

	analysistest.RunWithSuggestedFixes(t, testdata, analyzer.Analyzer, "a")
}
