package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// PerfProfile captures performance metrics for evaluation.
type PerfProfile struct {
	StartupOverhead   time.Duration            `json:"startup_overhead"`
	TestCaseTimings   map[string]time.Duration `json:"test_case_timings"`
	AgentInvocations  int                      `json:"agent_invocations"`
	TotalEvalTime     time.Duration            `json:"total_eval_time"`
	MemoryUsage       MemoryStats              `json:"memory_usage"`
	ParallelPotential ParallelAnalysis         `json:"parallel_potential"`
	Bottlenecks       []Bottleneck             `json:"bottlenecks"`
}

// MemoryStats captures memory usage during evaluation.
type MemoryStats struct {
	MaxHeapMB      float64 `json:"max_heap_mb"`
	MaxStackMB     float64 `json:"max_stack_mb"`
	GoroutinesPeak int     `json:"goroutines_peak"`
}

// ParallelAnalysis evaluates opportunities for parallel execution.
type ParallelAnalysis struct {
	IndependentTests    []string `json:"independent_tests"`
	EstimatedSpeedup    float64  `json:"estimated_speedup"`
	SafeParallelization bool     `json:"safe_parallelization"`
	RecommendedWorkers  int      `json:"recommended_workers"`
}

// Bottleneck identifies performance bottlenecks.
type Bottleneck struct {
	Component   string        `json:"component"`
	Description string        `json:"description"`
	Impact      time.Duration `json:"impact"`
	Severity    string        `json:"severity"` // "low", "medium", "high"
}

// ProfilerState tracks performance metrics during evaluation.
type profilerState struct {
	mu              sync.RWMutex
	startTime       time.Time
	testTimings     map[string]time.Duration
	agentCallCount  int
	memStats        []MemoryStats
	startupMeasured bool
	startupOverhead time.Duration
}

var globalProfiler = &profilerState{
	testTimings: make(map[string]time.Duration),
	memStats:    make([]MemoryStats, 0),
}

// StartProfiling initializes performance profiling for evaluation.
func StartProfiling() {
	globalProfiler.mu.Lock()
	defer globalProfiler.mu.Unlock()

	globalProfiler.startTime = time.Now()
	globalProfiler.testTimings = make(map[string]time.Duration)
	globalProfiler.agentCallCount = 0
	globalProfiler.memStats = make([]MemoryStats, 0)
	globalProfiler.startupMeasured = false
}

// MeasureStartupOverhead profiles the inference backend's startup time.
func MeasureStartupOverhead() time.Duration {
	globalProfiler.mu.Lock()
	defer globalProfiler.mu.Unlock()

	if globalProfiler.startupMeasured {
		return globalProfiler.startupOverhead
	}

	// Measure startup overhead via the configured backend (0 for backends
	// that start no process).
	overhead := cfg.backend.StartupProbe()

	globalProfiler.startupOverhead = overhead
	globalProfiler.startupMeasured = true

	return overhead
}

// TrackTestCase records timing for a test case execution.
func TrackTestCase(testName string, duration time.Duration) {
	globalProfiler.mu.Lock()
	defer globalProfiler.mu.Unlock()

	globalProfiler.testTimings[testName] = duration
	globalProfiler.agentCallCount++

	// Sample memory usage periodically
	if len(globalProfiler.memStats) < 10 || globalProfiler.agentCallCount%5 == 0 {
		globalProfiler.memStats = append(globalProfiler.memStats, captureMemoryStats())
	}
}

// GenerateProfile creates comprehensive performance analysis.
func GenerateProfile() *PerfProfile {
	globalProfiler.mu.RLock()
	defer globalProfiler.mu.RUnlock()

	profile := &PerfProfile{
		StartupOverhead:  globalProfiler.startupOverhead,
		TestCaseTimings:  make(map[string]time.Duration),
		AgentInvocations: globalProfiler.agentCallCount,
		TotalEvalTime:    time.Since(globalProfiler.startTime),
		MemoryUsage:      aggregateMemoryStats(globalProfiler.memStats),
	}

	// Copy test timings
	for name, duration := range globalProfiler.testTimings {
		profile.TestCaseTimings[name] = duration
	}

	// Analyze bottlenecks
	profile.Bottlenecks = identifyBottlenecks(profile)

	// Analyze parallel potential
	profile.ParallelPotential = analyzeParallelPotential(profile.TestCaseTimings)

	return profile
}

// captureMemoryStats samples current memory usage.
func captureMemoryStats() MemoryStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return MemoryStats{
		MaxHeapMB:      float64(m.HeapInuse) / 1024 / 1024,
		MaxStackMB:     float64(m.StackInuse) / 1024 / 1024,
		GoroutinesPeak: runtime.NumGoroutine(),
	}
}

