package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type ComparisonReport struct {
	MissingInTemplates []string          `json:"missing_in_templates"`
	ContentDifferences []ContentDiff     `json:"content_differences"`
	MissingInLive      []string          `json:"missing_in_live"`
	Summary            ComparisonSummary `json:"summary"`
}

type ContentDiff struct {
	Path         string `json:"path"`
	TemplatePath string `json:"template_path"`
	LivePath     string `json:"live_path"`
	Reason       string `json:"reason"`
}

type ComparisonSummary struct {
	TotalTemplateFiles int    `json:"total_template_files"`
	TotalLiveFiles     int    `json:"total_live_files"`
	SyncNeeded         int    `json:"sync_needed"`
	Status             string `json:"status"`
}

func main() {
	report := ComparisonReport{
		MissingInTemplates: []string{},
		ContentDifferences: []ContentDiff{},
		MissingInLive:      []string{},
	}

	templateBase := "cmd/kairon/templates"

	// Compare kairon directory
	compareDirectory(templateBase+"/kairon", ".kairon", &report)

	// Compare kiro directory
	compareDirectory(templateBase+"/kiro", ".kiro", &report)

	// Calculate summary
	report.Summary.TotalTemplateFiles = countFiles(templateBase)
	report.Summary.TotalLiveFiles = countFiles(".kairon") + countFiles(".kiro")
	report.Summary.SyncNeeded = len(report.MissingInTemplates) + len(report.ContentDifferences)

	if report.Summary.SyncNeeded == 0 {
		report.Summary.Status = "synchronized"
	} else {
		report.Summary.Status = "sync_required"
	}

	// Output structured report
	output, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating report: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(output))

	if report.Summary.SyncNeeded > 0 {
		os.Exit(1)
	}
}

func compareDirectory(templateDir, liveDir string, report *ComparisonReport) {
	// Scan live directory and compare with templates
	err := filepath.Walk(liveDir, func(livePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible files
		}

		if info.IsDir() {
			return nil
		}

		// Skip generated/runtime files
		if shouldSkip(livePath) {
			return nil
		}

		// Calculate corresponding template path
		relPath, _ := filepath.Rel(liveDir, livePath)
		templatePath := filepath.Join(templateDir, relPath)

		// Check if template exists
		if _, err := os.Stat(templatePath); os.IsNotExist(err) {
			report.MissingInTemplates = append(report.MissingInTemplates, relPath)
			return nil
		}

		// Compare content
		if different, reason := compareFiles(templatePath, livePath); different {
			report.ContentDifferences = append(report.ContentDifferences, ContentDiff{
				Path:         relPath,
				TemplatePath: templatePath,
				LivePath:     livePath,
				Reason:       reason,
			})
		}

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking live directory %s: %v\n", liveDir, err)
	}

	// Check for template files missing in live
	err = filepath.Walk(templateDir, func(templatePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(templateDir, templatePath)
		livePath := filepath.Join(liveDir, relPath)

		if _, err := os.Stat(livePath); os.IsNotExist(err) {
			report.MissingInLive = append(report.MissingInLive, relPath)
		}

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking template directory %s: %v\n", templateDir, err)
	}
}

func shouldSkip(path string) bool {
	skipPaths := []string{
		"specs/", "artifacts/", "retries/", "results/", "skills/",
		"config.yaml", "validation-results.md", "build-instructions.md",
	}

	for _, skip := range skipPaths {
		if strings.Contains(path, skip) {
			return true
		}
	}
	return false
}

func compareFiles(templatePath, livePath string) (bool, string) {
	templateHash, err1 := fileHash(templatePath)
	liveHash, err2 := fileHash(livePath)

	if err1 != nil {
		return true, fmt.Sprintf("template read error: %v", err1)
	}
	if err2 != nil {
		return true, fmt.Sprintf("live read error: %v", err2)
	}

	if templateHash != liveHash {
		return true, "content differs"
	}

	return false, ""
}

func fileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	_, err = io.Copy(hash, file)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func countFiles(dir string) int {
	count := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && !shouldSkip(path) {
			count++
		}
		return nil
	})
	return count
}
