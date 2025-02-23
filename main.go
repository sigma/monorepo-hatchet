package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ethereum-optimism/monorepo-hatchet/pkg/cleaner"
)

type goListPackage struct {
	Dir         string
	ImportPath  string
	Deps        []string
	EmbedFiles  []string // Files embedded using //go:embed
	GoFiles     []string // Regular .go files
	TestGoFiles []string // Test .go files
	OtherFiles  []string // Non-Go files in the package directory
}

func findPackagesInRepo(sourceDir string) (map[string]*goListPackage, error) {
	// Save current directory
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %v", err)
	}
	defer os.Chdir(currentDir) // Restore original directory when done

	// Change to source directory
	if err := os.Chdir(sourceDir); err != nil {
		return nil, fmt.Errorf("failed to change to source directory: %v", err)
	}

	// Remove -test=false to include test files in the listing
	cmd := exec.Command("go", "list", "-json", "./...")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list packages: %v", err)
	}

	// Split the output into individual JSON objects
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	packages := make(map[string]*goListPackage)

	for decoder.More() {
		var pkg goListPackage
		if err := decoder.Decode(&pkg); err != nil {
			return nil, fmt.Errorf("failed to decode package info: %v", err)
		}
		packages[pkg.ImportPath] = &pkg
		log.Printf("Found package: %s at %s", pkg.ImportPath, pkg.Dir)
	}

	return packages, nil
}

func matchPackage(pattern, importPath, dir string) bool {
	// Convert paths to slash form for comparison
	pattern = filepath.ToSlash(pattern)
	importPath = filepath.ToSlash(importPath)
	dir = filepath.ToSlash(dir)

	log.Printf("    Matching pattern '%s' against import '%s' and dir '%s'", pattern, importPath, dir)

	// Handle wildcards first
	if strings.HasSuffix(pattern, "/...") {
		prefix := strings.TrimSuffix(pattern, "/...")
		log.Printf("    Checking wildcard with prefix '%s'", prefix)

		if prefix == "." {
			log.Printf("    -> Matched '.' wildcard")
			return true // Match everything for "./..."
		}

		// For wildcards, normalize both paths by removing the org prefix
		normalizeImportPath := func(path string) string {
			parts := strings.Split(path, "/")
			if len(parts) > 2 && parts[0] == "github.com" {
				// Keep the org name for matching
				return strings.Join(parts[2:], "/")
			}
			return path
		}

		normalizedPrefix := normalizeImportPath(prefix)
		normalizedImport := normalizeImportPath(importPath)

		// Check import path
		if strings.HasPrefix(normalizedImport, normalizedPrefix) {
			log.Printf("    -> Matched import path prefix")
			return true
		}

		// Check directory path
		relDir := strings.TrimPrefix(dir, "/")
		if strings.Contains(relDir, "/"+normalizedPrefix) {
			log.Printf("    -> Matched directory path")
			return true
		}

		log.Printf("    -> No wildcard matches found")
		return false
	}

	// For exact matches
	normalizeImportPath := func(path string) string {
		parts := strings.Split(path, "/")
		if len(parts) > 2 && parts[0] == "github.com" {
			return strings.Join(parts[2:], "/")
		}
		return path
	}

	normalizedPattern := normalizeImportPath(pattern)
	normalizedImport := normalizeImportPath(importPath)

	if normalizedImport == normalizedPattern {
		log.Printf("    -> Matched normalized import path")
		return true
	}

	// Try relative path match from the end
	relDir := strings.TrimPrefix(dir, "/")
	if strings.HasSuffix(relDir, "/"+normalizedPattern) {
		log.Printf("    -> Matched relative path")
		return true
	}

	log.Printf("    -> No exact matches found")
	return false
}