// aggregateMemoryStats finds peak memory usage.
func aggregateMemoryStats(samples []MemoryStats) MemoryStats {
	if len(samples) == 0 {
		return MemoryStats{}
	}

	peak := samples[0]
	for _, sample := range samples[1:] {
		if sample.MaxHeapMB > peak.MaxHeapMB {
			peak.MaxHeapMB = sample.MaxHeapMB
		}
		if sample.MaxStackMB > peak.MaxStackMB {
			peak.MaxStackMB = sample.MaxStackMB
		}
		if sample.GoroutinesPeak > peak.GoroutinesPeak {
			peak.GoroutinesPeak = sample.GoroutinesPeak
		}
	}

	return peak
}

// identifyBottlenecks analyzes performance data to find bottlenecks.
func identifyBottlenecks(profile *PerfProfile) []Bottleneck {
	var bottlenecks []Bottleneck

	// Check startup overhead
	if profile.StartupOverhead > 5*time.Second {
		bottlenecks = append(bottlenecks, Bottleneck{
			Component:   "kiro-cli startup",
			Description: "High startup overhead detected",
			Impact:      profile.StartupOverhead,
			Severity:    "high",
		})
	} else if profile.StartupOverhead > 2*time.Second {
		bottlenecks = append(bottlenecks, Bottleneck{
			Component:   "kiro-cli startup",
			Description: "Moderate startup overhead",
			Impact:      profile.StartupOverhead,
			Severity:    "medium",
		})
	}

	// Check for slow test cases
	if len(profile.TestCaseTimings) == 0 {
		return bottlenecks
	}
	totalTime := profile.TotalEvalTime
	avgTime := totalTime / time.Duration(len(profile.TestCaseTimings))

	for testName, duration := range profile.TestCaseTimings {
		if duration > avgTime*3 {
			bottlenecks = append(bottlenecks, Bottleneck{
				Component:   "test case: " + testName,
				Description: "Significantly slower than average",
				Impact:      duration - avgTime,
				Severity:    "medium",
			})
		}
	}

	// Check memory usage
	if profile.MemoryUsage.MaxHeapMB > 500 {
		bottlenecks = append(bottlenecks, Bottleneck{
			Component:   "memory usage",
			Description: fmt.Sprintf("High memory usage: %.1f MB", profile.MemoryUsage.MaxHeapMB),
			Impact:      0, // Memory is not time-based
			Severity:    "medium",
		})
	}

	return bottlenecks
}

// analyzeParallelPotential evaluates opportunities for parallel execution.
func analyzeParallelPotential(timings map[string]time.Duration) ParallelAnalysis {
	if len(timings) < 2 {
		return ParallelAnalysis{
			SafeParallelization: false,
			EstimatedSpeedup:    1.0,
			RecommendedWorkers:  1,
		}
	}

	// All test cases are independent in current design
	independentTests := make([]string, 0, len(timings))
	var totalDuration time.Duration

	for testName, duration := range timings {
		independentTests = append(independentTests, testName)
		totalDuration += duration
	}

	// Calculate potential speedup with parallel execution
	numCPU := runtime.NumCPU()
	recommendedWorkers := min(len(independentTests), numCPU)

	// Estimate speedup (conservative due to startup overhead)
	estimatedSpeedup := float64(len(independentTests)) / float64(recommendedWorkers) * 0.7 // 70% efficiency

	return ParallelAnalysis{
		IndependentTests:    independentTests,
		EstimatedSpeedup:    estimatedSpeedup,
		SafeParallelization: true, // Test cases are isolated
		RecommendedWorkers:  recommendedWorkers,
	}
}

// InvestigateParallelExecution runs a parallel execution experiment.
func InvestigateParallelExecution(agent string) (*ParallelBenchmark, error) {
	cases, err := loadCases(agent)
	if err != nil {
		return nil, fmt.Errorf("failed to load test cases: %w", err)
	}

	if len(cases) < 2 {
		return &ParallelBenchmark{
			Sequential: 0,
			Parallel:   0,
			Speedup:    1.0,
			Safe:       true,
			Note:       "Too few test cases for meaningful comparison",
		}, nil
	}

	// Take first 2 test cases for quick benchmark
	testCases := cases[:min(2, len(cases))]

	// Sequential execution
	startSeq := time.Now()
	for _, tc := range testCases {
		prompt, err := assemblePrompt(tc.Setup, tc.Input)
		if err != nil {
			continue
		}
		_, _, _, _, invokeErr := invokeAgent(agent, prompt, nil, tc.Stub)
		if invokeErr != nil {
			return nil, fmt.Errorf("sequential benchmark failed: %w", invokeErr)
		}
	}
	seqDuration := time.Since(startSeq)

	// Parallel execution
	startPar := time.Now()
	var wg sync.WaitGroup
	var parallelErr error
	var errMu sync.Mutex
	for _, tc := range testCases {
		wg.Add(1)
		go func(testCase TestCase) {
			defer wg.Done()
			prompt, err := assemblePrompt(testCase.Setup, testCase.Input)
			if err != nil {
				return
			}
			_, _, _, _, invokeErr := invokeAgent(agent, prompt, nil, testCase.Stub)
			if invokeErr != nil {
				errMu.Lock()
				parallelErr = invokeErr
				errMu.Unlock()
			}
		}(tc)
	}
	wg.Wait()
	parDuration := time.Since(startPar)

	if parallelErr != nil {
		return nil, fmt.Errorf("parallel benchmark failed: %w", parallelErr)
	}

	speedup := float64(seqDuration) / float64(parDuration)

	return &ParallelBenchmark{
		Sequential: seqDuration,
		Parallel:   parDuration,
		Speedup:    speedup,
		Safe:       true, // Test cases are isolated
		Note:       fmt.Sprintf("Tested with %d test cases", len(testCases)),
	}, nil
}

