# Performance Optimization Investigation

## Overview

Task 7 of the enhanced evaluation framework introduces comprehensive performance profiling and optimization investigation capabilities. This feature helps identify bottlenecks, evaluate parallel execution opportunities, and provides recommendations for improving evaluation performance.

## Features

### 1. Startup Overhead Profiling
- Measures kiro-cli startup time across multiple samples
- Identifies startup bottlenecks that affect total evaluation time
- Provides variance analysis to detect system load factors

### 2. Test Case Performance Tracking
- Records execution time for each test case
- Identifies slow test cases relative to average performance
- Tracks memory usage during evaluation

### 3. Parallel Execution Analysis
- Evaluates potential for safe parallel test execution
- Benchmarks sequential vs parallel performance
- Recommends optimal worker count based on system resources

### 4. Bottleneck Detection
- Automatically identifies performance bottlenecks
- Categorizes severity levels (low/medium/high)
- Provides actionable recommendations

### 5. Comprehensive Performance Reporting
- Saves detailed JSON performance reports
- Displays console summaries with key metrics
- Includes recommendations for optimization

## Usage

### Basic Performance Profiling
All evaluation runs now include automatic performance profiling:

```bash
# Regular evaluation includes performance metrics
kairon eval architect

# Single test case with performance tracking
kairon eval architect basic-spec-generation
```

### Dedicated Performance Investigation
Run comprehensive performance analysis:

```bash
# Full performance investigation for an agent
kairon eval architect --perf
```

This mode:
- Measures startup overhead with multiple samples
- Analyzes test case complexity
- Benchmarks parallel vs sequential execution
- Generates detailed performance report
- Provides optimization recommendations

## Performance Report Structure

The performance report (`performance.json`) includes:

```json
{
  "profile": {
    "startup_overhead": "2.5s",
    "test_case_timings": {
      "test1": "15s",
      "test2": "22s"
    },
    "agent_invocations": 2,
    "total_eval_time": "45s",
    "memory_usage": {
      "max_heap_mb": 125.5,
      "max_stack_mb": 8.2,
      "goroutines_peak": 15
    },
    "parallel_potential": {
      "independent_tests": ["test1", "test2"],
      "estimated_speedup": 1.8,
      "safe_parallelization": true,
      "recommended_workers": 4
    },
    "bottlenecks": [
      {
        "component": "kiro-cli startup",
        "description": "Moderate startup overhead",
        "impact": "2.5s",
        "severity": "medium"
      }
    ]
  },
  "parallel_benchmark": {
    "sequential_duration": "30s",
    "parallel_duration": "18s", 
    "speedup": 1.67,
    "safe": true,
    "note": "Tested with 2 test cases"
  },
  "timestamp": "2026-06-20T13:24:05Z"
}
```

## Key Metrics

### Startup Overhead
- **Low**: < 2 seconds (good)
- **Medium**: 2-5 seconds (monitor)
- **High**: > 5 seconds (investigate)

### Memory Usage
- **Normal**: < 200 MB heap
- **Elevated**: 200-500 MB heap 
- **High**: > 500 MB heap (investigate)

### Parallel Speedup
- **Excellent**: > 2.0x speedup
- **Good**: 1.5-2.0x speedup
- **Limited**: < 1.5x speedup (startup overhead dominant)

## Optimization Recommendations

### Common Bottlenecks and Solutions

1. **High Startup Overhead**
   - Consider kiro-cli daemon mode
   - Batch test cases to amortize startup cost
   - Pre-warm agent models if supported

2. **Memory Usage Issues**
   - Monitor heap growth for large test suites
   - Consider streaming evaluation for large outputs
   - Review test case complexity

3. **Limited Parallel Benefits**
   - Startup overhead may dominate for short tests
   - Focus on optimizing individual test performance
   - Consider test case grouping strategies

### Safe Parallel Execution Strategy

The current evaluation framework supports safe parallelization because:
- Test cases are independent (no shared state)
- Each invocation uses isolated kiro-cli processes
- File I/O is managed through atomic operations
- Result aggregation handles concurrent updates

Recommended parallel execution approach:
1. Group test cases by estimated duration
2. Use worker pools sized to CPU count
3. Maintain progress tracking across workers
4. Aggregate results atomically

## Integration with Existing Features

Performance profiling integrates seamlessly with:
- **Progressive Saving**: Tracks save performance overhead
- **Error Handling**: Profiles error recovery time
- **Resume Functionality**: Measures resume detection overhead
- **Selective Execution**: Optimizes single test performance

## Future Enhancements

Potential optimizations identified:
1. **Result Caching**: Skip unchanged test cases
2. **Incremental Evaluation**: Smart test selection
3. **Agent Pooling**: Reduce startup overhead
4. **Streaming Results**: Handle large outputs efficiently
5. **Distributed Execution**: Scale across multiple machines

## File Locations

- Performance module: `internal/eval/perf.go`
- Test coverage: `internal/eval/perf_test.go`
- Integration: Modified `internal/eval/runner.go`
- CLI support: Enhanced `cmd/kairon/cmd/eval.go`
- Reports saved: `.kairon/evals/results/<timestamp>/performance.json`