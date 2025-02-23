package cleaner

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func TestCleaner(t *testing.T) {
	// Create a memory filesystem
	fs := afero.NewMemMapFs()

	// Create some test files
	testFiles := []string{
		"/src/pkg1/file1.go",
		"/src/pkg1/file2.go",
		"/src/pkg2/file3.go",
		"/src/pkg2/file4_test.go",
	}

	for _, file := range testFiles {
		err := afero.WriteFile(fs, file, []byte("test content"), 0644)
		assert.NoError(t, err)
	}

	// Files we want to keep
	keepFiles := []string{
		"/src/pkg1/file1.go",
		"/src/pkg2/file4_test.go",
	}

	// Create cleaner with memory filesystem
	c := NewWithFs("/src", keepFiles, fs)

	// Clean the directory
	err := c.Clean()
	assert.NoError(t, err)

	// Check that only the kept files exist
	for _, file := range testFiles {
		exists, err := afero.Exists(fs, file)
		assert.NoError(t, err)

		shouldExist := false
		for _, keepFile := range keepFiles {
			if keepFile == file {
				shouldExist = true
				break
			}
		}

		assert.Equal(t, shouldExist, exists, "File %s existence state is incorrect", file)
	}
}