// ParallelBenchmark captures parallel execution performance.
type ParallelBenchmark struct {
	Sequential time.Duration `json:"sequential_duration"`
	Parallel   time.Duration `json:"parallel_duration"`
	Speedup    float64       `json:"speedup"`
	Safe       bool          `json:"safe"`
	Note       string        `json:"note"`
}

// SavePerformanceReport writes performance analysis to file.
func SavePerformanceReport(resultsDir string, profile *PerfProfile, benchmark *ParallelBenchmark) error {
	report := struct {
		Profile   *PerfProfile       `json:"profile"`
		Benchmark *ParallelBenchmark `json:"parallel_benchmark,omitempty"`
		Timestamp time.Time          `json:"timestamp"`
	}{
		Profile:   profile,
		Benchmark: benchmark,
		Timestamp: time.Now(),
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal performance report: %w", err)
	}

	reportPath := filepath.Join(resultsDir, "performance.json")
	if err := os.WriteFile(reportPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write performance report: %w", err)
	}

	return nil
}

// PrintPerformanceReport displays performance analysis to console.
func PrintPerformanceReport(profile *PerfProfile, benchmark *ParallelBenchmark) {
	fmt.Println("\n🔍 Performance Analysis")
	fmt.Println("=======================")

	// Startup overhead
	fmt.Printf("Startup Overhead: %v\n", profile.StartupOverhead)
	if profile.StartupOverhead > 2*time.Second {
		fmt.Printf("⚠️  High startup overhead detected\n")
	}

	// Memory usage
	fmt.Printf("Peak Memory: %.1f MB heap, %.1f MB stack\n",
		profile.MemoryUsage.MaxHeapMB, profile.MemoryUsage.MaxStackMB)

	// Test case performance
	fmt.Printf("\nTest Case Performance:\n")
	for testName, duration := range profile.TestCaseTimings {
		fmt.Printf("  %s: %v\n", testName, duration)
	}

	// Bottlenecks
	if len(profile.Bottlenecks) > 0 {
		fmt.Printf("\n🚨 Bottlenecks Identified:\n")
		for _, bottleneck := range profile.Bottlenecks {
			severity := ""
			switch bottleneck.Severity {
			case "high":
				severity = "🔴"
			case "medium":
				severity = "🟡"
			case "low":
				severity = "🟢"
			}
			fmt.Printf("  %s %s: %s", severity, bottleneck.Component, bottleneck.Description)
			if bottleneck.Impact > 0 {
				fmt.Printf(" (impact: %v)", bottleneck.Impact)
			}
			fmt.Println()
		}
	}

	// Parallel potential
	fmt.Printf("\n⚡ Parallelization Analysis:\n")
	fmt.Printf("  Independent tests: %d\n", len(profile.ParallelPotential.IndependentTests))
	fmt.Printf("  Estimated speedup: %.1fx\n", profile.ParallelPotential.EstimatedSpeedup)
	fmt.Printf("  Recommended workers: %d\n", profile.ParallelPotential.RecommendedWorkers)
	fmt.Printf("  Safe for parallelization: %v\n", profile.ParallelPotential.SafeParallelization)

	// Parallel benchmark results
	if benchmark != nil {
		fmt.Printf("\n🏃 Parallel Execution Benchmark:\n")
		fmt.Printf("  Sequential: %v\n", benchmark.Sequential)
		fmt.Printf("  Parallel: %v\n", benchmark.Parallel)
		fmt.Printf("  Actual speedup: %.1fx\n", benchmark.Speedup)
		if benchmark.Note != "" {
			fmt.Printf("  Note: %s\n", benchmark.Note)
		}
	}

	// Recommendations
	fmt.Printf("\n💡 Recommendations:\n")
	if profile.StartupOverhead > 2*time.Second {
		fmt.Printf("  • Consider kiro-cli daemon mode to reduce startup overhead\n")
	}
	if profile.ParallelPotential.EstimatedSpeedup > 1.5 {
		fmt.Printf("  • Implement parallel test execution with %d workers\n", profile.ParallelPotential.RecommendedWorkers)
	}
	if profile.MemoryUsage.MaxHeapMB > 200 {
		fmt.Printf("  • Monitor memory usage for large evaluation suites\n")
	}

	fmt.Printf("  • Consider result caching for unchanged test cases\n")
	fmt.Printf("  • Profile individual criterion evaluation for optimization\n")
}
