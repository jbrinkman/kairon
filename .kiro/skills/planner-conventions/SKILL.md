---
name: planner-conventions
description: Comprehensive analysis methodology and conventions for deep root cause investigation. Transforms symptom-level reports into root cause understanding through systematic code tracing and hypothesis validation.
---

# Planner Analysis Methodology

Advanced root cause analysis techniques for transforming surface-level issue reports into deep understanding of underlying problems.

## Root Cause Analysis Process

### Phase 1: Initial Assessment
1. **Symptom Collection**: Document all reported symptoms, error messages, and observable behaviors
2. **Context Mapping**: Identify when, where, and under what conditions symptoms occur
3. **Impact Scope**: Determine breadth of affected functionality or users
4. **Evidence Gathering**: Collect logs, stack traces, and reproduction steps

### Phase 2: Code Tracing Investigation
1. **Error Point Location**: Find exact source location of error/symptom in codebase
2. **Call Stack Analysis**: Trace execution path backwards from error point
3. **Data Flow Tracking**: Follow data transformations that lead to problematic state
4. **Dependency Mapping**: Identify external dependencies, APIs, or services involved
5. **Configuration Analysis**: Examine relevant config files, environment variables, build settings

### Phase 3: Hypothesis Formation
1. **Root Cause Candidates**: Generate 3-5 potential underlying causes
2. **Hypothesis Ranking**: Prioritize by likelihood and impact
3. **Test Design**: Plan specific experiments to validate/invalidate each hypothesis
4. **Success Criteria**: Define what evidence would confirm each hypothesis

### Phase 4: Hypothesis Validation
1. **Planning Worktree Creation**: Use isolated environment for testing
2. **Controlled Experiments**: Test one hypothesis at a time
3. **Evidence Collection**: Document results of each test
4. **Iterative Refinement**: Adjust hypotheses based on evidence

## Code Tracing Techniques

### Error Backward Tracing
```
ERROR MESSAGE → Exception Location → Calling Function → Data Source → Root Cause
```

### Data Flow Investigation
1. **Input Validation**: Check data at system boundaries
2. **Transformation Points**: Examine where data changes format/structure
3. **State Mutations**: Identify where shared state gets modified
4. **Output Generation**: Trace how results are produced

### Dependency Chain Analysis
1. **Direct Dependencies**: Libraries, frameworks, APIs
2. **Indirect Dependencies**: Configuration, environment, infrastructure  
3. **Temporal Dependencies**: Timing, ordering, race conditions
4. **Resource Dependencies**: Memory, disk, network, CPU

## Planning Worktree Usage

### Creation Guidelines
```bash
# Create planning worktree for investigation
.kairon/scripts/planning-worktree-create.sh
# Returns: /path/to/planning/worktree
```

### Investigation Activities (Allowed in Planning Worktree)
- **Code Modifications**: Add debug logging, instrumentation
- **Test Creation**: Write minimal reproduction cases  
- **Configuration Changes**: Temporarily modify settings for testing
- **Dependency Testing**: Add/remove dependencies to isolate issues
- **Environment Simulation**: Replicate production conditions

### Cleanup Requirements
```bash
# Mandatory cleanup after investigation
.kairon/scripts/planning-worktree-cleanup.sh <worktree-path>
```

**Critical**: Planning worktrees must NEVER affect main branch. All changes are investigative only.

## Hypothesis Validation Methods

### Code Path Testing
1. **Minimal Reproduction**: Create simplest case that triggers issue
2. **Variable Isolation**: Change one factor at a time
3. **Boundary Conditions**: Test edge cases and limits
4. **State Verification**: Confirm intermediate states match expectations

### Integration Testing  
1. **Mock Dependencies**: Replace external services with controlled responses
2. **Configuration Variants**: Test different configuration combinations
3. **Load Simulation**: Test under various load conditions
4. **Error Injection**: Introduce controlled failures to test resilience

### Historical Analysis
1. **Git History**: When was problematic code introduced?
2. **Change Correlation**: What changes coincided with symptom appearance?
3. **Regression Testing**: Does reverting specific changes resolve symptoms?

## Decision Framework: Root Cause vs Symptom

When investigation reveals root cause differs from reported symptom:

### Option Presentation Format
```
## Analysis Results

**Reported Symptom**: [User's description]
**Root Cause Discovered**: [Technical root cause]

### Implementation Options:

a) **Address Reported Symptom**: [Surface-level fix description]
   - Pros: Quick resolution, matches user expectation  
   - Cons: Underlying issue persists, potential for recurrence

b) **Address Root Cause**: [Comprehensive fix description]  
   - Pros: Permanent resolution, prevents related issues
   - Cons: Larger scope, may affect other functionality

c) **Hybrid Approach**: [Combined strategy]
   - Immediate symptom relief + planned root cause fix
```

### User Decision Authority
- Present technical analysis clearly
- Explain implications of each approach
- Respect user's final choice
- Document reasoning regardless of selected option

## Quality Validation

### Analysis Completeness Checklist
- [ ] Symptom fully documented with reproduction steps
- [ ] Code path traced from symptom to source
- [ ] At least 3 potential root causes identified  
- [ ] Hypotheses tested in planning worktree
- [ ] Evidence collected for preferred explanation
- [ ] Impact and scope clearly defined
- [ ] Solution options presented with trade-offs

### Investigation Depth Indicators
- **Surface Level**: "Error happens when..."
- **Intermediate**: "Error happens because X calls Y with Z..."  
- **Deep Analysis**: "Error happens because configuration A causes component B to initialize state C, leading to invalid input D when condition E occurs..."

## Common Investigation Patterns

### Configuration Issues
1. **Environment Mismatches**: Development vs production config differences
2. **Default Value Problems**: Missing or incorrect default configurations
3. **Override Conflicts**: Multiple configuration sources with conflicting values

### Integration Issues  
1. **API Contract Changes**: External service modifications breaking assumptions
2. **Version Compatibility**: Library updates introducing breaking changes
3. **Network/Infrastructure**: Connectivity, timeout, or resource limit issues

### Logic Issues
1. **Race Conditions**: Timing-dependent behavior in concurrent code  
2. **State Management**: Shared state corruption or invalid transitions
3. **Edge Cases**: Unhandled boundary conditions or input validation gaps

### Resource Issues
1. **Memory Leaks**: Gradual resource exhaustion over time
2. **Disk Space**: Storage limitations affecting functionality  
3. **Performance Degradation**: Algorithmic complexity issues under load

## Reporting Standards

### Root Cause Report Structure
```markdown
## Root Cause Analysis Report

### Symptom Summary
- **What**: Observable behavior
- **When**: Timing/conditions  
- **Where**: Affected components
- **Impact**: User/system effects

### Investigation Findings
- **Code Path**: [Error location → Root source]  
- **Contributing Factors**: [Environmental/configuration issues]
- **Evidence**: [Test results, logs, traces]

### Root Cause
**Primary Cause**: [Technical explanation]
**Contributing Factors**: [Secondary issues that enable primary cause]  

### Recommended Solution
**Approach**: [Root cause vs symptom fix reasoning]
**Implementation**: [High-level solution strategy]
**Risk Assessment**: [Potential impacts and mitigations]
```

This methodology ensures thorough investigation that distinguishes between symptoms and actual root causes, leading to more effective and lasting solutions.
