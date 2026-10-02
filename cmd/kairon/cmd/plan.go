package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jbrinkman/kairon/internal/plan"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Plan execution utilities for krew-lead agent",
	Long:  `Parse, validate, and analyze execution plans from architect specs`,
}

var planParseCmd = &cobra.Command{
	Use:   "parse <spec-file>",
	Short: "Parse and validate a plan from a spec file",
	Long:  `Extracts the kiro-plan artifact from a spec file, validates it, and outputs execution layers as JSON`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		specFile := args[0]

		// Read the spec file. A genuine file/IO error (missing or unreadable
		// file) is an environmental failure the architect cannot fix by
		// regenerating the plan, so it stays a hard error.
		content, err := os.ReadFile(specFile)
		if err != nil {
			return fmt.Errorf("failed to read spec file: %w", err)
		}

		// Parse the plan artifact from the file content. A malformed artifact
		// (bad YAML, unclosed block, multiple plan blocks) is a validation-class
		// problem: emit the same machine-readable validation_failed result that
		// the krew-lead prompt consumes so the architect-retry path is taken,
		// rather than a Cobra error that would bypass it.
		parsedPlan, err := plan.ParsePlanFromMarkdown(content)
		if err != nil {
			output := map[string]interface{}{
				"status": "validation_failed",
				"error":  err.Error(),
			}
			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
			return nil // Don't return error - we want to output JSON
		}

		if parsedPlan == nil {
			// No plan found - output special marker for backward compatibility
			fmt.Println(`{"status":"no_plan"}`)
			return nil
		}

		// Discover agents for validation
		agentDir := ".kiro/agents"
		registry, err := plan.DiscoverAgents(agentDir)
		if err != nil {
			return fmt.Errorf("failed to discover agents: %w", err)
		}

		// Validate the plan
		validator := plan.NewValidator(registry)
		if err := validator.ValidatePlan(parsedPlan); err != nil {
			// Output validation errors as JSON
			output := map[string]interface{}{
				"status": "validation_failed",
				"error":  err.Error(),
			}
			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
			return nil // Don't return error - we want to output JSON
		}

		// Perform topological sort to get execution layers
		layers, err := plan.TopologicalSort(parsedPlan)
		if err != nil {
			return fmt.Errorf("topological sort failed: %w", err)
		}

		// Build output structure with execution layers
		type TaskInfo struct {
			ID                 string   `json:"id"`
			Agent              string   `json:"agent"`
			Description        string   `json:"description"`
			Dependencies       []string `json:"dependencies"`
			AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
			ValidationCommands []string `json:"validation_commands,omitempty"`
		}

		type LayerInfo struct {
			Layer int        `json:"layer"`
			Tasks []TaskInfo `json:"tasks"`
		}

		output := map[string]interface{}{
			"status":      "valid",
			"total_tasks": len(parsedPlan.Tasks),
			"layers":      make([]LayerInfo, 0),
		}

		layersList := make([]LayerInfo, 0)
		for i, layer := range layers {
			layerTasks := make([]TaskInfo, 0)
			for _, task := range layer {
				layerTasks = append(layerTasks, TaskInfo{
					ID:                 task.ID,
					Agent:              task.Agent,
					Description:        task.Description,
					Dependencies:       task.Dependencies,
					AcceptanceCriteria: task.AcceptanceCriteria,
					ValidationCommands: task.ValidationCommands,
				})
			}
			layersList = append(layersList, LayerInfo{
				Layer: i,
				Tasks: layerTasks,
			})
		}
		output["layers"] = layersList

		// Output as formatted JSON
		jsonBytes, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal output: %w", err)
		}

		fmt.Println(string(jsonBytes))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(planCmd)
	planCmd.AddCommand(planParseCmd)
}
