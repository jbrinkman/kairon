package incidents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type IncidentLogger struct {
	baseDir  string
	repoName string
}

type IncidentInfo struct {
	IssueNumber int
	Attempt     int
	Timestamp   time.Time
	Title       string
	FilePath    string
}

func NewIncidentLogger() (*IncidentLogger, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".kairon", "logs")
	repoName, err := getRepoName()
	if err != nil {
		return nil, fmt.Errorf("failed to get repository name: %w", err)
	}

	return &IncidentLogger{
		baseDir:  baseDir,
		repoName: repoName,
	}, nil
}

func (il *IncidentLogger) LogIncident(issue, attempt int, content string) error {
	if err := il.ensureIncidentDir(); err != nil {
		return fmt.Errorf("failed to create incident directory: %w", err)
	}

	filename := il.generateFilename(issue, attempt)
	return il.writeIncidentFile(filename, content)
}

func (il *IncidentLogger) ListIncidents() ([]IncidentInfo, error) {
	incidentDir := filepath.Join(il.baseDir, il.repoName, "incidents")

	files, err := os.ReadDir(incidentDir)
	if os.IsNotExist(err) {
		return []IncidentInfo{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read incident directory: %w", err)
	}

	var incidents []IncidentInfo
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), "incident-") {
			if info, err := parseIncidentFilename(file.Name()); err == nil {
				info.FilePath = filepath.Join(incidentDir, file.Name())
				incidents = append(incidents, info)
			}
		}
	}

	return incidents, nil
}

func (il *IncidentLogger) GetIncident(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read incident file: %w", err)
	}
	return string(content), nil
}

func (il *IncidentLogger) RepoName() string {
	return il.repoName
}

func parseIncidentFilename(filename string) (IncidentInfo, error) {
	// Parse incident-<issue>-<attempt>-<timestamp>.md
	parts := strings.Split(strings.TrimSuffix(filename, ".md"), "-")
	if len(parts) < 4 || parts[0] != "incident" {
		return IncidentInfo{}, fmt.Errorf("invalid filename format")
	}

	var info IncidentInfo
	if _, err := fmt.Sscanf(parts[1], "%d", &info.IssueNumber); err != nil {
		return IncidentInfo{}, fmt.Errorf("invalid issue number")
	}
	if _, err := fmt.Sscanf(parts[2], "%d", &info.Attempt); err != nil {
		return IncidentInfo{}, fmt.Errorf("invalid attempt number")
	}

	timestampStr := strings.Join(parts[3:], "-")
	timestamp, err := time.Parse("20060102-150405", timestampStr)
	if err != nil {
		return IncidentInfo{}, fmt.Errorf("invalid timestamp format")
	}
	info.Timestamp = timestamp

	return info, nil
}

func getRepoName() (string, error) {
	data, err := os.ReadFile(".kairon/config.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to read config: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "repo:") {
			repo := strings.TrimSpace(strings.TrimPrefix(line, "repo:"))
			repo = strings.Trim(repo, "\"'")
			if parts := strings.Split(repo, "/"); len(parts) == 2 {
				return parts[1], nil
			}
			return repo, nil
		}
	}

	return "", fmt.Errorf("repo field not found in config")
}
