package main

import (
	"flag"
	"log"
	"path/filepath"
	"strings"

	"github.com/sigma/monorepo-hatchet/pkg/cleaner"
	"github.com/sigma/monorepo-hatchet/pkg/pkglist"
)

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
		patterns[i] = strings.TrimSuffix(strings.TrimSpace(p), "/")
	}

	if *sourceDir == "" {
		log.Fatalf("Source directory is required")
	}

	absSourceDir, err := filepath.Abs(*sourceDir)
	if err != nil {
		log.Fatalf("Failed to get absolute path: %v", err)
	}

	// Step 1: Find all packages
	finder := pkglist.NewFinder(absSourceDir)
	if err := finder.FindAll(); err != nil {
		log.Fatalf("Failed to find packages: %v", err)
	}

	// Step 2: Filter packages based on patterns
	keepPackages := finder.FilterByPatterns(patterns)

	// Step 3: Add dependencies
	finder.AddDependencies(keepPackages)

	// Step 4: Build list of files to keep
	allFiles := finder.GetFileList(keepPackages, *withTests)

	log.Printf("Total files to keep: %d", len(allFiles))
	for _, f := range allFiles {
		log.Printf("  Keeping: %s", f)
	}

	// Step 5: Clean
	c := cleaner.New(*sourceDir, allFiles,
		cleaner.WithGitProtection(*protectGit),
		cleaner.WithGoModProtection(*protectGoMod),
		cleaner.WithTestKeeping(*withTests),
		cleaner.WithDryRun(*dryRun),
		cleaner.WithGoModTidy(true),
	)
	if err := c.Clean(); err != nil {
		log.Fatalf("Failed to clean directory: %v", err)
	}
}
