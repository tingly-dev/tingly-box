// Package enterprise provides a test runner for enterprise module tests
// This runs all enterprise tests and generates coverage reports
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	// Get the repository root
	root, err := getRepoRoot()
	if err != nil {
		fmt.Printf("Error finding repo root: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("        ENTERPRISE MODULE TEST RUNNER")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Test packages
	testPackages := []string{
		"internal/enterprise/auth",
		"internal/enterprise/user",
		"internal/enterprise/token",
		"internal/enterprise/rbac",
	}

	// Run tests for each package
	passed := 0
	failed := 0

	for _, pkg := range testPackages {
		fmt.Printf("Testing: %s\n", pkg)
		fmt.Println(strings.Repeat("-", 60))

		if runTest(root, pkg) {
			fmt.Printf("✓ PASSED\n\n")
			passed++
		} else {
			fmt.Printf("✗ FAILED\n\n")
			failed++
		}
	}

	// Summary
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("                        TEST SUMMARY")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("Total Packages: %d\n", passed+failed)
	fmt.Printf("Passed:         %d\n", passed)
	fmt.Printf("Failed:         %d\n", failed)
	fmt.Println()

	if failed > 0 {
		fmt.Println("⚠️  Some tests failed. Please review the output above.")
		os.Exit(1)
	} else {
		fmt.Println("✓ All tests passed!")
	}
}

func runTest(root, pkg string) bool {
	args := []string{"test", "-v", pkg}
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	return err == nil
}

func getRepoRoot() (string, error) {
	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up to find .git directory
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("could not find .git directory")
}
