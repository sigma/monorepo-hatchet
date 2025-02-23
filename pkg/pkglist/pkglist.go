package pkglist

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

// Package represents a Go package with its files and dependencies
type Package struct {
	Dir         string
	ImportPath  string
	Deps        []string
	EmbedFiles  []string // Files embedded using //go:embed
	GoFiles     []string // Regular .go files
	TestGoFiles []string // Test .go files
	OtherFiles  []string // Non-Go files in the package directory
}

// Finder handles discovering and filtering Go packages
type Finder struct {
	sourceDir string
	packages  map[string]*Package
	fs        afero.Fs
	commander Commander
}

// NewFinder creates a new package finder for the given source directory
func NewFinder(sourceDir string) *Finder {
	return &Finder{
		sourceDir: sourceDir,
		packages:  make(map[string]*Package),
		fs:        afero.NewOsFs(),
		commander: &RealCommander{},
	}
}

// FindAll discovers all packages in the repository
func (f *Finder) FindAll() error {
	cmd := f.commander.Command("go", "list", "-json", "./...")
	cmd.SetDir(f.sourceDir)

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list packages: %v", err)
	}

	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for decoder.More() {
		var pkg Package
		if err := decoder.Decode(&pkg); err != nil {
			return fmt.Errorf("failed to decode package info: %v", err)
		}
		f.packages[pkg.ImportPath] = &pkg
		log.Printf("Found package: %s at %s", pkg.ImportPath, pkg.Dir)
	}

	return nil
}

// FilterByPatterns returns packages matching the given patterns
func (f *Finder) FilterByPatterns(patterns []string) map[string]struct{} {
	keepPackages := make(map[string]struct{})
	for _, pattern := range patterns {
		log.Printf("Processing pattern: %s", pattern)
		for _, pkg := range f.packages {
			if f.matchPackage(pattern, pkg.ImportPath, pkg.Dir) {
				log.Printf("  Matched package: %s at %s", pkg.ImportPath, pkg.Dir)
				keepPackages[pkg.ImportPath] = struct{}{}
			}
		}
	}
	return keepPackages
}

// AddDependencies adds all dependencies of the kept packages to the keep set
func (f *Finder) AddDependencies(keepPackages map[string]struct{}) {
	toProcess := make([]string, 0, len(keepPackages))
	for pkg := range keepPackages {
		toProcess = append(toProcess, pkg)
	}

	for i := 0; i < len(toProcess); i++ {
		pkg := toProcess[i]
		if p, ok := f.packages[pkg]; ok {
			for _, dep := range p.Deps {
				if _, ok := keepPackages[dep]; !ok {
					if _, inRepo := f.packages[dep]; inRepo {
						keepPackages[dep] = struct{}{}
						toProcess = append(toProcess, dep)
					}
				}
			}
		}
	}
}

// GetFileList returns all files from the kept packages
func (f *Finder) GetFileList(keepPackages map[string]struct{}, withTests bool) []string {
	var allFiles []string
	for pkgPath, pkg := range f.packages {
		if _, keep := keepPackages[pkgPath]; !keep {
			continue
		}

		// Add all Go files from the package
		for _, file := range pkg.GoFiles {
			allFiles = append(allFiles, filepath.Join(pkg.Dir, file))
		}

		// Add test files if requested
		if withTests {
			for _, file := range pkg.TestGoFiles {
				allFiles = append(allFiles, filepath.Join(pkg.Dir, file))
				log.Printf("  Keeping test file: %s", filepath.Join(pkg.Dir, file))
			}

			// Add all other files when tests are included
			for _, file := range pkg.OtherFiles {
				allFiles = append(allFiles, filepath.Join(pkg.Dir, file))
				log.Printf("  Keeping other file: %s", filepath.Join(pkg.Dir, file))
			}
		} else {
			// If not keeping tests, only add non-testdata files
			for _, file := range pkg.OtherFiles {
				absPath := filepath.Join(pkg.Dir, file)
				if !strings.Contains(absPath, "/testdata/") {
					allFiles = append(allFiles, absPath)
					log.Printf("  Keeping other file: %s", absPath)
				}
			}
		}

		// Add all embedded files
		for _, file := range pkg.EmbedFiles {
			allFiles = append(allFiles, filepath.Join(pkg.Dir, file))
			log.Printf("  Keeping embedded file: %s", filepath.Join(pkg.Dir, file))
		}
	}
	return allFiles
}

// matchPackage checks if a package matches the given pattern
func (f *Finder) matchPackage(pattern, importPath, dir string) bool {
	// Convert paths to slash form for comparison
	pattern = filepath.ToSlash(pattern)
	importPath = filepath.ToSlash(importPath)
	dir = filepath.ToSlash(dir)

	log.Printf("    Matching pattern '%s' against import '%s' and dir '%s'", pattern, importPath, dir)

	// First check exact match against import path
	if pattern == importPath {
		log.Printf("    -> Matched exact import path")
		return true
	}

	// Handle wildcards
	if strings.HasSuffix(pattern, "/...") {
		prefix := strings.TrimSuffix(pattern, "/...")
		log.Printf("    Checking wildcard with prefix '%s'", prefix)

		if prefix == "." {
			log.Printf("    -> Matched '.' wildcard")
			return true // Match everything for "./..."
		}

		// Check if the package path ends with the prefix
		if strings.HasSuffix(importPath, "/"+prefix) || strings.HasPrefix(importPath, prefix+"/") {
			log.Printf("    -> Matched import path")
			return true
		}

		// Check if the directory path contains the prefix
		if strings.Contains(dir, "/"+prefix+"/") {
			log.Printf("    -> Matched directory path")
			return true
		}

		log.Printf("    -> No wildcard matches found")
		return false
	}

	// For exact matches, try both the full import path and the last component
	importParts := strings.Split(importPath, "/")
	if len(importParts) > 0 && importParts[len(importParts)-1] == pattern {
		log.Printf("    -> Matched package name")
		return true
	}

	// Try matching against the full import path
	if strings.HasSuffix(importPath, "/"+pattern) {
		log.Printf("    -> Matched import path")
		return true
	}

	// Try matching against the directory path
	if strings.HasSuffix(dir, "/"+pattern) {
		log.Printf("    -> Matched directory path")
		return true
	}

	log.Printf("    -> No exact matches found")
	return false
}
