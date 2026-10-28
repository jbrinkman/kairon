package incidents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIncidentLogger(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "incident-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create logger with custom base directory
	logger := &IncidentLogger{
		baseDir:  tempDir,
		repoName: "test-repo",
	}

	// Test logging an incident
	testContent := "# Test Incident\n\nThis is a test incident."
	err = logger.LogIncident(123, 4, testContent)
	if err != nil {
		t.Fatalf("Failed to log incident: %v", err)
	}

	// Verify directory was created
	incidentDir := filepath.Join(tempDir, "test-repo", "incidents")
	if _, err := os.Stat(incidentDir); os.IsNotExist(err) {
		t.Fatalf("Incident directory was not created")
	}

	// List incidents
	incidents, err := logger.ListIncidents()
	if err != nil {
		t.Fatalf("Failed to list incidents: %v", err)
	}

	if len(incidents) != 1 {
		t.Fatalf("Expected 1 incident, got %d", len(incidents))
	}

	incident := incidents[0]
	if incident.IssueNumber != 123 {
		t.Errorf("Expected issue number 123, got %d", incident.IssueNumber)
	}
	if incident.Attempt != 4 {
		t.Errorf("Expected attempt 4, got %d", incident.Attempt)
	}

	// Get incident content
	content, err := logger.GetIncident(incident.FilePath)
	if err != nil {
		t.Fatalf("Failed to get incident content: %v", err)
	}

	if strings.TrimSpace(content) != strings.TrimSpace(testContent) {
		t.Errorf("Content mismatch.\nExpected: %q\nGot: %q", testContent, content)
	}
}

func TestParseIncidentFilename(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		wantErr   bool
		wantIssue int
		wantAtmpt int
	}{
		{"valid", "incident-42-1-20240101-120000.md", false, 42, 1},
		{"multi-digit", "incident-999-12-20240315-083045.md", false, 999, 12},
		{"empty string", "", true, 0, 0},
		{"no prefix", "report-1-2-20240101-120000.md", true, 0, 0},
		{"too few parts", "incident-1.md", true, 0, 0},
		{"non-numeric issue", "incident-abc-1-20240101-120000.md", true, 0, 0},
		{"non-numeric attempt", "incident-1-xyz-20240101-120000.md", true, 0, 0},
		{"bad timestamp", "incident-1-1-notadate.md", true, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := parseIncidentFilename(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseIncidentFilename(%q) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if info.IssueNumber != tt.wantIssue {
					t.Errorf("IssueNumber = %d, want %d", info.IssueNumber, tt.wantIssue)
				}
				if info.Attempt != tt.wantAtmpt {
					t.Errorf("Attempt = %d, want %d", info.Attempt, tt.wantAtmpt)
				}
			}
		})
	}
}
