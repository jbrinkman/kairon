#!/usr/bin/env bash
#
# Integration Test Script for Plan-and-Execute System
# Tests the complete workflow from spec parsing to plan execution
#

set -e

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== Kiro-Krew Plan-and-Execute Integration Tests ===${NC}\n"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Private temp file for per-test output capture. Using mktemp (instead of a
# fixed /tmp/test_output.log) avoids a predictable path that could be
# pre-created as a symlink on a shared host and followed on redirect (CWE-377).
TEST_OUTPUT="$(mktemp "${TMPDIR:-/tmp}/kiro-krew-itest.XXXXXX")"

# Helper functions
run_test() {
    local test_name="$1"
    local test_cmd="$2"
    
    TESTS_RUN=$((TESTS_RUN + 1))
    echo -e "${YELLOW}Running:${NC} $test_name"
    
    if eval "$test_cmd" > "$TEST_OUTPUT" 2>&1; then
        echo -e "${GREEN}✓ PASS${NC}: $test_name"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}✗ FAIL${NC}: $test_name"
        echo "Output:"
        cat "$TEST_OUTPUT"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        # Do not return non-zero here: under `set -e` a non-zero return from this
        # bare `run_test` call would abort the whole script on the first failing
        # test and skip every later phase. Record the failure and let the
        # print_summary EXIT trap produce the final nonzero status.
        return 0
    fi
}

print_summary() {
    echo ""
    echo -e "${BLUE}=== Test Summary ===${NC}"
    echo "Total tests run: $TESTS_RUN"
    echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
    
    if [ $TESTS_FAILED -gt 0 ]; then
        echo -e "${RED}Failed: $TESTS_FAILED${NC}"
        exit 1
    else
        echo -e "\n${GREEN}All tests passed!${NC}"
        exit 0
    fi
}

# Clean up the temp output file on exit, then print the summary. print_summary
# calls exit, so cleanup must run first; chain them in the trap rather than
# complicating print_summary itself.
cleanup() { rm -f "$TEST_OUTPUT"; }
trap 'cleanup; print_summary' EXIT

echo -e "${BLUE}Phase 1: Unit Tests${NC}\n"

# Test 1: Plan types and schema validation
run_test "Plan types and schema validation" \
    "go test ./internal/plan -run TestPlanValidate -v"

run_test "Plan schema validation" \
    "go test ./internal/plan -run TestCompatibility_SchemaValidationErrors -v"

# Test 2: Agent registry
run_test "Agent registry discovery" \
    "go test ./internal/plan -run TestAgentRegistry -v"

run_test "Registry discovery from filesystem" \
    "go test ./internal/plan -run TestDiscoverAgents -v"

# Test 3: Plan parser
run_test "Plan parser functionality" \
    "go test ./internal/plan -run TestParsePlanFromMarkdown -v"

run_test "Parser edge cases" \
    "go test ./internal/plan -run TestParsePlanFromFile -v"

# Test 4: Plan validator
run_test "Plan validator" \
    "go test ./internal/plan -run TestValidator -v"

run_test "Cycle detection" \
    "go test ./internal/plan -run TestValidatorDetects -v"

run_test "Dependency resolution" \
    "go test ./internal/plan -run TestValidatorWithMissingDependency -v"

# Test 5: Plan executor
run_test "Plan executor" \
    "go test ./internal/plan -run TestExecutor -v"

run_test "Topological sort" \
    "go test ./internal/plan -run TestTopologicalSort -v"

run_test "Parallel execution" \
    "go test ./internal/plan -run TestExecutorParallelExecution -v"

echo -e "\n${BLUE}Phase 2: Integration Tests${NC}\n"

# Test 6: Integration tests
run_test "Parallel task execution (integration)" \
    "go test ./internal/plan -run TestIntegration_ParallelExecution -v"

run_test "Sequential task execution (integration)" \
    "go test ./internal/plan -run TestIntegration_SequentialExecution -v"

run_test "Mixed dependencies (integration)" \
    "go test ./internal/plan -run TestIntegration_MixedDependencies -v"

run_test "Invalid plan handling (integration)" \
    "go test ./internal/plan -run TestIntegration_InvalidPlanParsing -v"

run_test "Task failure handling (integration)" \
    "go test ./internal/plan -run TestIntegration_TaskFailureHandling -v"

run_test "End-to-end workflow (integration)" \
    "go test ./internal/plan -run TestIntegration_EndToEnd -v"

echo -e "\n${BLUE}Phase 3: Backward Compatibility Tests${NC}\n"

# Test 7: Backward compatibility
run_test "Legacy spec without plan" \
    "go test ./internal/plan -run TestCompatibility_LegacySpecWithoutPlan -v"

run_test "Spec with non-plan code blocks" \
    "go test ./internal/plan -run TestCompatibility_SpecWithNonPlanCodeBlocks -v"

run_test "Mixed plan and legacy content" \
    "go test ./internal/plan -run TestCompatibility_MixedPlanAndLegacyContent -v"

run_test "Parser robustness" \
    "go test ./internal/plan -run TestCompatibility_ParserRobustness -v"

run_test "Validation backward compatibility" \
    "go test ./internal/plan -run TestCompatibility_ValidationBackwardCompatibility -v"

run_test "Executor empty plan" \
    "go test ./internal/plan -run TestCompatibility_ExecutorEmptyPlan -v"

run_test "Legacy workflow simulation" \
    "go test ./internal/plan -run TestCompatibility_LegacyWorkflowSimulation -v"

run_test "Plan-based workflow simulation" \
    "go test ./internal/plan -run TestCompatibility_PlanBasedWorkflowSimulation -v"

run_test "Schema validation errors" \
    "go test ./internal/plan -run TestCompatibility_SchemaValidationErrors -v"

run_test "Topological sort stability" \
    "go test ./internal/plan -run TestCompatibility_TopologicalSortStability -v"

echo -e "\n${BLUE}Phase 4: Full Test Suite${NC}\n"

# Test 8: Run all plan package tests
run_test "All plan package tests" \
    "go test ./internal/plan/... -v -race -coverprofile=coverage.out"

# Test 9: Coverage check
run_test "Code coverage threshold (>85%)" \
    "go tool cover -func=coverage.out | grep total | awk '{if (\$3+0 >= 85.0) exit 0; else exit 1}'"

echo -e "\n${BLUE}Phase 5: Build Verification${NC}\n"

# Test 10: Build verification
run_test "Application builds successfully" \
    "go build ./cmd/kiro-krew"

echo -e "\n${BLUE}Phase 6: Existing Test Suite${NC}\n"

# Test 11: Verify no regressions in existing tests
run_test "All existing tests pass (no regressions)" \
    "go test ./... -short -v"

# Summary will be printed by trap