func main() {
	sourceDir := flag.String("dir", "", "Source directory to analyze")
	packagePatterns := flag.String("packages", "", "Comma-separated list of packages to keep")
	withTests := flag.Bool("with-tests", false, "Include test files for kept packages")
	protectGit := flag.Bool("protect-git", true, "Protect .git directories from being cleaned")
	protectGoMod := flag.Bool("protect-gomod", true, "Protect go.mod and go.sum files from being cleaned")
	dryRun := flag.Bool("dry-run", false, "Don't actually remove files, just show what would be done")
	flag.Parse()

	patterns := strings.Split(*packagePatterns, ",")
	if len(patterns) == 0 {
		flag.Usage()
		return
	}

	// Clean up patterns
	for i, p := range patterns {
		p = strings.TrimSpace(p)
		p = strings.TrimSuffix(p, "/") // Remove trailing slashes
		patterns[i] = p
	}

	if *sourceDir == "" {
		log.Fatalf("Source directory is required")
	}

	// Get absolute path to source directory
	absSourceDir, err := filepath.Abs(*sourceDir)
	if err != nil {
		log.Fatalf("Failed to get absolute path: %v", err)
	}

	// 1. Find all packages in the repository
	allPackages, err := findPackagesInRepo(absSourceDir)
	if err != nil {
		log.Fatalf("Failed to find packages: %v", err)
	}

	// 2. Find the subset of packages we want to keep
	keepPackages := make(map[string]struct{})
	for _, pattern := range patterns {
		log.Printf("Processing pattern: %s", pattern)
		for _, pkg := range allPackages {
			if matchPackage(pattern, pkg.ImportPath, pkg.Dir) {
				log.Printf("  Matched package: %s at %s", pkg.ImportPath, pkg.Dir)
				keepPackages[pkg.ImportPath] = struct{}{}
			}
		}
	}

	// 3. Add all dependencies
	toProcess := make([]string, 0, len(keepPackages))
	for pkg := range keepPackages {
		toProcess = append(toProcess, pkg)
	}

	for i := 0; i < len(toProcess); i++ {
		pkg := toProcess[i]
		if p, ok := allPackages[pkg]; ok {
			for _, dep := range p.Deps {
				if _, ok := keepPackages[dep]; !ok {
					if _, inRepo := allPackages[dep]; inRepo {
						keepPackages[dep] = struct{}{}
						toProcess = append(toProcess, dep)
					}
				}
			}
		}
	}

	// 4. Build list of files to keep
	var allFiles []string
	for pkgPath, pkg := range allPackages {
		// Only process packages we want to keep
		if _, keep := keepPackages[pkgPath]; keep {
			// Add all Go files from the package
			for _, f := range pkg.GoFiles {
				absPath := filepath.Join(pkg.Dir, f)
				allFiles = append(allFiles, absPath)
			}

			// Add test files if requested
			if *withTests {
				for _, f := range pkg.TestGoFiles {
					absPath := filepath.Join(pkg.Dir, f)
					allFiles = append(allFiles, absPath)
					log.Printf("  Keeping test file: %s", absPath)
				}

				// Only add testdata files if we're keeping tests for this package
				for _, f := range pkg.OtherFiles {
					absPath := filepath.Join(pkg.Dir, f)
					if strings.Contains(absPath, "/testdata/") {
						allFiles = append(allFiles, absPath)
						log.Printf("  Keeping testdata file: %s", absPath)
					} else {
						allFiles = append(allFiles, absPath)
						log.Printf("  Keeping other file: %s", absPath)
					}
				}
			} else {
				// If not keeping tests, only add non-testdata files
				for _, f := range pkg.OtherFiles {
					absPath := filepath.Join(pkg.Dir, f)
					if !strings.Contains(absPath, "/testdata/") {
						allFiles = append(allFiles, absPath)
						log.Printf("  Keeping other file: %s", absPath)
					}
				}
			}

			// Add all embedded files
			for _, f := range pkg.EmbedFiles {
				absPath := filepath.Join(pkg.Dir, f)
				allFiles = append(allFiles, absPath)
				log.Printf("  Keeping embedded file: %s", absPath)
			}
		}
	}

	log.Printf("Total files to keep: %d", len(allFiles))
	for _, f := range allFiles {
		log.Printf("  Keeping: %s", f)
	}

	c := cleaner.New(*sourceDir, allFiles,
		cleaner.WithGitProtection(*protectGit),
		cleaner.WithGoModProtection(*protectGoMod),
		cleaner.WithTestKeeping(*withTests),
		cleaner.WithDryRun(*dryRun),
	)
	if err := c.Clean(); err != nil {
		log.Fatalf("Failed to clean directory: %v", err)
	}

	// Run go mod tidy after cleaning if not in dry run mode
	if !*dryRun {
		// Save current directory
		currentDir, err := os.Getwd()
		if err != nil {
			log.Printf("Warning: Failed to get current directory before go mod tidy: %v", err)
		} else {
			defer os.Chdir(currentDir) // Restore original directory when done
		}

		// Change to source directory
		if err := os.Chdir(*sourceDir); err != nil {
			log.Printf("Warning: Failed to change to source directory for go mod tidy: %v", err)
		} else {
			cmd := exec.Command("go", "mod", "tidy")
			if out, err := cmd.CombinedOutput(); err != nil {
				log.Printf("Warning: Failed to run go mod tidy: %v\nOutput: %s", err, out)
			} else {
				log.Printf("Successfully ran go mod tidy in %s", *sourceDir)
			}
		}
	}
}
