package cleaner

import (
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"

	"log"

	"github.com/spf13/afero"
)

type Cleaner struct {
	sourceDir    string
	filesToKeep  map[string]struct{}
	fs           afero.Fs
	protectGit   bool
	protectGoMod bool
	keepTests    bool
	dryRun       bool
	runGoModTidy bool
}

type Option func(*Cleaner)

// WithGitProtection enables or disables .git directory protection
func WithGitProtection(protect bool) Option {
	return func(c *Cleaner) {
		c.protectGit = protect
	}
}

// WithGoModProtection enables or disables go.mod and go.sum protection
func WithGoModProtection(protect bool) Option {
	return func(c *Cleaner) {
		c.protectGoMod = protect
	}
}

// WithDryRun enables or disables dry-run mode (no files will be removed)
func WithDryRun(dryRun bool) Option {
	return func(c *Cleaner) {
		c.dryRun = dryRun
	}
}

// WithTestKeeping enables or disables keeping test files and testdata directories
func WithTestKeeping(keep bool) Option {
	return func(c *Cleaner) {
		c.keepTests = keep
	}
}

// WithGoModTidy enables or disables running go mod tidy after cleaning
func WithGoModTidy(enabled bool) Option {
	return func(c *Cleaner) {
		c.runGoModTidy = enabled
	}
}

func New(sourceDir string, filesToKeep []string, opts ...Option) *Cleaner {
	keepFiles := make(map[string]struct{})
	for _, file := range filesToKeep {
		keepFiles[file] = struct{}{}
	}

	c := &Cleaner{
		sourceDir:    sourceDir,
		filesToKeep:  keepFiles,
		fs:           afero.NewOsFs(),
		protectGit:   true,  // protect .git by default
		protectGoMod: true,  // protect go.mod and go.sum by default
		keepTests:    false, // don't keep tests by default
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// NewWithFs creates a new Cleaner with a custom filesystem - useful for testing
func NewWithFs(sourceDir string, filesToKeep []string, fs afero.Fs, opts ...Option) *Cleaner {
	c := New(sourceDir, filesToKeep, opts...)
	c.fs = fs
	return c
}

func (c *Cleaner) Clean() error {
	// First pass: collect all files to remove
	var toRemove []string
	err := afero.Walk(c.fs, c.sourceDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories for now
		if info.IsDir() {
			return nil
		}

		// Convert to absolute path for comparison
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for %s: %v", path, err)
		}

		// Keep files that are in our keep list
		if _, keep := c.filesToKeep[absPath]; keep {
			return nil
		}

		// Keep .git files if protection is enabled
		if c.protectGit && strings.Contains(absPath, "/.git/") {
			return nil
		}

		// Handle testdata directories
		inTestdata := strings.Contains(absPath, "/testdata/")

		// Keep go.mod and go.sum files if protection is enabled (except in testdata)
		if c.protectGoMod && !inTestdata {
			base := filepath.Base(absPath)
			if base == "go.mod" || base == "go.sum" {
				return nil
			}
		}

		toRemove = append(toRemove, absPath)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk directory: %v", err)
	}

	// Second pass: remove files
	if !c.dryRun {
		for _, path := range toRemove {
			if err := c.fs.Remove(path); err != nil {
				return fmt.Errorf("failed to remove %s: %v", path, err)
			}
		}
	}

	// Third pass: remove empty directories
	if err := c.removeEmptyDirs(c.sourceDir); err != nil {
		return fmt.Errorf("failed to clean empty directories: %v", err)
	}

	// Run go mod tidy after cleaning if requested
	if !c.dryRun && c.runGoModTidy {
		cmd := exec.Command("go", "mod", "tidy")
		cmd.Dir = c.sourceDir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to run go mod tidy: %v\nOutput: %s", err, out)
		}
		log.Printf("Successfully ran go mod tidy in %s", c.sourceDir)
	}

	return nil
}

func (c *Cleaner) removeEmptyDirs(path string) error {
	entries, err := afero.ReadDir(c.fs, path)
	if err != nil {
		return err
	}

	// First, recursively process subdirectories
	for _, entry := range entries {
		if entry.IsDir() {
			subpath := filepath.Join(path, entry.Name())
			// Skip .git directory if protected
			if c.protectGit && entry.Name() == ".git" {
				continue
			}
			if err := c.removeEmptyDirs(subpath); err != nil {
				return err
			}
		}
	}

	// Check if directory is empty
	entries, err = afero.ReadDir(c.fs, path)
	if err != nil {
		return err
	}

	// Remove if empty (except source directory)
	if len(entries) == 0 && path != c.sourceDir {
		if c.dryRun {
			return nil
		}
		return c.fs.Remove(path)
	}

	return nil
}
