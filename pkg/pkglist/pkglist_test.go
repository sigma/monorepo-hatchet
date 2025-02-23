package pkglist

import (
	"fmt"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockCommand implements Command for testing
type MockCommand struct {
	output []byte
	err    error
	dir    string
}

func (c *MockCommand) SetDir(dir string) {
	c.dir = dir
}

func (c *MockCommand) Output() ([]byte, error) {
	return c.output, c.err
}

// MockCommander implements Commander for testing
type MockCommander struct {
	commands map[string]*MockCommand
}

func (c *MockCommander) Command(name string, args ...string) Command {
	key := fmt.Sprintf("%s %v", name, args)
	return c.commands[key]
}

func TestFinder_FindAll(t *testing.T) {
	tests := []struct {
		name       string
		jsonOutput string
		wantErr    bool
		wantPkgs   map[string]*Package
	}{
		{
			name: "basic package list",
			jsonOutput: `
				{"ImportPath": "github.com/test/repo/pkg1", "Dir": "/go/src/github.com/test/repo/pkg1", "GoFiles": ["file1.go"]}
				{"ImportPath": "github.com/test/repo/pkg2", "Dir": "/go/src/github.com/test/repo/pkg2", "GoFiles": ["file2.go"]}
			`,
			wantPkgs: map[string]*Package{
				"github.com/test/repo/pkg1": {
					ImportPath: "github.com/test/repo/pkg1",
					Dir:        "/go/src/github.com/test/repo/pkg1",
					GoFiles:    []string{"file1.go"},
				},
				"github.com/test/repo/pkg2": {
					ImportPath: "github.com/test/repo/pkg2",
					Dir:        "/go/src/github.com/test/repo/pkg2",
					GoFiles:    []string{"file2.go"},
				},
			},
		},
		{
			name:       "invalid json",
			jsonOutput: "invalid json",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCmd := &MockCommand{
				output: []byte(tt.jsonOutput),
			}

			commander := &MockCommander{
				commands: map[string]*MockCommand{
					"go [list -json ./...]": mockCmd,
				},
			}

			f := &Finder{
				sourceDir: "/test",
				packages:  make(map[string]*Package),
				fs:        afero.NewMemMapFs(),
				commander: commander,
			}

			err := f.FindAll()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantPkgs, f.packages)
		})
	}
}

