package plan

import (
	"testing"
)

// TestIntegration_InvalidPlanParsing tests handling of malformed plan artifacts
func TestIntegration_InvalidPlanParsing(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		wantErr  bool
	}{
		{
			name:     "valid plan",
			markdown: "# Spec\n```kiro-plan\nversion: \"1.0\"\ntasks:\n  - id: task-1\n    agent: builder\n    description: Test\n    dependencies: []\n```",
			wantErr:  false,
		},
		{
			name:     "no plan artifact",
			markdown: "# Spec\nThis is a legacy spec without a plan.",
			wantErr:  false, // Should return nil, nil (not an error)
		},
		{
			name:     "malformed YAML",
			markdown: "# Spec\n```kiro-plan\nversion: 1.0\ntasks:\n  - id: task-1\n    agent: builder\n    description: Test\n    dependencies: [\n```",
			wantErr:  true,
		},
		{
			name:     "multiple plan blocks",
			markdown: "# Spec\n```kiro-plan\nversion: \"1.0\"\ntasks: []\n```\n\n```kiro-plan\nversion: \"1.0\"\ntasks: []\n```",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := ParsePlanFromMarkdown([]byte(tt.markdown))

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				// For "no plan artifact", we expect nil plan
				if tt.name == "no plan artifact" && plan != nil {
					t.Error("Expected nil plan for legacy spec")
				}
			}
		})
	}
}
