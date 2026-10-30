package eval

import (
	"testing"
	"time"
)

func TestPerformanceProfiling(t *testing.T) {
	// Initialize profiling
	StartProfiling()

	// Simulate some test cases
	TrackTestCase("test1", 100*time.Millisecond)
	TrackTestCase("test2", 200*time.Millisecond)
	TrackTestCase("test3", 150*time.Millisecond)

	// Generate profile
	profile := GenerateProfile()

	// Verify basic structure
	if profile == nil {
		t.Fatal("Expected profile to be generated")
	}

	if len(profile.TestCaseTimings) != 3 {
		t.Errorf("Expected 3 test cases, got %d", len(profile.TestCaseTimings))
	}

	if profile.AgentInvocations != 3 {
		t.Errorf("Expected 3 agent invocations, got %d", profile.AgentInvocations)
	}

	// Check test case timings
	if profile.TestCaseTimings["test1"] != 100*time.Millisecond {
		t.Errorf("Expected test1 timing to be 100ms, got %v", profile.TestCaseTimings["test1"])
	}

	if profile.TestCaseTimings["test2"] != 200*time.Millisecond {
		t.Errorf("Expected test2 timing to be 200ms, got %v", profile.TestCaseTimings["test2"])
	}
}

func TestBottleneckDetection(t *testing.T) {
	StartProfiling()

	// Simulate a slow test case
	TrackTestCase("fast_test", 50*time.Millisecond)
	TrackTestCase("slow_test", 500*time.Millisecond) // Much slower

	profile := GenerateProfile()

	// Should detect the slow test as a bottleneck
	found := false
	for _, bottleneck := range profile.Bottlenecks {
		if bottleneck.Component == "test case: slow_test" {
			found = true
			if bottleneck.Severity != "medium" {
				t.Errorf("Expected medium severity for slow test, got %s", bottleneck.Severity)
			}
		}
	}

	if !found {
		t.Error("Expected slow test to be identified as bottleneck")
	}
}

func TestParallelAnalysis(t *testing.T) {
	timings := map[string]time.Duration{
		"test1": 100 * time.Millisecond,
		"test2": 200 * time.Millisecond,
		"test3": 150 * time.Millisecond,
	}

	analysis := analyzeParallelPotential(timings)

	if !analysis.SafeParallelization {
		t.Error("Expected safe parallelization to be true")
	}

	if len(analysis.IndependentTests) != 3 {
		t.Errorf("Expected 3 independent tests, got %d", len(analysis.IndependentTests))
	}

	if analysis.EstimatedSpeedup <= 0 {
		t.Errorf("Expected positive speedup, got %.2f", analysis.EstimatedSpeedup)
	}

	if analysis.RecommendedWorkers <= 0 {
		t.Errorf("Expected positive number of workers, got %d", analysis.RecommendedWorkers)
	}
}