func TestFinder_FilterByPatterns(t *testing.T) {
	tests := []struct {
		name     string
		packages map[string]*Package
		patterns []string
		want     map[string]struct{}
	}{
		{
			name: "exact match by package name",
			packages: map[string]*Package{
				"github.com/test/repo/pkg1": {
					ImportPath: "github.com/test/repo/pkg1",
					Dir:        "/go/src/github.com/test/repo/pkg1",
				},
				"github.com/test/repo/pkg2": {
					ImportPath: "github.com/test/repo/pkg2",
					Dir:        "/go/src/github.com/test/repo/pkg2",
				},
			},
			patterns: []string{"github.com/test/repo/pkg1"},
			want: map[string]struct{}{
				"github.com/test/repo/pkg1": {},
			},
		},
		{
			name: "wildcard match with prefix",
			packages: map[string]*Package{
				"github.com/test/repo/pkg/a": {
					ImportPath: "github.com/test/repo/pkg/a",
					Dir:        "/go/src/github.com/test/repo/pkg/a",
				},
				"github.com/test/repo/pkg/b": {
					ImportPath: "github.com/test/repo/pkg/b",
					Dir:        "/go/src/github.com/test/repo/pkg/b",
				},
				"github.com/test/repo/other": {
					ImportPath: "github.com/test/repo/other",
					Dir:        "/go/src/github.com/test/repo/other",
				},
			},
			patterns: []string{"github.com/test/repo/pkg/..."},
			want: map[string]struct{}{
				"github.com/test/repo/pkg/a": {},
				"github.com/test/repo/pkg/b": {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Finder{
				packages: tt.packages,
				fs:       afero.NewMemMapFs(),
			}

			got := f.FilterByPatterns(tt.patterns)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFinder_GetFileList(t *testing.T) {
	tests := []struct {
		name         string
		packages     map[string]*Package
		keepPackages map[string]struct{}
		withTests    bool
		want         []string
	}{
		{
			name: "basic files",
			packages: map[string]*Package{
				"pkg1": {
					Dir:     "/test/pkg1",
					GoFiles: []string{"main.go", "util.go"},
				},
			},
			keepPackages: map[string]struct{}{
				"pkg1": {},
			},
			withTests: false,
			want: []string{
				"/test/pkg1/main.go",
				"/test/pkg1/util.go",
			},
		},
		{
			name: "with tests",
			packages: map[string]*Package{
				"pkg1": {
					Dir:         "/test/pkg1",
					GoFiles:     []string{"main.go"},
					TestGoFiles: []string{"main_test.go"},
					OtherFiles:  []string{"testdata/fixture.json"},
				},
			},
			keepPackages: map[string]struct{}{
				"pkg1": {},
			},
			withTests: true,
			want: []string{
				"/test/pkg1/main.go",
				"/test/pkg1/main_test.go",
				"/test/pkg1/testdata/fixture.json",
			},
		},
		{
			name: "with XTestGoFiles",
			packages: map[string]*Package{
				"pkg1": {
					Dir:          "/test/pkg1",
					GoFiles:      []string{"main.go"},
					TestGoFiles:  []string{"main_test.go"},
					XTestGoFiles: []string{"export_test.go"},
				},
			},
			keepPackages: map[string]struct{}{
				"pkg1": {},
			},
			withTests: true,
			want: []string{
				"/test/pkg1/main.go",
				"/test/pkg1/main_test.go",
				"/test/pkg1/export_test.go",
			},
		},
		{
			name: "with embedded files",
			packages: map[string]*Package{
				"pkg1": {
					Dir:          "/test/pkg1",
					GoFiles:      []string{"main.go"},
					EmbedFiles:   []string{"embed/data.json", "embed/config.yaml"},
					TestGoFiles:  []string{"main_test.go"},
					XTestGoFiles: []string{"export_test.go"},
				},
			},
			keepPackages: map[string]struct{}{
				"pkg1": {},
			},
			withTests: true,
			want: []string{
				"/test/pkg1/main.go",
				"/test/pkg1/main_test.go",
				"/test/pkg1/export_test.go",
				"/test/pkg1/embed/data.json",
				"/test/pkg1/embed/config.yaml",
			},
		},
		{
			name: "without tests should exclude test files",
			packages: map[string]*Package{
				"pkg1": {
					Dir:          "/test/pkg1",
					GoFiles:      []string{"main.go"},
					EmbedFiles:   []string{"embed/data.json"},
					TestGoFiles:  []string{"main_test.go"},
					XTestGoFiles: []string{"export_test.go"},
				},
			},
			keepPackages: map[string]struct{}{
				"pkg1": {},
			},
			withTests: false,
			want: []string{
				"/test/pkg1/main.go",
				"/test/pkg1/embed/data.json",
			},
		},
		{
			name: "multiple packages with various file types",
			packages: map[string]*Package{
				"pkg1": {
					Dir:         "/test/pkg1",
					GoFiles:     []string{"main.go"},
					EmbedFiles:  []string{"embed/data.json"},
					TestGoFiles: []string{"main_test.go"},
				},
				"pkg2": {
					Dir:          "/test/pkg2",
					GoFiles:      []string{"util.go"},
					XTestGoFiles: []string{"util_export_test.go"},
					OtherFiles:   []string{"testdata/sample.txt"},
				},
			},
			keepPackages: map[string]struct{}{
				"pkg1": {},
				"pkg2": {},
			},
			withTests: true,
			want: []string{
				"/test/pkg1/main.go",
				"/test/pkg1/main_test.go",
				"/test/pkg1/embed/data.json",
				"/test/pkg2/util.go",
				"/test/pkg2/util_export_test.go",
				"/test/pkg2/testdata/sample.txt",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Finder{
				packages: tt.packages,
				fs:       afero.NewMemMapFs(),
			}

			got := f.GetFileList(tt.keepPackages, tt.withTests)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
