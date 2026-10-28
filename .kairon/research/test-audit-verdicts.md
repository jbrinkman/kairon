# Test audit: per-test verdicts

Companion to [test-performance-analysis.md](test-performance-analysis.md) §8. One row per candidate test flagged by the per-test coverage measurement (2026-10-28). Verdicts are reviewer judgments from reading each test and its partner; treat DELETE and MERGE as proposals to confirm when the change is made.


## Group: eval-a

### internal/eval candidate verdicts (eval-a)

Scope: 187 candidate tests from `/tmp/testaudit/review/eval-a.json`. Each one was judged by reading it and the test it is said to be subsumed by. Tests showing `blocks=6` cover only package-init blocks, so they are effectively zero-coverage. Partners marked `cmd` are in `cmd/kairon/cmd`.

| test | file:line | secs | verdict | partner | reason |
|---|---|---|---|---|---|
| TestAttemptAccountingRepeats | attempts_test.go:37 | 0.08 | KEEP | - | Pins that repeats count as attempts not cases and success_rate is pooled over attempts (documented verdict semantics). |
| TestAttemptAccountingLegacyResultRecomputed | attempts_test.go:62 | 0.07 | KEEP | - | Legacy results with Passed==nil must be re-judged against the threshold; distinct branch with real backward-compat value. |
| TestAttemptAccountingNoOutputIsFailedAttempt | attempts_test.go:74 | 0.07 | KEEP | - | Pins that stampCase marks an empty-output case failed even at full marks, which feeds the attempt count. |
| TestAttemptAccountingZeroResults | attempts_test.go:85 | 0.07 | KEEP | - | Guards the zero-denominator case (no NaN success_rate in summary.json). |
| TestVerdictPassesWithSuccessRateBelow100 | attempts_test.go:99 | 0.1 | KEEP | - | Pins the key documented rule that the verdict passes on the pooled score even when individual attempts failed. |
| TestAttemptCountsIndependentOfFloorAndThreshold | attempts_test.go:122 | 0.09 | KEEP | - | Guards that attempt counts come from per-case Passed, not the agent threshold or floor; cheap and specific. |
| TestAuditFixtureStructure | audit_test.go:275 | 0.07 | KEEP | - | docs/evaluation.md names no-rtcc/partial/wrong-order as the fixtures that isolate the structure rule; this is the only test pinning that documented behavior. |
| TestAuditUnmappedFixture | audit_test.go:375 | 0.08 | KEEP | - | Pins the documented unmapped fixture's exact violation lines and report totals (docs example). |
| TestAuditErrorFormat | audit_test.go:624 | 0.07 | KEEP | - | Pins the file:line: rule: message format including the line-less form, which the docs and agents parse. |
| TestAuditUncheckedCriterion | audit_test.go:1297 | 0.14 | KEEP | - | Covers the unchecked rule's real edge cases (cost exclusion, second case, unknown criteria, padded names) that TestAuditReportCriteria does not assert. |
| TestInvokeAgentDefaultBackendCommandLine | backend_routing_test.go:46 | 0.16 | KEEP | - | Pins the exact kiro-cli argv (chat --agent X --no-interactive --trust-all-tools), ANSI stripping and estimated cost: a real external-CLI contract. |
| TestScoreLLMJudgeDefaultBackend | backend_routing_test.go:131 | 0.17 | KEEP | - | Pins judge argv (chat --no-interactive) and JSON-delimiter parsing on the default backend; partner only checks prompt text. |
| TestMeasureStartupOverheadUsesBackendProbe | backend_routing_test.go:216 | 0.15 | KEEP | - | Behavioral check that the stub backend spawns no process and the default backend probes --version. |
| TestLoadRunInfoCarriesPromptFile | candidate_resume_diff_test.go:78 | 0.07 | KEEP | - | Pins the summary-first, agent-file-fallback lookup of prompt_file used by diff. |
| TestApplyToRecordsPromptFile | candidate_test.go:332 | 0.04 | KEEP | - | Cheap unit for pin->result copy and the documented key-absent-without-candidate rule. |
| TestStageKiroWithoutCandidateUsesLivePrompt | candidate_test.go:522 | 0.16 | KEEP | - | Negative control for candidate staging: without --prompt-file the live and overlay prompts are staged. |
| TestCaseCheckInputCarriesUserTurns | checks_judge_test.go:69 | 0.05 | KEEP | - | Pins that check input carries the sent user turns and per-turn evidence. |
| TestScoreCaseChecksOnlyNeverJudgesOrRunsHeuristic | checks_only_test.go:95 | 0.05 | KEEP | - | Only test where an unchecked LLM criterion exists under checks_only; proves no judge call and no heuristic score (partner has clarity checked). |
| TestLoadCasesChecksOnlyRejectsCheckLessCase | checks_only_test.go:223 | 0.11 | KEEP | - | Pins errInvalidChecks wrapping and file naming at the loadCases level. |
| TestChecksYAMLDecodesFieldNames | checks_test.go:45 | 0.05 | KEEP | - | Pins the user-authored case YAML schema (expect_exit, inject, allow, pattern). |
| TestChecksCriterionScoreJSON | checks_test.go:259 | 0.05 | KEEP | - | Pins the documented per-check result keys and that results without checks omit the key. |
| TestDefaultConfig | config_test.go:45 | 0.04 | KEEP | - | Pins the default evals dir and kiro-cli default backend that users rely on. |
| TestConfigureUnknownBackend | config_test.go:97 | 0.05 | KEEP | - | Error lists valid backends and config is left untouched on failure (unique assertion). |
| TestRebaseEvalsPath | config_test.go:124 | 0.05 | KEEP | - | Table of real prefix edge cases (no trailing slash, evalsx, absolute paths). |
| TestCostFromUsage | config_test.go:347 | 0.05 | KEEP | - | Pins pricing math and usage-source propagation. |
| TestRunnerHasNoRuntimeInstalls | container_runner_test.go:40 | 0.05 | KEEP | - | Source guard that runner.go never calls sandbox.Container.CopyTo (which still exists) or the removed install helpers; protects a containment property. |
| TestAgentExecerOnlyExecs | container_runner_test.go:55 | 0.05 | KEEP | - | Type-level boundary: the agent seam exposes only ExecWithStdin. |
| TestPrepareBaseImageFailsFastWithoutKaironBinary | container_runner_test.go:204 | 0.05 | KEEP | - | User-facing fail-fast error naming KAIRON_SANDBOX_BINARY and the platform, with config left untouched. |
| TestPrepareBaseImageNativeRunSkipsPreflight | container_runner_test.go:235 | 0.05 | KEEP | - | Guards that native runs never require a linux binary. |
| TestContainerConfigKeepAliveIsEffectivelyUnbounded | container_runner_test.go:331 | 0.05 | KEEP | - | Regression guard for the old 1h keep-alive that killed long multi-turn cases; plain-integer check for BusyBox. |
| TestContainerRequestWorkDirIsContainerPath | container_runner_test.go:508 | 0.07 | KEEP | - | Pins that the wire request uses the container workspace path, not the host path (real bug class). |
| TestContainerSessionThreeTurnsOneOpenOneClose | container_session_test.go:67 | 0.24 | KEEP | - | Core multi-turn container contract: one session open, N runs, one clean close, outputs per turn. |
| TestContainerSessionNativeMultiTurnOpensNothing | container_session_test.go:234 | 0.23 | KEEP | - | Guards that native multi-turn cases never open a container session. |
| TestContainmentFixtures | containment_sandbox_test.go:64 | 0.1 | KEEP | - | Pins the live .kairon/evals containment cases (requires_sandbox, gh_issue, mock-aws) that the sandbox evals depend on. |
| TestContainmentMockCLIFixtureParity | containment_sandbox_test.go:121 | 0.05 | KEEP | - | Live/self-test fixture byte-sync contract (zero Go coverage expected). |
| TestApplyRunContextContainerVsNative | containment_test.go:30 | 0.04 | KEEP | - | Pins mode/containment recording and that provenance is identical across modes. |
| TestApplyRunContextClearsStaleContainment | containment_test.go:67 | 0.04 | KEEP | - | Guards against a resumed result keeping stale containment on a native run. |
| TestContainmentToolTrustSerialisesEmptyArray | containment_test.go:90 | 0.04 | KEEP | - | Docs promise tool_trust [] is always written; pins it at the struct level. |
| TestCoverageStampFullRun | coverage_test.go:74 | 0.05 | KEEP | - | Host for the stampCoverage table (suite hash, complete, empty case_filter). |
| TestCoverageAgentResultJSON | coverage_test.go:153 | 0.05 | KEEP | - | Docs say complete:false and case_filter:"" are written explicitly and absent when unrecorded; pins that rule. |
| TestCoverageUpdateIncrementalSummaryCopiesFields | coverage_test.go:231 | 0.05 | KEEP | - | Pins that SuiteSHA256 alone creates the summary agents entry, with explicit false/"" values. |
| TestAgentVerdictFloorUnsetIsUnchanged | criterion_floor_test.go:29 | 0.05 | KEEP | - | Documented 'no floor, no change' rule and omitted floor keys in verdict JSON. |
| TestAgentVerdictFloorFailsLowCriterionDespiteScore | criterion_floor_test.go:46 | 0.05 | KEEP | - | Core floor rule: fails with score >= threshold, score unchanged, no case failed; Boundary uses threshold 1 and does not cover this. |
| TestAgentCriterionTotals | criterion_totals_test.go:15 | 0.04 | KEEP | - | Table of pooling rules (repeats, skipped, zero max, first-seen order). |
| TestAgentCriterionTotalsMatchAgentScoreTotals | criterion_totals_test.go:92 | 0.05 | KEEP | - | Property test of the documented invariant that criteria sum to the agent score totals. |
| TestAgentVerdictCriteria | criterion_totals_test.go:110 | 0.05 | KEEP | - | Pins criteria on the verdict while score, pass and case accounting are unchanged. |
| TestAgentVerdictCriteriaEmptyIsJSONArray | criterion_totals_test.go:131 | 0.05 | KEEP | - | Docs say criteria is always written and [] when empty. |
| TestPrintAgentVerdictCriteria | criterion_totals_test.go:141 | 0.04 | KEEP | - | Pins the documented per-criterion verdict lines (grep-able format). |
| TestDiffFilterReportsOnlyMatchingCases | diff_filter_test.go:102 | 0.05 | KEEP | - | Main filtered-diff assertion: scores and costs recomputed from matching cases only. |
| TestDiffNativeVsContainerReportsModeDifference | diff_provenance_test.go:101 | 0.05 | KEEP | - | Pins the user-facing 'not directly comparable' warning and block order. |
| TestDiffLegacyRunShowsUnknownMode | diff_provenance_test.go:270 | 0.05 | KEEP | - | Only test for two unknown-mode runs (no warning). |
| TestDiffToWritesOnlyToWriter | diff_provenance_test.go:385 | 0.05 | KEEP | - | Guards that diffTo never writes to stdout (TUI embedding). |
| TestRunFixturesShapes | diff_provenance_test.go:451 | 0.05 | KEEP | - | Sanity checks the checked-in run fixtures other diff tests rely on, including exact totals. |
| TestExecuteCaseWorkspaceExistsWhileScoring | execute_case_test.go:109 | 0.24 | KEEP | - | Ordering contract: workspace (with agent output) exists during scoring and is removed after. |
| TestConfigureKeepWorkspaces | execute_case_test.go:204 | 0.04 | KEEP | - | Host for the configure-resets-run-scoped-fields table. |
| TestPrintAgentVerdictFormat | exit_status_test.go:49 | 0.04 | KEEP | - | Pins the documented grep-able PASS/FAIL verdict line format. |
| TestThresholdErrorWrapsSentinelAndNamesAgents | exit_status_test.go:69 | 0.05 | KEEP | - | errors.Is(ErrThresholdFailed) drives the CLI exit code; message names failing agents. |
| TestRunFilterRunsOnlyMatchingCases | filter_run_test.go:179 | 0.42 | KEEP | - | Primary end-to-end check that --filter selects cases (4-case fixture, cheap). |
| TestRunSkippedAgentModelDoesNotBlockFilteredRun | filter_run_test.go:510 | 0.05 | KEEP | - | Pins scopeAgents precedence in agentsInScope (name is misleading; consider renaming). |
| TestFilteredRunRecordsIncompleteAndLabel | filter_run_test.go:629 | 0.34 | KEEP | - | Pins the documented canonical sorted AND label and complete=false in both files. |
| TestStampCaseCopiesMetadata | filter_run_test.go:772 | 0.05 | KEEP | - | Guards against the result aliasing the case's metadata map. |
| TestResumeRefusesChangedOrMissingFilter | filter_run_test.go:885 | 1.7 | KEEP | - | Core resume-refusal contract over three filter changes; already uses a 4-case fixture. |
| TestPrintTestCaseListMetadataLinesSortedAndUnchangedWhenUntagged | filter_run_test.go:1054 | 0.19 | KEEP | - | Pins the user-facing --list output with sorted metadata. |
| TestFilterParseOperators | filter_test.go:18 | 0.05 | KEEP | - | Table host for ParseFilter. |
| TestFilterMatchEquality | filter_test.go:167 | 0.04 | KEEP | - | Table host for match semantics. |
| TestFilterCasesDoesNotMutateInput | filter_test.go:376 | 0.05 | KEEP | - | Guards the in-place-slice mutation bug class. |
| TestFilterResultLookup | filter_test.go:385 | 0.04 | KEEP | - | Pins that legacy results without metadata match only built-in keys. |
| TestJudgeCheckRequestAndPrompt | judge_check_test.go:70 | 0.05 | KEEP | - | Judge request contract: role, yes/no, pinned model, timeout, untrusted framing and delimiters. |
| TestJudgeCheckMultiTurnRendersTurnsAsInput | judge_check_test.go:109 | 0.05 | KEEP | - | Pins that multi-turn cases render turns into the judge prompt. |
| TestJudgeCheckAccountingSuccess | judge_check_test.go:132 | 0.05 | KEEP | - | Pins one record per judge check and the summed judge cost. |
| TestJudgeCheckParseFailureKeepsRecordZeroCost | judge_check_test.go:160 | 0.05 | KEEP | - | Accounting edge: spent tokens recorded, JudgeCost zero, check failed with a parse error. |
| TestJudgeCheckBackendErrorRecorded | judge_check_test.go:184 | 0.05 | KEEP | - | Pins 'failed, not skipped' for a backend error with the pinned model recorded. |
| TestJudgeCheckMixedCriterionScoresOutOfTwoWithoutLegacyJudge | judge_check_test.go:205 | 0.05 | KEEP | - | Documented rule: checks replace the legacy judge, so no legacy call is made. |
| TestMetadataLoadsScalarTextForms | metadata_test.go:36 | 0.06 | KEEP | - | Pins the documented scalar normalization (3.0 kept as written, booleans lowercased). |
| TestMetadataUnknownSiblingFieldStillRejected | metadata_test.go:166 | 0.04 | KEEP | - | Guards that the custom UnmarshalYAML does not disable strict unknown-field rejection. |
| TestSuiteSHA256HashesRawCaseBytes | metadata_test.go:246 | 0.07 | KEEP | - | Pins raw-byte hashing so a parse-and-reserialize change, which would invalidate every recorded suite, fails. |
| TestMockCLISyntaxAndHeader | mockcli_test.go:137 | 0.06 | KEEP | - | POSIX syntax check (sh -n under sh/dash/busybox) of the shipped mock-cli.sh; the header-phrase half is weaker. |
| TestMockCLITemplateParity | mockcli_test.go:171 | 0.04 | KEEP | - | Template/live sync contract for mock-cli.sh. |
| TestMockCLILogsOneLinePerCall | mockcli_test.go:186 | 0.5 | KEEP | - | Pins the call-log contract that eval checks read. |
| TestMockCLILogQuoting | mockcli_test.go:210 | 0.17 | KEEP | - | Pins shell-safe quoting in the log that checks match against. |
| TestMockCLIReplyLookupOrder | mockcli_test.go:220 | 0.64 | KEEP | - | Pins the documented reply lookup order. |
| TestMockCLISkipsLeadingFlags | mockcli_test.go:245 | 0.46 | KEEP | - | Pins documented flag-skipping when choosing a reply. |
| TestMockCLISanitizesArguments | mockcli_test.go:260 | 0.55 | KEEP | - | Security: argument sanitization blocks path traversal out of the data dir. |
| TestMockCLIExitStatusFromRC | mockcli_test.go:293 | 0.37 | KEEP | - | Pins .rc exit-status semantics including the invalid and orphan cases. |
| TestMockCLIUnsimulatedCall | mockcli_test.go:323 | 0.13 | KEEP | - | Pins that unsimulated calls exit 1, print the documented message and are still logged. |
| TestMockCLIEvalDirErrors | mockcli_test.go:341 | 0.14 | KEEP | - | Pins the KAIRON_EVAL_DIR error paths. |
| TestMockCLIMockDataOverride | mockcli_test.go:364 | 0.19 | KEEP | - | Pins KAIRON_MOCK_DATA override, including that an empty override does not fall back. |
| TestMockCLINeverRunsRealCommand | mockcli_test.go:395 | 0.23 | KEEP | - | Safety: a real aws on PATH is never executed. |
| TestMocksAbsentLeavesOnlyGH | mocks_test.go:158 | 0.15 | KEEP | - | Negative control: no mocks means only the fake gh is on the bin dir. |
| TestMultiTurnClassicCaseHasNoTurnFields | multi_turn_test.go:100 | 0.23 | KEEP | - | Documented rule: single-turn results have no turn_outputs and no calls[].turn. |
| TestAttemptAccountingOnePerCase | attempts_test.go:25 | 0.12 | MERGE | TestAttemptAccountingRepeats | Same attemptAccounting(ar) -> (total, passed) contract with a single-run input; make it a row of a table with the repeats case. |
| TestSelftestFailRepeatAttempts | attempts_test.go:218 | 11.77 | MERGE | TestRepeatExecutionWholeFailAgentSpreadMatchesRecomputedTotals | Both run the whole 22-case selftest-fail suite with --repeat 2; fold the attempts_total/attempts_passed/FAIL-line assertions into the partner's run (saves 11.8s), or run both on a 2-case fixture. |
| TestAuditOrderingIsStable | audit_test.go:634 | 0.07 | MERGE | TestAuditUnmappedFixture | Runs the same unmapped fixture again only to check rule order; add the auditRules order assertion to TestAuditUnmappedFixture. |
| TestAuditEvidenceFixtures | audit_test.go:913 | 0.09 | MERGE | cmd TestAuditFlatFixtureEvidenceAndDuplicates | Same three evidence fixtures are already asserted through the CLI in cmd/kairon/cmd; keep one table (fold the exact violation-count and coverage=100% checks into it). |
| TestAuditDuplicateRuleOrder | audit_test.go:1109 | 0.1 | MERGE | TestAuditDuplicates | First half asserts constant auditRuleOrder map values; the line-6 duplicate-before-unmapped order belongs as one assertion in TestAuditDuplicates' 'repeat of an unmapped line' subtest. |
| TestAuditDuplicateLineFixture | audit_test.go:1148 | 0.08 | MERGE | cmd TestAuditFlatFixtureEvidenceAndDuplicates | duplicate-line fixture is already asserted by the cmd-level fixture table; keep one location with the stricter exact-count check. |
| TestCandidateE2ELiveAgentsUnchanged | candidate_e2e_test.go:321 | 4.23 | MERGE | TestCandidateE2EKeptWorkspacesHoldCandidate | Identical RunOptions{PromptFile, KeepWorkspaces} full-suite run; take the before/after tree and git-status snapshots around the partner's single run. |
| TestCandidateE2ESameContentCandidateHashEqualsBaseline | candidate_e2e_test.go:426 | 8.27 | MERGE | TestCandidateE2EProvenanceRecorded | Two more full runs (8.3s) to compare hashes; add a same-content candidate run to the partner's baseline run (one-case filter) instead. |
| TestUpdateIncrementalSummaryPromptFile | candidate_test.go:356 | 0.05 | MERGE | TestSummaryMultiAgentOmitsTopLevelAgentFields | Same single-vs-multi-agent top-level-field rule; add PromptFile to the partner's mk() result. |
| TestStageCandidatePromptReplacesSymlinkWithoutWritingThrough | candidate_test.go:533 | 0.07 | MERGE | TestStageCandidatePromptReplacesHardlinkWithoutWritingThrough | Same no-write-through behavior with a different link type; one table over symlink/hardlink. |
| TestLoadCasesWithoutChecksOnlyAllowsCheckLessCase | checks_only_test.go:246 | 0.14 | MERGE | TestLoadCasesChecksOnlyRejectsCheckLessCase | Same loadCases call with checks_only=false; make a table over (checks_only, case set) -> error/no error. |
| TestLoadCasesChecksOnlyAllowsFullyCheckedCases | checks_only_test.go:266 | 0.05 | MERGE | TestLoadCasesChecksOnlyRejectsCheckLessCase | Third row of the same checks_only x case-set table. |
| TestChecksValidateNilRubricSkipsCrossCheck | checks_test.go:144 | 0.06 | MERGE | TestChecksValidateRubricCrossCheck | Same ValidateChecks call with a nil rubric; add a rubric field to the partner's table. |
| TestContainerKiroCLIPromptOnStdinNotArgv | container_agent_test.go:192 | 0.06 | MERGE | TestContainerExecsAreWrappedAndPromptStaysOnStdin | Same invokeAgent(builder, nastyPrompt, container) setup and stdin/argv assertion; fold the exact-stdin, ANSI and cost/record checks into the partner. |
| TestContainerHelperPromptOnStdinNotArgv | container_agent_test.go:222 | 0.06 | MERGE | TestContainerExecsAreWrappedAndPromptStaysOnStdin | Partner's helper half uses the same setup; add the exact helper command-line check there. |
| TestContainerTimeoutMapping | container_agent_test.go:423 | 0.09 | MERGE | TestContainerTimeoutNamesEffectiveTimeout | Same timeout mapping via the sandbox limit instead of the case timeout; add as a third subtest. |
| TestBuildContainerMountsHelper | container_runner_test.go:122 | 0.06 | MERGE | TestBuildContainerMountsKiroCLI | Same buildContainerMounts with the stub backend and a custom workspace; one table over backend/workspace. |
| TestBuildContainerMountsKaironBinaryOnPathForEveryBackend | container_runner_test.go:159 | 0.06 | MERGE | TestBuildContainerMountsKiroCLI | Per-backend kairon mount is the same table; keep the containerPath-on-PATH check as a table assertion. |
| TestContainerEnvPathAndEvalDir | container_runner_test.go:547 | 0.05 | MERGE | TestContainerConfigUserEnvAndWorkdir | Same container env contract (PATH prefix, KAIRON_EVAL_DIR, GH_*); add these entries to the partner's env list. |
| TestContainerSessionPerTurnRequestSetup | container_session_test.go:149 | 0.23 | MERGE | TestContainerSessionThreeTurnsOneOpenOneClose | Same three-turn case and fake session; assert per-turn timeout/workdir/trust/prompt in the partner. |
| TestContainmentKaironBinaryJSON | containment_test.go:124 | 0.05 | MERGE | TestContainmentToolTrustSerialisesEmptyArray | Documented key-name check fits one Containment JSON-shape test; the legacy-decodes-false half is Go zero-value behavior. |
| TestUpdateIncrementalSummaryLegacyResultLeavesModeUntouched | containment_test.go:291 | 0.04 | MERGE | TestUpdateIncrementalSummaryRecordsModeAndContainment | Third input (Sandbox nil) to the same writer; add as a case next to container/native. |
| TestCoverageResumeUnchangedSuiteContinues | coverage_run_test.go:229 | 4.74 | MERGE | TestCoverageResumeChangedSuiteRefused | Positive control for the same resume path; in one run, refuse after editing the case, revert the edit, then resume successfully (small fixture suite). |
| TestCoverageStampIncompleteRun | coverage_test.go:92 | 0.04 | MERGE | TestCoverageStampFullRun | Same stampCoverage call with an uncovered case; a table row. |
| TestCoverageStampCaseFilterIsNeverComplete | coverage_test.go:104 | 0.05 | MERGE | TestCoverageStampFullRun | Same stampCoverage call with a caseFilter; a table row. |
| TestCoverageStampNoOpWithoutSuiteEntry | coverage_test.go:121 | 0.05 | MERGE | TestCoverageStampFullRun | Same stampCoverage call with no suite entry; a table row. |
| TestCoverageConfigureResetsRunScopedFields | coverage_test.go:141 | 0.05 | MERGE | TestConfigureKeepWorkspaces | Same 'configure resets run-scoped cfg fields' pattern; one table over fields (suites, caseFilter, keepWorkspaces, debug). |
| TestCoverageUpdateIncrementalSummaryNoEntryWhenUnrecorded | coverage_test.go:258 | 0.05 | MERGE | TestCoverageUpdateIncrementalSummaryCopiesFields | Same writer with an unrecorded result; pair it with the partner as a two-row table. |
| TestAgentVerdictFloorNamesSeveralCriteriaInOrder | criterion_floor_test.go:92 | 0.05 | MERGE | TestAgentVerdictFloorBoundary | Same agentVerdict call returning CriteriaBelowFloor; add a multi-criterion row (wantBelow [a b]). |
| TestDiffFilterQualityPerDollarUsesFilteredCost | diff_filter_test.go:139 | 0.05 | MERGE | TestDiffFilterReportsOnlyMatchingCases | Identical runDiffFiltered(r1, r2, speed==fast) call; add the quality/$ lines to the partner's requireContains. |
| TestDiffFilterNoMatchOnEitherSideOmitsAgent | diff_filter_test.go:163 | 0.05 | MERGE | TestDiffFilterBuiltInAgentAndNameKeys | Partner's agent==zz already checks agent omission on no match; add the Filter/Cost Delta lines there. |
| TestDiffCasesBlockOnlyListsMatchingCasesWithFilter | diff_filter_test.go:254 | 0.05 | MERGE | TestDiffFilterReportsOnlyMatchingCases | Identical diff invocation; add the Cases block lines to the partner. |
| TestDiffLegacyFixtureVsRecordedNative | diff_provenance_test.go:538 | 0.06 | MERGE | TestDiffLegacyFixtureVsContainer | Same legacy fixture and assertions with the other side native; one table over native/container. |
| TestCaseAndFilterRunRecordsCombinedLabel | filter_run_test.go:659 | 0.3 | MERGE | TestRunCaseAndFilterCombineWithAND | Same RunWithOptions(alpha, a1, speed==fast) as the partner's 'both match' subtest; add the case=a1 label assertion there. |
| TestResumeChangedFilterRefusedEvenWhenSavedAgentNowMatchesNothing | filter_run_test.go:929 | 0.41 | MERGE | TestResumeChangedFilterRefusedBeforeEarlierMatchingAgentWrites | Mirror image of the partner (saved agent sorts first vs last); one table over the two agent orderings. |
| TestFilterParseTrimsWhitespace | filter_test.go:47 | 0.05 | MERGE | TestFilterParseOperators | Same ParseFilter expr->Filter contract; add the two whitespace rows. |
| TestFilterParseOperatorChoosesLongest | filter_test.go:60 | 0.05 | MERGE | TestFilterParseOperators | Same contract; add the a<=>b row. |
| TestFilterMatchMissingKeyNeverMatches | filter_test.go:196 | 0.04 | MERGE | TestFilterMatchEquality | Same matches(look) contract with an absent key; add rows. |
| TestFilterEqualityNeverParsesNumbers | filter_test.go:267 | 0.05 | MERGE | TestFilterMatchEquality | Same matches(look) contract on a numeric-looking value; add rows. |
| TestFilterBuiltInKeysAndMetadataKeys | filter_test.go:315 | 0.04 | MERGE | TestFilterResultLookup | Same built-in vs metadata key lookup, for TestCase instead of CaseResult; one table over both lookups. |
| TestJudgeCheckDebugOffWritesNothing | judge_check_test.go:250 | 0.05 | MERGE | TestJudgeCheckDebugPrintsPrompt | Off/on are the same captureDebug setup; two rows of one test. |
| TestJudgeCheckConfigureCarriesDebug | judge_check_test.go:263 | 0.05 | MERGE | TestConfigureKeepWorkspaces | Same configure set-then-reset pattern; a row in the configure table. |
| TestJudgeCheckEmptyOutputMakesNoJudgeCall | judge_check_test.go:279 | 0.05 | MERGE | TestChecksScoringNoOutputOrWorkspaceScoresZero | Same no-output gate; add a judge check plus recording backend to the partner's table. |
| TestJudgeCheckPromptOmitsCaseExpectations | judge_check_test.go:298 | 0.05 | MERGE | TestJudgeCheckRequestAndPrompt | Same scoreCase+recording setup; set ExpectedOutput in the partner and assert it is absent. |
| TestMetadataReservedKeyErrorExplains | metadata_test.go:150 | 0.05 | MERGE | TestMetadataInvalidFailsLoadCases | Partner has the 'reserved agent' row; add an optional explanation-substring column. |
| TestMetadataValueErrorSaysWhatIsAllowed | metadata_test.go:158 | 0.04 | MERGE | TestMetadataInvalidFailsLoadCases | Partner has the 'list value' row; same explanation column. |
| TestMultiTurnClassicCaseSendsTurnZero | multi_turn_test.go:161 | 0.21 | MERGE | TestMultiTurnClassicCaseHasNoTurnFields | Same classic stub case through executeCase; add the recording-backend request check to the partner. |
| TestInvokeAgentDefaultBackendTimeout | backend_routing_test.go:95 | 5.27 | REWRITE-CHEAPER | - | Valuable timeout-message contract, but 200ms timeout takes 5.2s because the fake's child `sleep 5` holds the pipe (kirocli.go sets no WaitDelay/process group); use `exec sleep 5` or set cmd.WaitDelay, and assert elapsed < 1s, which also exposes a real hang. |
| TestCandidateE2EKeptWorkspacesHoldCandidate | candidate_e2e_test.go:269 | 5.23 | REWRITE-CHEAPER | - | Staged-candidate-in-every-workspace assertion is valuable, but it runs all 28 selftest cases; a 2-case fixture suite (or --filter name==a,b) proves the same in ~1s. |
| TestCandidateE2EProvenanceRecorded | candidate_e2e_test.go:359 | 7.82 | REWRITE-CHEAPER | - | Provenance is per-run, not per-case, yet it does two full 28-case runs (7.8s); filter both runs to one case (name==stub-basic). |
| TestCandidateE2EResumeRefusedWithoutCandidate | candidate_e2e_test.go:453 | 3.82 | REWRITE-CHEAPER | - | Refusal message and untouched-file contract is valuable; run and resume a one-case filtered run instead of the full suite. |
| TestChecksScoringCharacterizationLegacySelftestCases | checks_scoring_test.go:15 | 3.54 | REWRITE-CHEAPER | - | Characterizes five named cases but runs all 28; pass Filters: name==stub-basic,stub-usage,stub-quoted-input,<marker>,<seeded> (~0.5-1s instead of 3.5s). |
| TestContainerTimeoutNamesEffectiveTimeout | container_runner_test.go:446 | 1.08 | REWRITE-CHEAPER | - | Unique 'names the case timeout, not the sandbox default' contract, but waits a real 1s; use a 50ms callOpts.Timeout. |
| TestCoverageRunFullRunRecordsSuiteAndComplete | coverage_run_test.go:79 | 3.64 | REWRITE-CHEAPER | - | End-to-end wiring of suite_sha256/complete/case_filter is worth one test, but complete needs a full suite, not 28 cases; use a 2-case fixture suite. |
| TestCoverageRunCaseFilterRecorded | coverage_run_test.go:145 | 4.04 | REWRITE-CHEAPER | - | Keep the one-case run, but replace the full baseline run with a direct suiteSHA256(e2eAgent) call. |
| TestCoverageRunRepeatTwoCompleteWhenBothRunsExist | coverage_run_test.go:169 | 6.88 | REWRITE-CHEAPER | - | --repeat 2 over 28 cases (6.9s); use a 2-case fixture suite, and test the trimmed casesCovered check as a pure unit. |
| TestCoverageResumeChangedSuiteRefused | coverage_run_test.go:249 | 3.99 | REWRITE-CHEAPER | - | Refusal message (names both hashes) and untouched file is a real contract; run it on a 2-case fixture suite instead of all 28. |
| TestExecuteCaseTimeoutRecordedNatively | execute_case_test.go:135 | 1.23 | REWRITE-CHEAPER | - | Native timeout recording is valuable, but uses a 1s timeout plus sleep 3; use a 100ms timeout and sleep 1. |
| TestRunSelftestFailReturnsThresholdErrorAfterWritingResults | exit_status_test.go:103 | 3.77 | REWRITE-CHEAPER | - | Unique 'everything written before the error' contract; run selftest-fail filtered to one failing case instead of all 22. |
| TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent | exit_status_test.go:135 | 7.04 | REWRITE-CHEAPER | - | Cross-agent cost sum and '1 of 2 agents' message are real, but it runs selftest+selftest-fail in full (7s); filter each agent to one case with --filter name==... |
| TestResumeRunPrintsVerdictAndReturnsThresholdError | exit_status_test.go:215 | 3.76 | REWRITE-CHEAPER | - | Resume-path verdict/error contract is unique; run and resume a one-case filtered run instead of two full 22-case runs. |
| TestResultsRecordMetadataForEveryRepeat | filter_run_test.go:721 | 1.13 | REWRITE-CHEAPER | - | Metadata-on-every-repeat is worth pinning, but filter to a1 (tagged) and a3 (untagged) to halve the 1.1s. |
| TestArchitectureCompatibility | architecture_compatibility_test.go:13 | 0.78 | DELETE | sandbox.TestDetectHostArchitecture | Re-tests sandbox.DetectHostArchitecture (a pure GOARCH switch) from the eval package; the sandbox package already has TestDetectHostArchitecture. |
| TestRunnerArchitectureSupport | architecture_compatibility_test.go:29 | 0.07 | DELETE | - | Assigns struct fields and reads them back (Go language behavior); never calls the runner despite the name. |
| TestSandboxArchitectureDetection | architecture_compatibility_test.go:48 | 0.05 | DELETE | sandbox.TestDetectHostArchitecture | Same GOARCH->platform mapping already pinned in the sandbox package's own test. |
| TestEvalFrameworkArchitectureIndependence | architecture_compatibility_test.go:108 | 0.07 | DELETE | - | Asserts CaseResult/CostInfo field assignment round-trips; no eval code runs. |
| TestArchitecturePlatformConsistency | architecture_compatibility_test.go:133 | 0.08 | DELETE | sandbox.TestDetectHostArchitecture | Calls a pure switch five times to prove it is deterministic; no behavior beyond the sandbox test. |
| TestSuccessRateIsPercentNotFraction | attempts_test.go:170 | 0.06 | DELETE | TestAttemptAccountingRepeats | TestAttemptAccountingRepeats already asserts success_rate == 50 (a percent) and TestVerdictPassesWithSuccessRateBelow100 asserts 66.67. |
| TestAuditReportDuplicates | audit_test.go:1131 | 0.1 | DELETE | TestAuditDuplicates | TestAuditDuplicates already asserts duplicates=0 ('distinct lines') and the duplicates=2 report line ('third occurrence'). |
| TestScoreLLMJudgeFailureWording | backend_routing_test.go:150 | 0.23 | DELETE | TestCaseTotalsCountsLLMJudgeErrorAndNoUsableJudge | Partner already asserts skipped/zero with a reason; this only adds the exact wording produced by internal/inference, which nothing parses. |
| TestNoKiroCLIProcessOutsideDockerPath | backend_routing_test.go:353 | 0.1 | DELETE | TestMeasureStartupOverheadUsesBackendProbe | Greps perf.go for a literal string; the behavioral partner test already proves perf goes through the backend probe, and the grep is trivially bypassed. |
| TestScoredUserTurns | checks_judge_test.go:57 | 0.05 | DELETE | TestCaseCheckInputCarriesUserTurns | scoredUserTurns is `return tc.Turns`; the partner asserts the same nil/turns outcomes through caseCheckInput. |
| TestRubricChecksOnlyYAML | checks_only_test.go:33 | 0.07 | DELETE | TestLoadCasesChecksOnlyRejectsCheckLessCase | Asserts a yaml struct tag; the loadCases tests read checks_only from a rubric YAML file, so a tag break already fails them. |
| TestChecksScoringNoCommandRunWithoutOutput | checks_scoring_test.go:215 | 0.04 | DELETE | TestChecksScoringNoOutputOrWorkspaceScoresZero | Partner's 'empty output' subtest already asserts the touch command did not run. |
| TestPrintCaseResultNoChecksUnchanged | checks_scoring_test.go:336 | 0.05 | DELETE | TestPrintCaseResultBreakdownOnlyBelowThreshold | Partner asserts the same exact printCaseResult format (also with no checks) on a richer input. |
| TestLoadCasesWithoutChecksUnchanged | checks_test.go:227 | 0.05 | DELETE | TestLoadCasesClassicTurnOneLoads | Asserts a plain case loads with nil Checks (the zero value); every plain-case load in the package exercises this. |
| TestCostInfoJSONOmitsEmptyNewFields | config_test.go:358 | 0.05 | DELETE | - | Asserts omitempty struct tags on model/usage_source; no documented rule or consumer depends on the keys being absent. |
| TestTestCaseParsesStubFromYAML | config_test.go:373 | 0.05 | DELETE | - | Asserts YAML decoding of the stub-backend script, a test double; every selftest stub run (e.g. stub-usage) already fails if this breaks. |
| TestContainerHelperReportedTimeout | container_agent_test.go:452 | 0.05 | DELETE | TestContainerTimeoutNamesEffectiveTimeout | Identical setup to the partner's 'helper-reported timeout' subtest, which asserts strictly more. |
| TestMapContainerErrorNamesGivenTimeout | container_runner_test.go:493 | 0.06 | DELETE | TestContainerTimeoutNamesEffectiveTimeout | Partner asserts the named timeout through invokeAgent and TestContainerTimeoutMapping asserts the --resource-limit hint. |
| TestContainmentForRecordsKaironBinary | containment_test.go:113 | 0.05 | DELETE | TestContainmentMatchesEnforcement | Partner checks KaironBinary against the real mount list; this compares it to the kaironBinaryMounted constant. |
| TestUpdateIncrementalSummaryTrustNothingEmitsEmptyArray | containment_test.go:248 | 0.07 | DELETE | TestContainmentToolTrustSerialisesEmptyArray | Summary marshals the same Containment type, so this repeats the partner's tool_trust:[] assertion. |
| TestLegacyJSONStillUnmarshals | containment_test.go:302 | 0.05 | DELETE | - | Asserts json.Unmarshal leaves absent fields zero; Go language behavior. |
| TestRunFixturesLoadAndScore | containment_test.go:630 | 0.05 | DELETE | TestRunFixturesShapes | Partner loads the same two fixtures and asserts exact totals (6/12, 7/8), which implies nonzero max. |
| TestCoverageRunCaseEditChangesSuiteNotPrompt | coverage_run_test.go:118 | 7.37 | DELETE | TestSuiteSHA256HashesRawCaseBytes | Two full runs (7.4s) to show a byte edit changes suite_sha256, which the partner proves directly; the prompt hash ignoring case files is trivially true. |
| TestCoverageSummaryJSON | coverage_test.go:197 | 0.05 | DELETE | TestCoverageUpdateIncrementalSummaryCopiesFields | Partner asserts the same explicit false/"" keys through the real summary writer; the legacy-decode subtest is Go zero-value behavior. |
| TestCoverageEvaluateProgressiveCaseFilterIncomplete | coverage_test.go:373 | 0.14 | DELETE | TestCoverageStampCaseFilterIsNeverComplete | Logic is pinned by the stampCoverage unit and the wiring by TestCoverageEvaluateProgressiveStampsEverySave. |
| TestCoverageEvaluateUnpinnedLeavesFieldsUnset | coverage_test.go:412 | 0.23 | DELETE | TestCoverageStampNoOpWithoutSuiteEntry | Passes even if evaluate() never calls stampCoverage; the no-op rule is pinned by the unit. |
| TestContainmentDiffKaironBinary | diff_provenance_test.go:205 | 0.05 | DELETE | TestDiffListsKaironBinaryDifference | Partner asserts the same 'kairon_binary: false → true' line end to end; the identical-containment case is covered by TestDiffSameModeNoWarning. |
| TestDescribeContainmentIncludesKaironBinary | diff_provenance_test.go:216 | 0.04 | DELETE | TestDiffContainmentNoneVsPresent | Partner asserts the full describeContainment string through diff output. |
| TestDiffLegacyVsContainerDoesNotFail | diff_provenance_test.go:291 | 0.05 | DELETE | TestDiffLegacyFixtureVsContainer | Partner makes the same legacy-vs-container assertions using the real legacy fixture. |
| TestRunSelftestPassesAndPrintsOneVerdictLine | exit_status_test.go:91 | 3.56 | DELETE | TestRunBothAgentsSumsCostAndNamesOnlyFailingAgent | Line format is pinned by TestPrintAgentVerdictFormat, one-line-per-agent by the partner, and every runE2E caller already fails if selftest does not pass. |
| TestMetadataAbsentLoadsAsBefore | metadata_test.go:73 | 0.05 | DELETE | TestMetadataDoesNotChangeExecutionOrScoring | Partner asserts plain.Metadata == nil; the TestCase JSON omit check has no consumer. |
| TestMetadataUnmarshalYAMLDirect | metadata_test.go:174 | 0.04 | DELETE | TestMetadataLoadsScalarTextForms | Partner covers the same unmarshal through loadCases with more forms. |
| TestMetadataCaseResultJSON | metadata_test.go:186 | 0.05 | DELETE | TestResultsRecordMetadataForEveryRepeat | Partner asserts the metadata key present/absent in real result JSON; round trip is Go behavior. |
| TestSuiteSHA256ChangesOnMetadataOnlyEdit | metadata_test.go:222 | 0.12 | DELETE | TestSuiteSHA256HashesRawCaseBytes | A metadata edit is a byte edit; the partner proves the hash is over raw bytes. |
| TestMockCLIDefaultDataDirIsWorkspaceMocks | mockcli_test.go:384 | 0.12 | DELETE | TestMockCLIReplyLookupOrder | Every other mock test writes replies via e.data into .mocks/aws and reads them, so the default data dir is already proven. |

### Counts

| verdict | tests | total secs (run alone) |
|---|---|---|
| KEEP | 90 | 11.51 |
| MERGE | 47 | 32.48 |
| REWRITE-CHEAPER | 15 | 62.24 |
| DELETE | 35 | 14.10 |
| **total** | 187 | 120.33 |

Notes on the seconds. Most sub-0.1s figures are measurement floor, not real cost, so DELETE and MERGE save little wall time. They mainly remove code that adds nothing. The time is in the whole-suite end-to-end tests: about 82s of the REWRITE-CHEAPER and MERGE seconds, plus 11s of the DELETE seconds, come from tests that each run the full 22-28-case stub suite one or more times. One more finding: `TestInvokeAgentDefaultBackendTimeout` takes 5.2s with a 200ms timeout. `internal/inference/kirocli.go` uses `exec.CommandContext` without `WaitDelay` or a process group, so a kiro-cli whose child holds stdout blocks the eval past its timeout. The test passes only because it never checks the elapsed time.

### Recurring patterns

1. **Whole-suite runs to check per-run facts.** Provenance, coverage and exit-status assertions run the full 28-case `selftest` (or 22-case `selftest-fail`) suite, often twice, when a one-case `--filter name==...` or a 2-case fixture suite would prove the same thing. Examples: TestCandidateE2EProvenanceRecorded (7.8s), TestCoverageRunCaseEditChangesSuiteNotPrompt (7.4s), TestSelftestFailRepeatAttempts (11.8s).
2. **One test per input instead of a table.** Several functions get a separate top-level test for each input, each with the same setup and the same kind of assertion. Examples: TestCoverageStampIncompleteRun / CaseFilterIsNeverComplete / NoOpWithoutSuiteEntry; TestFilterParseTrimsWhitespace / OperatorChoosesLongest; TestDiffFilterQualityPerDollarUsesFilteredCost / TestDiffCasesBlockOnlyListsMatchingCasesWithFilter, which repeat the exact same `runDiffFiltered` call.
3. **Testing the language or the test's own setup.** Some tests set struct fields and read them back, check that `json.Unmarshal` leaves absent fields zero, or call a pure switch repeatedly. Examples: TestRunnerArchitectureSupport, TestEvalFrameworkArchitectureIndependence, TestLegacyJSONStillUnmarshals, TestArchitecturePlatformConsistency.
4. **The same assertion at two layers.** A unit test of a helper repeats exactly what an end-to-end test of the same feature already asserts (or the reverse), or the eval package repeats a cmd-level fixture test. Examples: TestContainmentDiffKaironBinary vs TestDiffListsKaironBinaryDifference; TestDescribeContainmentIncludesKaironBinary vs TestDiffContainmentNoneVsPresent; TestAuditEvidenceFixtures vs cmd TestAuditFlatFixtureEvidenceAndDuplicates.
5. **'...Unchanged' / '...AsBefore' tests from feature work.** Tests written to show a new field does not affect the old path end up checking only zero values or omitempty tags, which many other tests already exercise. Examples: TestLoadCasesWithoutChecksUnchanged, TestMetadataAbsentLoadsAsBefore, TestCostInfoJSONOmitsEmptyNewFields, TestPrintCaseResultNoChecksUnchanged.

Zero-coverage tests are not filler by default. The mock-cli.sh tests, the fixture-parity tests, TestContainmentFixtures and the documented audit fixtures are contract tests on shell scripts, data or docs examples, and they stay KEEP.

## Group: eval-b

### internal/eval candidate verdicts (eval-b)

Reviewer read every candidate body (and its partner where the verdict depends on it). Partners may live in cmd/kairon/cmd when the coverage tool named a cross-package test.

| test | file:line | secs | verdict | partner | reason |
|---|---|---|---|---|---|
| TestRepeatDiffAgainstRepeatOneRunUsesRunOneOutput | repeat_exec_test.go:489 | 12.61 | REWRITE-CHEAPER | TestRepeatShowOutputChangesKeepsRunOneOutput | Only asserts diffTo returns no error after 12.6s of full runs; write two small hand-built result dirs and call diffTo, asserting its output (run-1 semantics are unit-tested by the partner). |
| TestRepeatExecutionRepeatMultipliesCost | repeat_exec_test.go:258 | 12.34 | REWRITE-CHEAPER |  | Cost x3 at repeat 3 needs a single reported-usage case; filter both runs to stub-usage (12s -> ~1.5s). |
| TestRepeatExecutionResumeRefusesChangedRepeat | repeat_exec_test.go:428 | 12.14 | REWRITE-CHEAPER |  | Refusal happens before any case runs; seed with a one- or two-case filtered run (or a hand-written result file) instead of two full repeat-2 runs. |
| TestRepeatExecutionResumeSkipsCompletedCaseWithoutDuplicating | repeat_exec_test.go:382 | 11.61 | REWRITE-CHEAPER |  | Resume-without-duplication needs only two cases (one kept, one re-run); use Filters for two cases instead of the full suite twice. |
| TestSelfTestWorkspaceCasesNative | sandbox_workspace_test.go:269 | 10.61 | REWRITE-CHEAPER |  | Three full-suite runs (10.6s) for workspace-case assertions; filter to the workspace cases (marker, seeded, timeout) and the no-keep run to marker-case; suite completeness is already TestSelfTestStubRunEndToEnd's job. |
| TestProvenancePromptEditChangesHash | pinning_test.go:445 | 10.11 | REWRITE-CHEAPER | TestProvenanceHashChangesWithPromptResourceAndConfig | Three full runs (10s) for a hash wiring check; filter each run to stub-basic and drop the unedited copy (stability is unit-tested). |
| TestRepeatExecutionSelftestRecordsEveryRunInFreshWorkspace | repeat_exec_test.go:52 | 9.02 | REWRITE-CHEAPER |  | Fresh-workspace-per-run and run numbering are real, but repeat 3 of the whole suite (9s) is unnecessary; filter to marker-case plus one other case. |
| TestRepeatExecutionWholeFailAgentSpreadMatchesRecomputedTotals | repeat_exec_test.go:145 | 6.81 | REWRITE-CHEAPER | TestRepeatExecutionJudgeCaseShowsSpread | Recompute check needs a spread, which comes from check-judge-repeat; filter to that case plus one deterministic case instead of all selftest-fail at repeat 2. |
| TestSelfTestKeptWorkspacesAreClean | pinning_test.go:674 | 6.75 | REWRITE-CHEAPER |  | Workspace isolation from project overrides is valuable; filter both runs to stub-conventions-fixture plus stub-basic. |
| TestRepeatExecutionUnsetAndOneAreUnchanged | repeat_exec_test.go:208 | 6.55 | REWRITE-CHEAPER |  | Repeat 0 == repeat 1 console/verdict equivalence holds for any case; filter both runs to stub-basic. |
| TestWorkflowStubRunSurfacesSubAgentProvenance | provenance_wire_test.go:120 | 3.58 | REWRITE-CHEAPER |  | Workflow run is needed, but the second full `selftest` run only checks plain JSON lacks two keys; filter that run to one case. |
| TestSelfTestSimulatedFailingCase | selftest_simulated_test.go:133 | 3.57 | REWRITE-CHEAPER |  | fact_not_asked failure needs only sim-fact-not-asked; filter to it. |
| TestSelfTestJudgeListIsOneOfTwo | selftest_judge_test.go:127 | 3.56 | REWRITE-CHEAPER |  | Multi-question judge list scoring 1/2 is real; filter to check-judge-list. |
| TestSelfTestChecksFailingCases | selftest_checks_test.go:147 | 3.52 | REWRITE-CHEAPER |  | The per-type failure reasoning is valuable but only needs the 10 check-* cases; read from a shared cached `selftest-fail` run or use Filters on check-*. |
| TestSelfTestEmptyResponseCountsAsZero | selftest_checks_test.go:221 | 3.51 | REWRITE-CHEAPER | TestAgentScoreTotals | Only stub-empty-response matters; filter to it, since the with/without agent-total arithmetic is already unit-tested by TestAgentScoreTotals/TestCaseTotals. |
| TestSelfTestChecksPartialIsOneOfTwo | selftest_checks_test.go:260 | 3.51 | REWRITE-CHEAPER |  | Exact 1/2 partial score and reasoning label are real; filter to check-partial instead of a full selftest-fail run. |
| TestSelfTestChecksWrongTurnFails | selftest_checks_test.go:187 | 3.49 | REWRITE-CHEAPER |  | Proves turn scoping on one case; filter the run to multi-turn-wrong-turn (~0.5s) instead of the whole selftest-fail suite. |
| TestSelfTestJudgeFailingCases | selftest_judge_test.go:84 | 3.48 | REWRITE-CHEAPER |  | Distinct failure details (no/garbage/no-script/missing-file) matter; filter to the four check-judge-* cases rather than the full selftest-fail suite. |
| TestCallRecordsSelfTestProvenance | pinning_test.go:329 | 3.45 | REWRITE-CHEAPER |  | Per-case call-record invariants across every case kind genuinely need the whole suite; read from the shared cached selftest run instead of a private 3.5s run. |
| TestSelfTestStubRunDoesNotConsultPath | selftest_test.go:318 | 3.29 | REWRITE-CHEAPER |  | The LookPath regression is a startup property independent of case count; run a single case (stub-basic) with PATH stripped. |
| TestSelfTestVerdictListsCriteria | selftest_criteria_test.go:40 | 3.28 | REWRITE-CHEAPER |  | Criterion totals equal sums over saved cases for any case set; filter to two legacy cases (e.g. stub-basic, stub-usage) instead of the full suite. |
| TestSelfTestSimulatedPassingCases | selftest_simulated_test.go:103 | 3.25 | REWRITE-CHEAPER |  | Sim-log decisions and no-model-call AC only need sim-routes and sim-stop-when; filter to those cases. |
| TestSelfTestChecksInjectIsHiddenFromAgent | selftest_checks_test.go:279 | 3.18 | REWRITE-CHEAPER |  | End-to-end AC that the injected hidden file is invisible to the agent and removed after; filter to check-command-inject. |
| TestSelfTestChecksPassingCases | selftest_checks_test.go:62 | 3.12 | REWRITE-CHEAPER |  | Asserts every check type passes over 11 check-* plus 2 multi-turn cases; read them from one package-level cached `selftest` run (sync.Once) instead of a private 3s full-suite run. |
| TestSelfTestJudgePassingCases | selftest_judge_test.go:36 | 3.1 | REWRITE-CHEAPER |  | Judge-check pass path is valuable but needs only check-judge-yes/files/mixed; use Filters on those three cases. |
| TestSelfTestChecksRunExitStatus | selftest_checks_test.go:338 | 7.22 | DELETE | TestEvalSelfTestSucceedsWithDefaults | Two full-suite runs only to check exit status, already asserted by cmd TestEvalSelfTestSucceedsWithDefaults/TestEvalSelfTestFailExitsWithThresholdError and by the repeat tests' ErrThresholdFailed checks. |
| TestSelfTestChecksLegacyScoresUnchanged | selftest_checks_test.go:318 | 3.21 | DELETE | TestChecksScoringCharacterizationLegacySelftestCases | Same five cases and same score assertions as the characterization test, which additionally checks Deterministic/Skipped, so this one adds nothing. |
| TestSelfTestJudgeFilesReachTheJudge | selftest_judge_test.go:69 | 3.1 | DELETE | TestSelfTestJudgePassingCases | check-judge-files passing with exactly one judge call is already asserted inside TestSelfTestJudgePassingCases' loop. |
| TestWorkspacesAreIndependent | workspace_test.go:608 | 0.23 | DELETE |  | Asserts two mkdtemp-backed workspaces differ; trivially true and also checked by the repeat fresh-workspace test. |
| TestOneTurnClassicCaseStillUsesOneShotContainerPath | simulated_user_test.go:555 | 0.21 | DELETE | TestContainerSessionOneTurnAndClassicUseOneShot | Partner runs the same classic case through the same fakes and asserts 0 sessions and one-shot calls. |
| TestStartupOverheadMeasurement | perf_test.go:94 | 0.18 | DELETE |  | Asserts only 0 <= overhead <= 30s of whatever backend cfg holds; cannot fail meaningfully. |
| TestRemoveDeletesBinDir | workspace_test.go:762 | 0.13 | DELETE | TestWorkspaceRemove | BinDir lives under ws.root, which TestWorkspaceRemove already asserts is gone. |
| TestPlannerSuiteCaseFilesDeclarePlannerAgent | planner_suite_test.go:278 | 0.05 | DELETE |  | The loader overwrites tc.Agent from the directory (runner.go tc.Agent = agent), so the YAML agent field is inert, and name/count drift is caught by the green run's case count. |
| TestPlannerSuiteLegacyTranscriptsExistWithoutDocuments | planner_suite_test.go:414 | 0.05 | DELETE | TestPlannerSuiteRedLegacyTranscriptsFail | Guards the red test's own fixtures; the red test already asserts output_document_validity falls below the floor for the right reason. |
| TestPlannerSuiteRegexesCompile | planner_suite_test.go:469 | 0.05 | DELETE | TestPlannerSuiteGreenReferenceTranscriptsPass | checks.validateCheck already regexp.Compiles every pattern at load, so a bad pattern fails the green run. |
| TestNilRunPinsPrices | pricing_cost_test.go:440 | 0.05 | DELETE | TestCostFromUsageUnpinnedPricesNothing | Nil-receiver prices() is exercised by the unpinned-cost test, which asserts the observable result. |
| TestProvenanceTypesSerialisation | provenance_test.go:263 | 0.05 | DELETE | TestCallRecordsSelfTestProvenance | Asserts struct json tags on hand-built values; real files' resources_present/agents keys are asserted end to end. |
| TestAgentConfigFileTrustedAgents | provenance_workflow_test.go:390 | 0.05 | DELETE |  | Asserts encoding/json decodes a list and rejects a string into []string; Go library behavior. |
| TestScoreLLMJudge_ANSISequences | runner_test.go:260 | 0.05 | DELETE |  | Reimplements judge-response extraction inline and only exercises stripANSISequences + encoding/json, never scoreLLMJudge; if kept, call the production parser instead. |
| TestWantThresholdFailure | sandbox_workspace_test.go:103 | 0.05 | DELETE |  | Restates the switch of the test helper wantThresholdFailure; asserts test code, not product behavior. |
| TestSuiteSHA256Format | suite_test.go:112 | 0.05 | DELETE |  | Asserts hex.EncodeToString(sha256) is 64 lowercase hex; stdlib behavior. |
| TestTrendFixtureSuccessRate | trend_test.go:94 | 0.05 | DELETE | TestTrendTextOutput | Success rates 75/100/100/75 are already pinned in TestTrendTextOutput's exact lines. |
| TestUserTurns | turns_schema_test.go:159 | 0.05 | DELETE |  | Trivial accessor; every multi-turn and classic execution test depends on it already. |
| TestVerifyLogFixtureValidEvalIteration | verifylog_test.go:571 | 0.05 | DELETE |  | Asserts the shape of a committed fixture; running the valid set in TestVerifyLogFixtureSets and TestVerifyLogCycleFixtureSets already enforces the rules on it. |
| TestVerifyLogFixtureValidPhases | verifylog_test.go:610 | 0.05 | DELETE |  | Asserts phase lines in committed fixture files; TestVerifyLogCycleFixtureSets verifies the valid set with RequireCycle, which enforces the phase cycle. |
| TestVerifyLogErrorRendersOneLinePerViolation | verifylog_test.go:625 | 0.05 | DELETE | TestVerifyLogReportsAllViolations | The file: rule: message one-line-per-violation format is asserted line by line in TestVerifyLogReportsAllViolations. |
| TestVerifyLogConstants | verifylog_test.go:636 | 0.05 | DELETE |  | Asserts two exported constants equal their literals. |
| TestVerifyLogTestHelpersProduceValidJSON | verifylog_test.go:1829 | 0.05 | DELETE |  | Asserts the test file's own JSON builder helpers. |
| TestWorkflowDocsResumeAndPromptHash | workflow_docs_test.go:153 | 0.05 | DELETE |  | Greps docs/evaluation.md for exact prose phrases; no code or agent consumes that wording, so it only blocks doc edits. |
| TestRunOptionsKeepWorkspacesField | workspace_test.go:648 | 0.05 | DELETE |  | Asserts a Go struct field can be set; no behavior. |
| TestRepeatVerdictLegacyCaseAccountingUnchanged | repeat_test.go:60 | 0.04 | DELETE | TestAgentVerdictCasesTotalAndFailed | Stamped-fail and legacy-fail accounting are a subset of TestAgentVerdictCasesTotalAndFailed's cases. |
| TestRepeatVerdictResultFieldsOmitEmpty | repeat_test.go:241 | 0.04 | DELETE |  | Asserts omitempty tags; every reader decodes absent as 0, so no code depends on omission. |
| TestSelftestPriceFixturesDefineStubUsage | selftest_pricing_test.go:210 | 0.04 | DELETE | TestSelftestPriceSonnetIsPricedPerModel | Asserts fixture contents (model, 1000/500 tokens) that the two price run tests already assert from the run output. |
| TestSimulatedUserDocStatesKeyRules | simulated_user_docs_test.go:93 | 0.04 | DELETE | TestMaxTurnsDefaultAndExplicit | Phrase grep over docs plus the default max_turns=10 already asserted by TestMaxTurnsDefaultAndExplicit. |
| TestRubricThreshold | threshold_test.go:9 | 0.04 | DELETE | TestLoadRubricsPassThreshold | Asserts the DefaultPassThreshold constant and a trivial getter, covered via rubric loading. |
| TestTrendFixtureProvenance | trend_test.go:107 | 0.04 | DELETE | TestTrendTextOutput | Prompt prefixes and agent/judge models are already pinned in TestTrendTextOutput's exact lines. |
| TestVerifyLogPhaseRuleIDsAreRegisteredInOrder | verifylog_test.go:1971 | 0.04 | DELETE | TestVerifyLogPhaseViolationDoesNotSuppressOtherChecks | Asserts the internal ruleOrder map ranks; the observable output ordering is checked behaviorally by the partner. |
| TestVerifyLogFixtureEvidenceRuns | verifylog_test.go:2327 | 0.04 | DELETE |  | Pins check states and scores inside committed fixture runs; asserts fixtures, and the evidence-rule tests that use them fail if they drift. |
| TestWorkflowDocsSectionExists | workflow_docs_test.go:50 | 0.04 | DELETE |  | Greps docs/evaluation.md for exact prose phrases; no code or agent consumes that wording, so it only blocks doc edits. |
| TestWorkflowDocsCallRecordTable | workflow_docs_test.go:65 | 0.04 | DELETE |  | Greps docs/evaluation.md for exact prose phrases; no code or agent consumes that wording, so it only blocks doc edits. |
| TestWorkflowDocsStepRecordsFormat | workflow_docs_test.go:77 | 0.04 | DELETE |  | Greps docs/evaluation.md for exact prose phrases; no code or agent consumes that wording, so it only blocks doc edits. |
| TestWorkflowDocsSubAgentProvenance | workflow_docs_test.go:112 | 0.04 | DELETE |  | Greps docs/evaluation.md for exact prose phrases; no code or agent consumes that wording, so it only blocks doc edits. |
| TestWorkflowDocsWorkflowSHA256 | workflow_docs_test.go:133 | 0.04 | DELETE |  | Greps docs/evaluation.md for exact prose phrases; no code or agent consumes that wording, so it only blocks doc edits. |
| TestCaseResultWorkspaceDirJSON | workspace_test.go:617 | 0.04 | DELETE |  | Asserts encoding/json omitempty and that unknown-free old JSON decodes; Go library behavior, not product logic. |
| TestWorkspaceDefaultIsOneEmptyCommit | workspace_test.go:76 | 0.18 | MERGE | TestWorkspaceFixtureIsSingleCommitEqualToFixture | Same single-commit/clean-status contract; make no-fixture and seeded-fixture two rows of one table. |
| TestRepeatExecutionRejectsRepeatWithCleanupAndPerf | repeat_exec_test.go:329 | 0.14 | MERGE | TestRepeatExecutionRunOptionsValidation | Identical harness and assertions to the validation table; add cleanup/perf as two more rows. |
| TestWorkspaceRelativeRootBecomesAbsolute | workspace_test.go:499 | 0.13 | MERGE | TestWorkspaceDirAbsoluteAndSymlinkResolved | Same absolute-path property with a relative root; add as a row of the symlink test. |
| TestVerifyLogScoreTolerance | verifylog_test.go:647 | 0.09 | MERGE | TestVerifyLogBaselineScoreToleranceBoundary | Same 0.1 score-match tolerance on result_score; one table covering baseline and result fields. |
| TestProvenanceHashResourceOrderMatters | provenance_test.go:97 | 0.08 | MERGE | TestProvenanceHashChangesWithPromptResourceAndConfig | Another 'mutation changes hash' input; fold into a mutation table. |
| TestProvenanceHashFramingIsUnambiguous | provenance_test.go:110 | 0.08 | MERGE | TestProvenanceHashChangesWithPromptResourceAndConfig | Byte-shift between parts is another mutation row of the same hash-sensitivity table. |
| TestVerifyLogRejectsRunNamesOutsideResults | verifylog_test.go:1714 | 0.08 | MERGE | TestVerifyLogDoesNotFollowRunNameOutOfResults | Same path-escape rejection for baseline_run vs result_run; one table over field and name. |
| TestVerifyLogNoLogs | verifylog_test.go:1609 | 0.07 | MERGE | TestVerifyLogCycleNoLogsReportsOnlyNoLogs | Identical subtests that differ only in RequireCycle; one table over RequireCycle in {false,true}. |
| TestProvenanceDispatcherEditChangesBothHashes | provenance_workflow_test.go:156 | 0.05 | MERGE | TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash | Same edit -> which-hash-moves behavior with a dispatcher edit; one table of edits. |
| TestWorkflowHashSwappingStepPromptsChangesHash | provenance_workflow_test.go:166 | 0.05 | MERGE | TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash | Swapped step prompts is another edit row of the same table. |
| TestRepeatVerdictMeanEqualsMeanOfRepeatScores | repeat_test.go:128 | 0.05 | MERGE | TestRepeatVerdictMinMaxMean | Same mean-of-repeat-scores property with 3 repeats/2 cases; add as a row. |
| TestCreateContainerConfig_NilSandboxConfig | runner_config_test.go:114 | 0.05 | MERGE | TestCreateContainerConfig_WithSandboxConfig | Nil config defaults is another row of the existing table. |
| TestCreateContainerConfig_EmptyStringValues | runner_config_test.go:229 | 0.05 | MERGE | TestCreateContainerConfig_WithSandboxConfig | Empty WorkspaceDir default is another row of the existing table. |
| TestCreateContainerConfig_ZeroValues | runner_config_test.go:245 | 0.05 | MERGE | TestCreateContainerConfig_WithSandboxConfig | Zero CPU/memory/timeout defaults is another row of the existing table. |
| TestCreateContainerConfig_DebugFlag | runner_config_test.go:267 | 0.05 | MERGE | TestCreateContainerConfig_WithSandboxConfig | Debug propagation can be a field in the same table. |
| TestLoadCasesTurnsAndInputStillExclusiveWithoutSimulatedUser | simulated_user_schema_test.go:407 | 0.05 | MERGE | TestLoadCasesTurnsAndInputMutuallyExclusive | Same mutual-exclusion error with a different YAML; one table. |
| TestTrendEmptyResultsDir | trend_test.go:375 | 0.05 | MERGE | TestTrendMissingResultsDir | Same 'no data -> error naming agent' behavior; one table of missing/empty/unknown-agent inputs. |
| TestTrendUnknownAgent | trend_test.go:385 | 0.05 | MERGE | TestTrendMissingResultsDir | Unknown agent is another row of the no-data error table (error names agent, nothing written). |
| TestLoadCasesClassicMultiEntryStubUnchanged | turns_schema_test.go:142 | 0.05 | MERGE | TestLoadCasesClassicTurnOneLoads | Classic case shape with a multi-entry stub is another loader row. |
| TestVerifyLogDoesNotFollowRunNameOutOfResults | verifylog_test.go:1737 | 0.05 | MERGE | TestVerifyLogRejectsRunNamesOutsideResults | result_run row of the same escape-rejection table. |
| TestVerifyLogPhaseMissing | verifylog_test.go:1851 | 0.05 | MERGE | TestVerifyLogPhaseUnknownValue | Missing phase is the empty-value row of the unknown-phase table (same rule, same message check). |
| TestChmodOpenVanishedFileIsSkipped | workspace_test.go:789 | 0.05 | MERGE | TestChmodOpenVanishedBeforeChmodIsSkipped | Same vanish-race tolerance at two points (Info vs Chmod); one table with two rows. |
| TestChmodOpenVanishedBeforeChmodIsSkipped | workspace_test.go:809 | 0.05 | MERGE | TestChmodOpenVanishedFileIsSkipped | Same vanish-race tolerance at the Chmod step; merge with the Info-step variant. |
| TestPermWalkFuncSkipsNotExist | workspace_test.go:850 | 0.05 | MERGE | TestPermWalkFuncPropagatesOtherErrors | Same callback error classification with different inputs; one table of error -> want nil/propagate. |
| TestPermWalkFuncRootNotExistPropagates | workspace_test.go:884 | 0.05 | MERGE | TestPermWalkFuncPropagatesOtherErrors | Root-not-exist is another row of the error-classification table. |
| TestPrintCaseResultNoBreakdownWhenPassing | scoring_test.go:139 | 0.04 | MERGE | TestPrintCaseResultBreakdownOnlyBelowThreshold | Same printCaseResult output check with a passing input; one table. |
| TestScoreCaseSkipsCostCriterion | scoring_test.go:155 | 0.04 | MERGE | TestScoreCaseNilScoresWhenNoNonCostCriteria | Same cost-criterion exclusion; add 'cost + deterministic' as a row of the nil-scores table. |
| TestScoreCaseDeterministicEmptyOutputSkipped | scoring_test.go:200 | 0.04 | MERGE | TestScoreCaseLLMJudgeEmptyOutputSkipped | Empty output -> skipped 0 for deterministic vs LLM criteria; one table. |
| TestLoadCasesJudgeTurnBeyondCaseTurns | selftest_judge_transcript_test.go:120 | 0.04 | MERGE | TestLoadCasesTurnsSchemaErrors | Another loader schema error with message substrings; add as a row. |
| TestSelfTestRejectsUnknownBackend | selftest_test.go:338 | 0.04 | MERGE | TestRunWithOptionsRejectsUnknownBackend | Same rejection path; fold its stronger assertions (names nope/kiro-cli/stub, no result dir, no kiro-cli call) into the config_test version. |
| TestSelfTestStubRunEndToEnd | selftest_test.go:287 | 3.15 | KEEP |  | The one canonical full-suite stub run: fixture completeness via assertSelfTestResults plus no kiro-cli exec; keep as the suite-wide run (and make it the shared cached run). |
| TestPinRefusesBeforeAnyCase | pinning_test.go:149 | 2.0 | KEEP |  | Eight user-facing refusal messages plus no kiro-cli call and no results dir; refusals are pre-run so runtime is already small. |
| TestPinSingleCaseModelFlags | pinning_test.go:67 | 1.6 | KEEP |  | Single-case runs per config override checking --model on agent/judge argv and provenance in result/summary; real pinning contract at 1.6s for 3 subtests. |
| TestPlannerSuiteGreenReferenceTranscriptsPass | planner_suite_test.go:309 | 1.06 | KEEP |  | Shipped planner eval suite passes on reference transcripts; real product-suite contract at ~1s. |
| TestPlannerSuiteRedLegacyTranscriptsFail | planner_suite_test.go:433 | 1.05 | KEEP |  | Proves the planner rubric discriminates (legacy transcripts fail named criteria); complements the green run. |
| TestSelfTestJudgeRepeatContinuesCursor | selftest_judge_test.go:185 | 0.59 | KEEP |  | Direct executeCase test of the stub.judge cursor semantics that repeat runs depend on; cheap. |
| TestEvaluateStampsThresholdAndPassed | verdict_test.go:158 | 0.57 | KEEP |  | Both evaluate and evaluateProgressive stamp threshold/passed. |
| TestPlannerEvidenceFabricatedReplyFails | planner_evidence_test.go:347 | 0.52 | KEEP |  | Negative evidence samples must be rejected for their fabricated citation; core value of the evidence check. |
| TestRepeatExecutionLoadsCasesOncePerRun | repeat_exec_test.go:283 | 0.42 | KEEP |  | Single-case (0.42s) runner-level proof that cases load once across repeats. |
| TestSelfTestChecksOnlyNoJudgeCallsInResultFile | selftest_checks_test.go:442 | 0.36 | KEEP |  | Cheap (0.36s) checks_only contract that no judge call is recorded, with a non-vacuity guard. |
| TestSelfTestChecksCriterionMaxEqualsCheckCount | selftest_criteria_test.go:102 | 0.35 | KEEP |  | Cheap (0.35s) checks_only criterion max == check count contract on the small selftest-checks suite. |
| TestEvaluateProgressiveResumeStampsThreshold | verdict_test.go:168 | 0.29 | KEEP |  | Resume of a pre-verdict result stamps and saves the threshold. |
| TestEvaluateProgressiveResumeStampsLegacyCaseAgainstOwnThreshold | verdict_test.go:200 | 0.29 | KEEP |  | Regression (F2): resumed legacy case judged against its own min_score. |
| TestSelftestPriceUnknownMarksCostUnknown | selftest_pricing_test.go:159 | 0.26 | KEEP |  | Single-case unknown-price cost_unknown contract (0.26s). |
| TestSelftestPriceSonnetIsPricedPerModel | selftest_pricing_test.go:113 | 0.25 | KEEP |  | Single-case priced-model cost through the whole result/summary pipeline (0.25s). |
| TestEvaluateProgressiveRefusesMidRunEdit | suite_drift_test.go:180 | 0.22 | KEEP |  | Mid-run suite edit aborts without writing results or running later cases. |
| TestStageKiroTrackedFilesOnly | workspace_test.go:1008 | 0.22 | KEEP |  | Only tracked (incl. intent-to-add) .kiro files with working-tree content are staged; untracked local files excluded. |
| TestWorkspacePermissions | workspace_test.go:389 | 0.21 | KEEP |  | Container uid contract: world-writable tree, non-writable .kiro, committable repo. |
| TestWorkspaceExcludesHarnessPathsFromStatus | workspace_test.go:287 | 0.17 | KEEP |  | changed_files checks depend on .eval being hidden from git status while real files show. |
| TestWorkspaceFixtureTrackedKiroStaysTracked | workspace_test.go:302 | 0.17 | KEEP |  | Tracked .kiro fixture files stay tracked and edits are visible; distinct from the staged-untracked case. |
| TestSuiteSHA256ChangesWhenSuiteInputChanges | suite_test.go:146 | 0.15 | KEEP |  | Table over every suite input kind incl. exec bit. |
| TestPinUnpinnedCallsOmitModel | pinning_test.go:281 | 0.14 | KEEP |  | Unpinned calls must not pass --model; cheap, direct invokeAgent test. |
| TestPlannerEvidenceJudgeLoadsSources | planner_evidence_test.go:250 | 0.14 | KEEP |  | Runtime proof that the judge is shown the right files with the setup's content. |
| TestWorkspaceKiroStagingEvalsOverridesProject | workspace_test.go:361 | 0.13 | KEEP |  | Evals overlay precedence over project .kiro is a core staging rule. |
| TestWorkspaceWithoutProjectKiroOrEvalAgents | workspace_test.go:377 | 0.13 | KEEP |  | .kiro must exist even with no sources; cheap edge case. |
| TestWorkspaceDirAbsoluteAndSymlinkResolved | workspace_test.go:467 | 0.13 | KEEP |  | KAIRON_EVAL_WORKSPACE_ROOT honored with absolute, symlink-resolved paths. |
| TestWorkspaceRemoveToleratesUnreadableDirs | workspace_test.go:546 | 0.13 | KEEP |  | Remove must not fail a run on unreadable dirs; real best-effort cleanup contract. |
| TestProvenanceHashChangesWithPromptResourceAndConfig | provenance_test.go:70 | 0.12 | KEEP |  | Prompt/resource/config sensitivity of prompt_sha256, with revert restoring the hash. |
| TestStageKiroIntoAnyDirectory | workspace_test.go:1076 | 0.12 | KEEP |  | Covers ignoreOverlay staging used for baselines, not covered by the newWS-based tests. |
| TestVerifyLogChain | verifylog_test.go:1536 | 0.11 | KEEP |  | Isolated chain rule with message naming both runs; the all-violations test does not check the message. |
| TestProvenanceGlobResourcesExpanded | provenance_test.go:154 | 0.09 | KEEP |  | Glob resources hash every match and list the pattern once. |
| TestCostFromUsageUnpinnedPricesNothing | pricing_cost_test.go:87 | 0.08 | KEEP |  | Unpinned runs must not invent prices. |
| TestProvenanceMissingResourceSkipped | provenance_test.go:123 | 0.08 | KEEP |  | Missing resources are skipped from resources_present and appear when created. |
| TestProvenanceSubAgentsFromStagedOverlay | provenance_workflow_test.go:406 | 0.08 | KEEP |  | Sub-agents come from the evals overlay (staged-workspace rule). |
| TestProvenanceHashShapeAndStability | provenance_test.go:48 | 0.07 | KEEP |  | Hash stability, trimmed model and non-nil empty resources_present serialisation. |
| TestProvenanceMissingPromptFileErrors | provenance_test.go:193 | 0.07 | KEEP |  | Missing prompt file errors and names the file. |
| TestProvenanceFallsBackToKiroAgents | provenance_test.go:231 | 0.07 | KEEP |  | Config resolution falls back to .kiro/agents without an overlay. |
| TestVerifyLogPhaseUnknownValue | verifylog_test.go:1858 | 0.07 | KEEP |  | Rejects unknown/case-variant phases with a message listing allowed values. |
| TestVerifyLogPhaseAllowedValues | verifylog_test.go:1869 | 0.07 | KEEP |  | Positive control that red/green/refactor are accepted. |
| TestShippedCaseWorkspacesAreMirroredInTemplates | planner_evidence_test.go:412 | 0.06 | KEEP |  | Live/template sync contract for shipped workspace fixtures; task sync:check is not part of task test. |
| TestPlannerSuiteDocumentCasesUseParseBackedCheck | planner_suite_test.go:541 | 0.06 | KEEP |  | Shipped-suite lint: document-returning cases must use the parse-backed check, derived from stub replies. |
| TestCostFromUsagePricedUsesModelPrice | pricing_cost_test.go:37 | 0.06 | KEEP |  | Price x tokens for priced models. |
| TestCalculateAgentJudgeCostsPropagatesIncomplete | pricing_cost_test.go:343 | 0.06 | KEEP |  | Run-level cost aggregation propagates incomplete for agent but not judge. |
| TestProvenanceInlinePromptNeedsNoFile | provenance_test.go:203 | 0.06 | KEEP |  | Inline prompts hash without a file; distinct config shape. |
| TestResolveProvenanceInRootHashesStagedConventions | provenance_test.go:380 | 0.06 | KEEP |  | Provenance resolves against the staged root, ignoring cwd decoys; exact hash pinned. |
| TestProvenanceDispatcherHashIsItsOwnFilesOnly | provenance_workflow_test.go:216 | 0.06 | KEEP |  | Pins dispatcher hash to its own config+prompt exactly. |
| TestImportStepRecordsMalformedImportsNothing | steprecords_test.go:205 | 0.06 | KEEP |  | All-or-nothing import with 1-based line in the error. |
| TestVerifyLogRunExists | verifylog_test.go:1573 | 0.06 | KEEP |  | run-exists for missing runs and for a file posing as a run. |
| TestPerformanceProfiling | perf_test.go:8 | 0.05 | KEEP |  | TrackTestCase -> GenerateProfile timing/invocation counts; perf.go is live via RunOptions.Perf. |
| TestBottleneckDetection | perf_test.go:43 | 0.05 | KEEP |  | Bottleneck identification and severity for a slow case. |
| TestParallelAnalysis | perf_test.go:68 | 0.05 | KEEP |  | analyzeParallelPotential output for independent cases (assertions are loose but on real code). |
| TestPlannerEvidenceChecksHaveSources | planner_evidence_test.go:118 | 0.05 | KEEP |  | Cheap shipped-suite lint that both evidence judge checks list source files. |
| TestPlannerEvidenceSourcesAreTheSetupBytes | planner_evidence_test.go:141 | 0.05 | KEEP |  | Static proof, for every evidence check, that judge files and agent prompt share the exact fixture bytes. |
| TestPlannerSuiteStubDocumentsAreValid | planner_suite_test.go:371 | 0.05 | KEEP |  | Validates shipped stub replies against the stageoutput schema, including punch_out invariants. |
| TestCostFromUsageUnpricedIsIncomplete | pricing_cost_test.go:57 | 0.05 | KEEP |  | Unpriced -> incomplete with named models; empty and zero-token edge cases. |
| TestServedModel | pricing_cost_test.go:98 | 0.05 | KEEP |  | Response model wins over request model; drives pricing. |
| TestCostInfoAddPropagatesIncomplete | pricing_cost_test.go:118 | 0.05 | KEEP |  | Incomplete propagation and sorted/deduped unpriced models. |
| TestNewCallRecordCostUnknownAndModelAgreement | pricing_cost_test.go:162 | 0.05 | KEEP |  | Call record model equals priced model; cost_unknown semantics. |
| TestPrintAgentVerdictCostIncompleteMarker | pricing_cost_test.go:269 | 0.05 | KEEP |  | User-facing cost-incomplete marker on the verdict line. |
| TestResolveResourceFilesInBackslashRootFindsPlainFile | provenance_test.go:176 | 0.05 | KEEP |  | Regression for literal-backslash roots hiding plain resources. |
| TestProvenanceSubAgentsResolved | provenance_workflow_test.go:68 | 0.05 | KEEP |  | Sub-agent resolution, models, resources and exact workflow hash. |
| TestProvenanceSubAgentEditChangesWorkflowButNotDispatcherHash | provenance_workflow_test.go:129 | 0.05 | KEEP |  | Sub-agent edits move workflow_sha256 but not the dispatcher hash; the key separation rule. |
| TestWorkflowHashIndependentOfTrustedAgentsOrder | provenance_workflow_test.go:178 | 0.05 | KEEP |  | Sub-agents sorted by name regardless of trustedAgents order; exact hash pinned. |
| TestProvenanceWithoutTrustedAgentsHasNoSubAgents | provenance_workflow_test.go:193 | 0.05 | KEEP |  | No trustedAgents keeps legacy framing and no workflow hash. |
| TestRepeatVerdictRunOrderIndependentOfStorageOrder | repeat_test.go:92 | 0.05 | KEEP |  | RepeatScores in run order despite case-major storage. |
| TestRepeatVerdictMeanDecidesPass | repeat_test.go:107 | 0.05 | KEEP |  | Boundary: mean, not best repeat, decides the pass. |
| TestRepeatVerdictCasesTotalCountsDistinctNames | repeat_test.go:161 | 0.05 | KEEP |  | cases_total counts distinct names across repeats. |
| TestRepeatVerdictJSONFieldsAlwaysWritten | repeat_test.go:211 | 0.05 | KEEP |  | score_min 0 / repeat fields must be present in summary.json for trend/diff consumers. |
| TestPrintAgentVerdictRepeatSuffix | repeat_test.go:267 | 0.05 | KEEP |  | Exact console verdict line with and without repeat suffix. |
| TestCreateContainerConfig_WithSandboxConfig | runner_config_test.go:11 | 0.05 | KEEP |  | Table of sandbox config -> container limits. |
| TestScoreCaseLLMJudgeEmptyOutputSkipped | scoring_test.go:215 | 0.05 | KEEP |  | Empty output skips the judge entirely (no call, no cost) with the exact reasoning. |
| TestScoreCaseLLMJudgeAccumulatesJudgeCost | scoring_test.go:244 | 0.05 | KEEP |  | Judge cost accumulates across criteria with stub estimate, no kiro-cli. |
| TestScoreCaseNilScoresWhenNoNonCostCriteria | scoring_test.go:272 | 0.05 | KEEP |  | Nil scores when nothing scorable. |
| TestAgentScoreTotals | scoring_test.go:431 | 0.05 | KEEP |  | Table over agent-level denominators; core scoring arithmetic. |
| TestSimulatedUserDocExampleLoads | simulated_user_docs_test.go:29 | 0.05 | KEEP |  | Executes the documented YAML example through the strict loader; docs example cannot drift from schema. |
| TestReadStepRecordsAbsentIsNormal | steprecords_test.go:32 | 0.05 | KEEP |  | Documented contract: absent calls.jsonl/.eval is normal. |
| TestReadStepRecordsValidLines | steprecords_test.go:47 | 0.05 | KEEP |  | Source forced to workspace, blank lines skipped, unknown fields tolerated. |
| TestImportStepRecordsAppendsAndCounts | steprecords_test.go:179 | 0.05 | KEEP |  | Imported steps append and add to agent cost, flipping usage source. |
| TestSuiteSHA256DiffersPerAgent | suite_test.go:137 | 0.05 | KEEP |  | Suite identity is per agent. |
| TestSuiteSHA256SharedFixtureCountedOnce | suite_test.go:237 | 0.05 | KEEP |  | Shared fixtures hashed once. |
| TestTrendFilterRecomputesScoreAndSuccessFromMatchingCases | trend_filter_test.go:122 | 0.05 | KEEP |  | Filtered trend recomputes score/success/metadata from matching cases only. |
| TestTrendRecordedZeroSuccessRate | trend_test.go:265 | 0.05 | KEEP |  | Distinguishes recorded 0 from absent (nil) success rate. |
| TestTrendSameTimestampSortsByName | trend_test.go:316 | 0.05 | KEEP |  | Deterministic tiebreak on equal timestamps. |
| TestTrendDoesNotUsePackageConfig | trend_test.go:416 | 0.05 | KEEP |  | Trend must ignore and not mutate package cfg; real isolation property. |
| TestResolveTrustSetMissingConfigListsPaths | trust_test.go:69 | 0.05 | KEEP |  | Error names both searched config paths. |
| TestContainerInvokeAgentTrustsAllowedToolsOnly | trust_test.go:105 | 0.05 | KEEP |  | Security: container argv uses --trust-tools allowlist, never --trust-all-tools. |
| TestAgentVerdictThresholdBoundary | verdict_test.go:20 | 0.05 | KEEP |  | Exact-threshold pass/fail boundary. |
| TestAgentVerdictDoesNotFailAgentOnCaseFailuresAlone | verdict_test.go:94 | 0.05 | KEEP |  | cases_failed is informational and does not fail the agent. |
| TestAgentCostTotalsIncludesAgentAndJudge | verdict_test.go:104 | 0.05 | KEEP |  | Agent totals include judge cost without aggregating model/source. |
| TestSummaryTotalCostAccumulatesAcrossAgents | verdict_test.go:299 | 0.05 | KEEP |  | Total cost is order-independent and idempotent across repeated progressive saves. |
| TestVerifyLogFixtureRunsRecordCoverage | verifylog_test.go:521 | 0.05 | KEEP |  | Pins the docs/evaluation.md claim that exactly two committed fixture runs are legacy-shaped; docs reference these fixtures as runnable examples. |
| TestVerifyLogBaselineStdRunsPass | verifylog_test.go:925 | 0.05 | KEEP |  | Positive control that the shared std run pair verifies clean; guards false positives cheaply. |
| TestVerifyLogFrontMatterBoundaryScoresAccepted | verifylog_test.go:1364 | 0.05 | KEEP |  | 0.0/100.0 and leap-day boundaries accepted. |
| TestVerifyLogExtraFrontMatterKeysIgnored | verifylog_test.go:1376 | 0.05 | KEEP |  | Forward-compat: unknown front-matter keys tolerated. |
| TestVerifyLogOutputCountsIterations | verifylog_test.go:1814 | 0.05 | KEEP |  | Pins the OK summary line users see from verify-log. |
| TestVerifyLogBaselinePhaseHasNoEvidenceRule | verifylog_test.go:2226 | 0.05 | KEEP |  | Baseline logs are exempt from evidence rules for both change types. |
| TestWorkflowDocsAlgorithmMatchesCode | workflow_docs_test.go:172 | 0.05 | KEEP |  | Independent reimplementation of the documented workflow_sha256 framing compared to workflowSHA256; a real algorithm-pinning unit test. |
| TestWorkspaceCreationFailureLeavesNothingBehind | workspace_test.go:591 | 0.05 | KEEP |  | Failed creation must not leak a workspace directory. |
| TestLoadCasesGHIssueDefaultsNumber | workspace_test.go:696 | 0.05 | KEEP |  | gh_issue.number defaults to 1 and explicit numbers survive; the validation table only checks error/no-error. |
| TestRunPinsApplyToCopiesWorkflowProvenance | provenance_wire_test.go:73 | 0.04 | KEEP |  | Direct unit test that only workflow agents get workflow provenance. |
| TestRepeatVerdictMinMaxMean | repeat_test.go:72 | 0.04 | KEEP |  | Repeat min/max/mean/RepeatScores and pass on mean. |
| TestPrintCaseResultBreakdownOnlyBelowThreshold | scoring_test.go:113 | 0.04 | KEEP |  | Exact case output and breakdown threshold rule (3/4 of max, skipped as 0). |
| TestScoreCaseDeterministicPath | scoring_test.go:174 | 0.04 | KEEP |  | Deterministic scoring result and zero judge cost. |
| TestCaseTotals | scoring_test.go:362 | 0.04 | KEEP |  | Table over skipped/zero-max denominators; core scoring arithmetic. |
| TestMaxTurnsDefaultAndExplicit | simulated_user_schema_test.go:118 | 0.04 | KEEP |  | Default max_turns 10 and explicit override; documented default. |
| TestTrendTextOutput | trend_test.go:140 | 0.04 | KEEP |  | Exact user-facing trend text lines including INCOMPLETE marker. |
| TestTrendJSONOutput | trend_test.go:175 | 0.04 | KEEP |  | JSON key set and shape for trend --json consumers. |
| TestUpdateIncrementalSummaryWritesAgentVerdicts | verdict_test.go:250 | 0.04 | KEEP |  | Incremental summary keeps prior agents' verdicts and the required keys. |

### Counts

| verdict | tests | seconds (run alone) |
|---|---|---|
| KEEP | 108 | 19.18 |
| MERGE | 30 | 1.90 |
| REWRITE-CHEAPER | 25 | 149.44 |
| DELETE | 39 | 15.74 |
| total | 202 | 186.26 |

REWRITE-CHEAPER is 149s of the 186s total. Applying the specific rewrites (filter to one or two cases, or a shared cached run) should bring that to about 15-25s with no assertion lost.

### Recurring patterns

1. **Each test runs its own full selftest suite and then reads one case.** `runSelfTestAgent`/`runWithSummary`/`runStubRepeat(..., "", ...)` run all of `selftest` or `selftest-fail` (~3.3s), and each test does this separately. Many of these tests then look at a single case. Examples: TestSelfTestChecksWrongTurnFails, TestSelfTestChecksPartialIsOneOfTwo, TestSelfTestJudgeListIsOneOfTwo (also TestSelfTestSimulatedFailingCase, TestSelfTestChecksInjectIsHiddenFromAgent). Fix: pass the case name or `Filters`. Tests that do need every case (TestSelfTestChecksPassingCases, TestCallRecordsSelfTestProvenance) should read one package-level `sync.Once` result per agent. Keep a single canonical full run (TestSelfTestStubRunEndToEnd).
2. **Repeat, resume and diff tests multiply the full suite.** Several tests run the whole suite two or three times at repeat 2-3, even though the property needs one case. Examples: TestRepeatExecutionRepeatMultipliesCost (repeat 1 + repeat 3 of everything, 12.3s), TestRepeatExecutionResumeRefusesChangedRepeat (two seed runs, and the refusal happens before any case runs, 12.1s), TestRepeatDiffAgainstRepeatOneRunUsesRunOneOutput (12.6s that only asserts `diffTo` returned nil). TestProvenancePromptEditChangesHash (three full runs to compare a hash) follows the same pattern.
3. **Docs tests that grep for phrases.** Some tests check that docs/evaluation.md contains exact prose substrings. They protect wording that nothing consumes, and they break when someone edits the docs. Examples: TestWorkflowDocsStepRecordsFormat, TestWorkflowDocsWorkflowSHA256, TestSimulatedUserDocStatesKeyRules (and the other four TestWorkflowDocs* phrase tests). Contrast the kept doc tests, which execute the documented artifact: TestSimulatedUserDocExampleLoads and TestWorkflowDocsAlgorithmMatchesCode.
4. **Tests of the language, the library, constants, test helpers or fixtures, not product behavior.** Examples: TestRunOptionsKeepWorkspacesField (a struct field can be set), TestAgentConfigFileTrustedAgents and TestCaseResultWorkspaceDirJSON (encoding/json behavior), TestWantThresholdFailure and TestVerifyLogTestHelpersProduceValidJSON (test helpers), TestVerifyLogFixtureEvidenceRuns and TestSelftestPriceFixturesDefineStubUsage (fixture contents that the tests running them already assert), TestPlannerSuiteRegexesCompile (the loader already compiles every pattern).
5. **One property split into several near-identical functions instead of a table.** Examples: TestCreateContainerConfig_NilSandboxConfig, _ZeroValues, _EmptyStringValues and _DebugFlag next to the existing WithSandboxConfig table; TestPermWalkFuncSkipsNotExist and TestPermWalkFuncRootNotExistPropagates next to TestPermWalkFuncPropagatesOtherErrors; TestVerifyLogNoLogs, which duplicates TestVerifyLogCycleNoLogsReportsOnlyNoLogs with only RequireCycle flipped. These are cheap, so the cost is noise and maintenance, not runtime.

## Group: sandbox-agent-cmd

### Test audit verdicts: internal/eval/sandbox, internal/agent, cmd/kairon/cmd

| test | pkg | file:line | secs | verdict | partner | reason |
|---|---|---|---|---|---|---|
| TestEvalUnknownBackendRejected | cmd/kairon/cmd | eval_test.go:44 | 0.1 | KEEP |  | Only test that the unknown-backend error names the bad value and every valid backend (kiro-cli, stub). |
| TestEvalSelfTestSucceedsWithDefaults | cmd/kairon/cmd | eval_test.go:149 | 7.66 | DELETE | TestEvalPromptFileRecorded | Duplicates the baseline run in TestEvalPromptFileRecorded (same no-config selftest, same exactly-one-summary.json check); suite behaviour belongs to internal/eval. |
| TestInferenceExecHidden | cmd/kairon/cmd | inference_exec_test.go:34 | 0.06 | MERGE | TestEvalFlagsRegistered | Asserts the Hidden constant and that --backend exists; fold into one table-driven CLI-surface test. |
| TestVerifyLogMissingAgentIsUsageError | cmd/kairon/cmd | eval_verify_log_test.go:212 | 0.06 | MERGE | TestAuditArgs | Same cobra.ExactArgs(1) arity check as TestAuditArgs/TestTrendArgs; one table over the three subcommands. |
| TestAuditArgs | cmd/kairon/cmd | eval_audit_test.go:249 | 0.1 | MERGE | TestVerifyLogMissingAgentIsUsageError | Arity check of a declared cobra.ExactArgs(1); identical shape to the verify-log and trend copies. |
| TestTrendRestoresFlagVars | cmd/kairon/cmd | eval_trend_test.go:122 | 0.07 | DELETE |  | Asserts that the test helper runTrend restores package vars, i.e. the test's own setup, not trend behaviour. |
| TestAuditHelpDescribesEvidenceAndDuplicates | cmd/kairon/cmd | eval_audit_test.go:170 | 0.08 | DELETE |  | Asserts phrases in auditCmd.Long help text (incidental wording). |
| TestAuditHelpDescribesRubricRules | cmd/kairon/cmd | eval_audit_test.go:201 | 0.09 | DELETE |  | Asserts phrases in auditCmd.Long help text (incidental wording); the rules themselves are tested by the fixture tests. |
| TestEvalPromptFileFlagRegistered | cmd/kairon/cmd | eval_test.go:195 | 0.1 | MERGE | TestEvalFlagsRegistered | Default/usage/local-vs-persistent of --prompt-file; only the not-inherited-by-diff row is contract, fold into the flag table. |
| TestEvalFlagsRegistered | cmd/kairon/cmd | eval_test.go:17 | 0.09 | KEEP |  | Host for the merged flag table: --backend default kiro-cli, usage listing every inference.Names() entry, --evals-dir inherited by diff are real CLI contracts. |
| TestTrendUnknownAgentErrorNamesAgent | cmd/kairon/cmd | eval_trend_test.go:109 | 0.08 | KEEP |  | Checks the error names the agent and nothing is written to stdout; TestTrendSilencesUsageOnError only checks SilenceUsage. |
| TestEvalPromptFileRecorded | cmd/kairon/cmd | eval_test.go:244 | 11.41 | REWRITE-CHEAPER |  | Runs the 28-case selftest suite twice (11.4s) to prove --prompt-file wiring; do one candidate run with evalFilter name==stub-basic and assert prompt_file only (sha256 difference is covered in internal/eval candidate tests). |
| TestVerifyLogFlagsRegistered | cmd/kairon/cmd | eval_verify_log_test.go:35 | 0.07 | MERGE | TestEvalFlagsRegistered | Flag defaults/inheritance mirror the declarations; fold into the shared flag table. |
| TestAuditSkillsFSWiredFromTemplates | cmd/kairon/cmd | eval_audit_test.go:305 | 0.09 | DELETE |  | In package tests Templates is the zero embed.FS, so it only asserts fs.Sub on an empty FS does not error (library behaviour), not real wiring. |
| TestTrendFlagsRegistered | cmd/kairon/cmd | eval_trend_test.go:30 | 0.07 | MERGE | TestEvalFlagsRegistered | Flag default/local/inherited checks mirror declarations; fold into the shared flag table. |
| TestVerifyLogValidSetWithoutRequireCycle | cmd/kairon/cmd | eval_verify_log_test.go:172 | 0.07 | MERGE | TestVerifyLogValidSet | Same valid fixture with require-cycle=false; loop TestVerifyLogValidSet over both values (FailingSets already does this). |
| TestEvalFilterAndStrictFlagsRegistered | cmd/kairon/cmd | eval_test.go:438 | 0.07 | MERGE | TestEvalFlagsRegistered | Inheritance by diff/trend is contract, but the stringArray type is already proven behaviourally by TestEvalFilterFlagRepeatedAndCommas; fold the rest into the flag table. |
| TestCheckCommandRegistered | cmd/kairon/cmd | check_test.go:87 | 0.84 | DELETE | TestCheckFlagsRegistered | rootCmd.Find("check") is repeated by TestCheckFlagsRegistered and implied by every runCheck test that executes through rootCmd. |
| TestVerifyLogNoRefactorFailsByDefault | cmd/kairon/cmd | eval_verify_log_test.go:152 | 0.07 | KEEP |  | The only fixture where --require-cycle changes the outcome (fails with cycle rule). |
| TestAuditFlagsRegistered | cmd/kairon/cmd | eval_audit_test.go:214 | 0.11 | MERGE | TestEvalFlagsRegistered | Registration/default/inheritance checks mirror declarations; fold into the shared flag table. |
| TestCheckWithoutJudgeNeverReadsEvalsConfig | cmd/kairon/cmd | check_test.go:304 | 0.2 | KEEP |  | Proves a broken evals config does not break judge-free checks (lazy load), a real behaviour no other test asserts. |
| TestEvalRepeatFlagRegistered | cmd/kairon/cmd | eval_test.go:318 | 0.08 | MERGE | TestEvalFlagsRegistered | Default 1/int type/not inherited mirror the declaration; wiring is proven by TestEvalRepeatPassedToRun. |
| TestEvalSelfTestFailExitsWithThresholdError | cmd/kairon/cmd | eval_test.go:180 | 6.3 | REWRITE-CHEAPER |  | Runs all 22 selftest-fail cases incl. stub-timeout (6.3s) to prove ErrThresholdFailed + SilenceUsage propagate; set evalFilter to one fast failing case (e.g. name==check-file-exists). |
| TestEvalFilterFlagRepeatedAndCommas | cmd/kairon/cmd | eval_test.go:481 | 0.14 | KEEP |  | Behavioural proof that repeated --filter accumulates and commas are not split (user-facing contract). |
| TestCheckFlagsRegistered | cmd/kairon/cmd | check_test.go:94 | 0.1 | MERGE | TestEvalFlagsRegistered | Required annotations on --checks/--dir are the real contract; fold into the shared flag table. |
| TestEvalRepeatPassedToRun | cmd/kairon/cmd | eval_test.go:375 | 10.41 | REWRITE-CHEAPER |  | Runs the full 28-case suite 3x (10.4s) to prove --repeat reaches RunOptions; filter to name==stub-basic with repeat 2 and assert summary repeat (per-case counting is covered by internal/eval attempts_test). |
| TestTrendArgs | cmd/kairon/cmd | eval_trend_test.go:136 | 0.07 | MERGE | TestAuditArgs | Same cobra.ExactArgs(1) arity check; one table over audit/verify-log/trend. |
| TestEvalUnknownBackendRejectedForPerfAndCleanup | cmd/kairon/cmd | eval_test.go:60 | 0.13 | MERGE | TestEvalUnknownBackendRejected | Same backend validation with different mode inputs (perf/cleanup vs selftest); make it one table over modes. |
| TestAuditSilencesUsageOnAuditFailure | cmd/kairon/cmd | eval_audit_test.go:261 | 0.16 | KEEP |  | Each command sets SilenceUsage itself; this is the only check for audit. |
| TestCheckJudgeRequestShape | cmd/kairon/cmd | check_test.go:235 | 0.1 | KEEP |  | Asserts the judge request sent to the backend (RoleJudge, YesNo, configured model, nil Stub); the stub test asserts different things. |
| TestVerifyLogRequireCycleFlagParsed | cmd/kairon/cmd | eval_verify_log_test.go:178 | 0.07 | MERGE | TestVerifyLogFlagsRegistered | Checks the flag is bound to verifyLogRequireCycle (other tests set the var directly); fold into the flag table as a bind row. |
| TestVerifyLogNoRefactorPassesWithoutRequireCycle | cmd/kairon/cmd | eval_verify_log_test.go:162 | 0.07 | MERGE | TestVerifyLogNoRefactorFailsByDefault | Same no-refactor fixture, flag flipped; one two-row table. |
| TestVerifyLogSilencesUsageOnVerificationFailure | cmd/kairon/cmd | eval_verify_log_test.go:224 | 0.07 | KEEP |  | Only check that verify-log silences usage on a verification failure. |
| TestPunchOut_KeepsLogRemedyForExistingCallers | internal/agent | manager_boundary_test.go:344 | 0.06 | KEEP |  | Asserts the non-boundary punch-out keeps the log remedy, the inverse of the boundary punch-out assertion. |
| TestPunchOut_RetriesRemainDoesNotRemoveWorktree | internal/agent | manager_failure_test.go:377 | 0.11 | MERGE | TestHandleExit_RetriesRemainNoWrites | Same retries-remain setup; add removedCount()==0 to that table (it checks cleanup but not worktree removal). |
| TestOutputCaptureRingBuffer | internal/agent | output_capture_test.go:47 | 0.06 | KEEP |  | Only exact eviction-order check; tui TestHighVolumeOutput only checks size and last line conditionally. |
| TestSpawn_ValidPassesDocumentToKrewLead | internal/agent | manager_boundary_test.go:165 | 0.06 | KEEP |  | Security contract: input doc path absolute, outside worktree, 0600, called untrusted, body never in argv/env. |
| TestIntakeActive_PIDMarker | internal/agent | intake_test.go:516 | 0.06 | KEEP |  | Only test that a live marker is active-but-not-owned and a dead marker is removed; the watcher test checks start counts only. |
| TestTerminalOutputEnabledFunction | internal/agent | manager_suspend_test.go:161 | 0.04 | DELETE | TestConditionalWriterRespectsSuspension | Asserts the private accessor toggles; the conditional-writer test exercises the same function through real writes. |
| TestOutputCaptureGeneration | internal/agent | output_capture_test.go:81 | 0.05 | DELETE | TestGetOutputLinesReturnsCompleteDataAfterResume | Monotonic generation is already asserted there; comments refer to a removed display-state mode. |
| TestSpawn_ValidKeepsOperationalParameters | internal/agent | manager_boundary_test.go:221 | 0.06 | KEEP |  | Pins the krew-lead env/args/Dir contract (ISSUE_NUMBER, REPO, WORKTREE_PATH, --agent krew-lead). |
| TestWorktreeCreate | internal/agent | worktree_create_test.go:243 | 24.34 | REWRITE-CHEAPER |  | Legit contract test of worktree-create.sh (zero Go coverage), but ~16 subtests serially build fresh bare+2 clones (24s); add t.Parallel() to subtests, build one fixture template and cp -r it, and run the script once for the four assertions that share the plain behind-origin setup. |
| TestManagerSuspendOnlyAffectsTerminalOutput | internal/agent | manager_suspend_test.go:11 | 0.05 | DELETE | TestConditionalWriterRespectsSuspension | Asserts a private bool flips; suspension behaviour is asserted through writers elsewhere. |
| TestHandleExit_ZeroExitWithoutPRDoesNotRunGenericCleanup | internal/agent | manager_failure_test.go:392 | 0.14 | MERGE | TestPunchOut_RemovesWorktreeBeforeApplyingFailLabel | Same exit-0/no-PR punch-out case; add len(h.cleaned)==0 to that subtest. |
| TestConditionalWriterRespectsSuspension | internal/agent | manager_suspend_test.go:183 | 0.05 | KEEP |  | Behavioural: terminal writes suppressed while suspended but byte count still reported. |
| TestSpawn_WorktreeFailureRemovesInputDocument | internal/agent | manager_boundary_test.go:391 | 0.06 | KEEP |  | Only check that the input document is cleaned up after a worktree failure. |
| TestCaptureWriterConcurrentWrites | internal/agent | output_capture_test.go:109 | 0.05 | KEEP |  | Concurrent Write under -race with prefix preservation; nothing else exercises concurrency. |
| TestSpawn_WriteInputFailureCreatesNothing | internal/agent | manager_boundary_test.go:378 | 0.04 | MERGE | TestSpawn_PreStartFailureIsUnavailable | Partner's writeInput-fails row is the same input; add the createWorktree/start-not-called assertion there. |
| TestOutputCaptureAlwaysAccumulates | internal/agent | output_capture_test.go:9 | 0.05 | DELETE | TestOutputCaptureRingBuffer | Below-capacity accumulation is covered by RingBuffer's first block (and tui TestOutputAlwaysAccumulates); 'regardless of display state' refers to removed behaviour. |
| TestFakeGH_RequiresEvalDir | internal/eval/sandbox | mock_github_test.go:382 | 0.05 | KEEP |  | Contract of the fake gh shell script (invisible to Go coverage): missing KAIRON_EVAL_DIR fails loudly. |
| TestFakeGH_BodyFileMissingValue | internal/eval/sandbox | mock_github_test.go:189 | 0.05 | KEEP |  | Script contract: --body-file with no value errors and is still logged. |
| TestDetectHostArchitecture | internal/eval/sandbox | architecture_test.go:14 | 0.81 | DELETE | TestArchitectureIntegration_CurrentArchitectureConsistency | Fake table: tt.goarch is never used, the mips row always skips; it re-asserts the GOARCH mapping the partner already checks. |
| TestTmpfsMounts_NoOwnershipOptions | internal/eval/sandbox | mounts_test.go:162 | 0.03 | DELETE | TestNewHostConfigWithMounts_Tmpfs | Partner already checks exactly 3 entries, no uid=/gid=, and the home options. |
| TestPodmanSocketCandidates_Darwin | internal/eval/sandbox | container_daemon_test.go:92 | 0.03 | MERGE | TestPodmanSocketCandidates_Linux | Same (goos, env, uid) -> []path shape; one table. |
| TestBaseImageTag_ArgOrderIndependent | internal/eval/sandbox | baseimage_test.go:86 | 0.03 | KEEP |  | Tag determinism under map-order changes; distinct property of the hash. |
| TestWithOpenUmask_DoesNotMutateInput | internal/eval/sandbox | mounts_test.go:334 | 0.03 | DELETE |  | Implementation copies into a fresh slice; guards an implausible regression, and the behavioural umask tests cover the function. |
| TestKiroCLIVersionedURL_PlatformMapping | internal/eval/sandbox | architecture_test.go:57 | 0.06 | KEEP |  | Exact URL per platform plus error cases; keep this one of the three near-identical URL tables. |
| TestFakeGH_PRCreateURL | internal/eval/sandbox | mock_github_test.go:198 | 0.05 | KEEP |  | Script contract: pr create returns a /pull/N URL (issue create test only covers /issues/). |
| TestContainerTimeout | internal/eval/sandbox | container_test.go:204 | 11.22 | DELETE | TestContainer_ExecWithStdin | Partner's cancellation subtest asserts ErrorIs DeadlineExceeded and promptness; this one only conditionally asserts and costs a 10s container. |
| TestKiroCLINotInstalledInBaseImage | internal/eval/sandbox | integration_installation_test.go:21 | 10.21 | DELETE |  | Asserts stock alpine:3.19 has no kiro-cli (third-party image property) via plain ExecWithOutput; no kairon logic, 10s container. |
| TestNewHostConfigWithMounts_PreservesLimitsAndNetwork | internal/eval/sandbox | mounts_test.go:179 | 0.04 | KEEP |  | Non-default limits prove passthrough and NetworkMode none is a security property (TestResourceLimits uses only defaults). |
| TestFakeGH_IssueViewFailures | internal/eval/sandbox | mock_github_test.go:268 | 0.33 | KEEP |  | Script contract: 12 failure modes, each exits 1, prints no partial JSON, still logged. |
| TestResourceLimitsEnforcement | internal/eval/sandbox | container_test.go:124 | 10.22 | REWRITE-CHEAPER |  | Only test that the cgroup memory limit really applies, but it starts its own 10s container; move the cgroup read into TestArchitectureIntegration_ResourceLimitsAcrossArchitectures, which already starts a 64MB container and asserts only an echo. |
| TestPodmanDiscovery_DefaultEndpointWins | internal/eval/sandbox | container_daemon_test.go:177 | 0.03 | DELETE | TestDiscoverDaemonHost_DefaultEndpointAnswers | Tests the test-only helper ensureContainerDaemon; the production discoverDaemonHost branch is tested by the partner. |
| TestFakeGH_BodyFileUnwritableEvalDirFailsFast | internal/eval/sandbox | mock_github_test.go:592 | 0.05 | KEEP |  | Regression for a real hang in the script's claim loop. |
| TestKiroCLIVerification_Detailed | internal/eval/sandbox | installation_test.go:245 | 0.04 | MERGE | TestRuntimeVerification_DoesNotInstall | Same no-container failure path (ValidateKiroCLI wraps verifyKiroCLIInstallation); fold its message assertion into the partner. |
| TestBaseImageTag_HasImageNamePrefix | internal/eval/sandbox | baseimage_test.go:27 | 0.03 | KEEP |  | Prefix contract is what stops CreateWithPlatform from pulling the image; regex pins format. |
| TestPodmanSocketCandidates_LinuxRoot | internal/eval/sandbox | container_daemon_test.go:87 | 0.04 | MERGE | TestPodmanSocketCandidates_Linux | Same function, uid=0 row; one table. |
| TestPodmanDiscovery_ExplicitDockerHostIsNotOverridden | internal/eval/sandbox | container_daemon_test.go:189 | 0.04 | DELETE | TestDiscoverDaemonHost_ExplicitDockerHostSkipsProbing | Tests the test-only helper; production behaviour is tested by the partner. |
| TestFakeGH_NeverInvokesAnotherGH | internal/eval/sandbox | mock_github_test.go:393 | 0.17 | KEEP |  | Behavioural trap proving the fake never shells out to a real gh (safety contract). |
| TestDetectHostArchitecture_Unsupported | internal/eval/sandbox | installation_test.go:208 | 0.03 | DELETE | TestArchitectureIntegration_CurrentArchitectureConsistency | Misnamed (tests the supported path); same assertion as the partner. |
| TestKiroCLIInstallation_VerificationLogic | internal/eval/sandbox | container_test.go:399 | 10.56 | REWRITE-CHEAPER |  | Only real-container run of verifyKiroCLIInstallation's not-found branch, but it pays a 10s container and asserts only Error; make it a subtest of TestContainer_Lifecycle's container asserting the 'not found at /usr/local/bin/kiro-cli' message. |
| TestFakeGH_IssueCreateLogsAndCopiesBody | internal/eval/sandbox | mock_github_test.go:137 | 0.11 | KEEP |  | Script contract: URL numbering, log line, byte-exact body copy. |
| TestPlatformSpecificImagePulling | internal/eval/sandbox | architecture_test.go:198 | 10.33 | DELETE | TestArchitectureIntegration_ContainerLifecycle | Same CreateWithPlatform(host arch)+Start+exec as the partner's current-arch row; never verifies any pull despite the name; 10s. |
| TestNewHostConfigWithMounts_BindFlags | internal/eval/sandbox | mounts_test.go:24 | 0.03 | KEEP |  | Pins bind type, RO flags, CreateMountpoint=false, no legacy Binds. |
| TestFakeGH_SimulatedCommandsSucceed | internal/eval/sandbox | mock_github_test.go:359 | 0.26 | KEEP |  | Script contract: the simulated allowlist exits 0. |
| TestFakeGH_LogQuotingAndSingleLine | internal/eval/sandbox | mock_github_test.go:308 | 0.19 | KEEP |  | Script contract: shell-quoted, one line per call in gh.log (graders parse it). |
| TestExecWithOutput_ErrorHandling | internal/eval/sandbox | container_test.go:364 | 10.39 | MERGE | TestContainer_ExecWithStdin | Partner's 'ExecWithOutput semantics unchanged' asserts the exact error string; move the nonexistent-binary case into the partner's container and drop this 10s container. |
| TestBaseDockerfile_ToolsOnly | internal/eval/sandbox | baseimage_test.go:134 | 0.04 | KEEP |  | Static contract on the embedded Dockerfile (no COPY/ADD, no toolchains, USER sandbox last); coverage cannot see it. |
| TestWithOpenUmask_PreservesArguments | internal/eval/sandbox | mounts_test.go:342 | 0.05 | KEEP |  | Runs the wrapper under a real sh with hostile args; proves argv survives verbatim. |
| TestDiscoverDaemonHost_DefaultEndpointAnswers | internal/eval/sandbox | container_daemon_test.go:207 | 0.04 | KEEP |  | Production branch: no DOCKER_HOST override when the default endpoint answers. |
| TestWriteFakeGH | internal/eval/sandbox | mock_github_test.go:424 | 0.03 | KEEP |  | Mode 0755, exact content, and mode repair on rewrite. |
| TestRunningUnderGoTest | internal/eval/sandbox | linuxbin_test.go:129 | 0.03 | KEEP |  | Only positive check of the guard that stops a go test binary being shipped into the container. |
| TestNewHostConfigWithMounts_SELinuxLabelDisable | internal/eval/sandbox | mounts_test.go:288 | 0.03 | KEEP |  | label=disable present only when enforcing. |
| TestFakeGH_MissingBodyFileFailsButIsLogged | internal/eval/sandbox | mock_github_test.go:177 | 0.05 | KEEP |  | Script contract: failed copy is logged and does not claim a body slot. |
| TestRenderGHIssue_ExplicitFieldsAndEmptyLabels | internal/eval/sandbox | mock_github_test.go:499 | 0.03 | KEEP |  | Explicit fields and labels rendered as [] not null; TestRenderGHIssue uses defaults. |
| TestInstallationVerification_PermissionChecks | internal/eval/sandbox | installation_test.go:63 | 0.03 | DELETE |  | Writes its own temp files and asserts os.Stat modes; touches no production code. |
| TestPodmanSocketCandidates_UnknownOS | internal/eval/sandbox | container_daemon_test.go:102 | 0.03 | MERGE | TestPodmanSocketCandidates_Linux | Same function, unknown-OS row; one table. |
| TestWithOpenUmask_AppliesUmask | internal/eval/sandbox | mounts_test.go:374 | 0.05 | KEEP |  | Behavioural proof that files are world read/write under the wrapper. |
| TestFakeGH_ConcurrentCreatesGetUniqueNumbers | internal/eval/sandbox | mock_github_test.go:537 | 0.2 | KEEP |  | Regression for duplicate numbers under concurrency in the script. |
| TestValidateKiroCLI_Verification | internal/eval/sandbox | installation_test.go:215 | 0.04 | DELETE | TestRuntimeVerification_DoesNotInstall | Body is identical to the partner (ValidateKiroCLI on a non-running container, assert Error). |
| TestToolSetBuildArgs | internal/eval/sandbox | baseimage_test.go:98 | 0.03 | KEEP |  | Exact build args per platform, no /latest/. |
| TestFakeGHScript_NoExternalGHInvocation | internal/eval/sandbox | mock_github_test.go:414 | 0.03 | KEEP |  | Cheap static scan of every script line; complements the 5-path behavioural trap. |
| TestFakeGH_BodyFileForms | internal/eval/sandbox | mock_github_test.go:156 | 0.12 | KEEP |  | Script contract: -F and --body-file= forms. |
| TestWithOpenUmask_Shape | internal/eval/sandbox | mounts_test.go:321 | 0.03 | DELETE |  | Pins the literal argv; PreservesArguments and AppliesUmask already fail if the wrapper shape breaks. |
| TestNewHostConfigWithMounts_NoWorkspaceTmpfs | internal/eval/sandbox | mounts_test.go:71 | 0.03 | MERGE | TestNewHostConfigWithMounts_Tmpfs | Partner's exactly-3-named-entries check already implies it; add the explicit no-/workspace assertion there. |
| TestArchitectureDetectionInContainer | internal/eval/sandbox | architecture_test.go:148 | 10.37 | DELETE | TestArchitectureIntegration_ContainerLifecycle | uname -m in alpine is a kernel/image property and the partner already checks it per platform; 10s. |
| TestSandboxHomeWritableAsSandboxUser | internal/eval/sandbox | container_test.go:164 | 10.32 | KEEP |  | Real host config: sandbox uid/gid, writable tmpfs home and /tmp, read-only rootfs, Podman tmpfs acceptance. |
| TestNewHostConfigWithMounts_Tmpfs | internal/eval/sandbox | mounts_test.go:113 | 0.03 | KEEP |  | Pins tmpfs paths, options, no noexec, no uid/gid. |
| TestFakeGH_UnsimulatedCommandFailsAndIsLogged | internal/eval/sandbox | mock_github_test.go:332 | 0.18 | KEEP |  | Script safety contract: destructive commands fail and are logged. |
| TestBaseImageTag_FieldBoundariesDoNotCollide | internal/eval/sandbox | baseimage_test.go:92 | 0.04 | KEEP |  | Hash input encoding must not let key/value boundaries collide. |
| TestPlatformSpecificBinaries | internal/eval/sandbox | installation_test.go:151 | 0.04 | DELETE | TestToolSetBuildArgs | Partner asserts the exact KIRO_CLI_URL per platform; PlatformMapping asserts the URL. |
| TestResolveLinuxBinary_SkipsGoTestExecutable | internal/eval/sandbox | linuxbin_test.go:138 | 0.03 | KEEP |  | On Linux it is the only end-to-end check that ResolveLinuxBinary never returns the test binary (the darwin branch only re-runs a TestPlanLinuxBinary row and could skip). |
| TestKiroCLIInstallationMocking | internal/eval/sandbox | architecture_test.go:234 | 10.26 | DELETE | TestArchitectureIntegration_KiroCLIInstallation | Near line-for-line copy of the partner (uname -m then URL mapping); mocks nothing; 10s. |
| TestFakeGH_BodyFileStdin | internal/eval/sandbox | mock_github_test.go:168 | 0.07 | KEEP |  | Script contract: --body-file - reads stdin. |
| TestNewHostConfigWithMounts_ResolvesSymlinks | internal/eval/sandbox | mounts_test.go:230 | 0.03 | KEEP |  | Symlinked host path resolved before mounting. |
| TestBaseImageTag_PureAcrossCwdEnvAndEvalsDir | internal/eval/sandbox | baseimage_test.go:63 | 0.05 | KEEP |  | Pins the AC 8 invariant that cwd/env/evals dir do not change the tag; cheap guard against hidden inputs. |
| TestFakeGH_DanglingBodySymlinkIsCollisionNotFailure | internal/eval/sandbox | mock_github_test.go:632 | 0.06 | KEEP |  | Regression for the claim loop misreading a dangling symlink. |
| TestNewHostConfigWithMounts_PublicUsesHostProbe | internal/eval/sandbox | mounts_test.go:309 | 0.03 | KEEP |  | Public wrapper is wired to the real SELinux probe. |
| TestExistingSockets_FiltersMissingAndNonSockets | internal/eval/sandbox | container_daemon_test.go:106 | 0.04 | KEEP |  | Only check that regular files and missing paths are filtered out. |
| TestKiroCLIDownloadURL_SupportedPlatforms | internal/eval/sandbox | installation_test.go:18 | 0.03 | DELETE | TestKiroCLIVersionedURL_PlatformMapping | Same function, same rows, same assertions as the partner. |
| TestSkipIfNoContainerDaemon_MessageNamesPodmanAndDocker | internal/eval/sandbox | container_daemon_test.go:198 | 0.04 | DELETE |  | Asserts the wording of a test-only skip message. |
| TestResourceLimits_Coverage | internal/eval/sandbox | installation_test.go:229 | 0.03 | DELETE | TestResourceLimits | NotZero/NotNil weaker copy of TestResourceLimits; the name says it exists for coverage. |
| TestFakeGH_IssueViewText | internal/eval/sandbox | mock_github_test.go:250 | 0.1 | KEEP |  | Script contract: text view fields; the JSON test does not cover text output. |

### Counts

| verdict | count | total secs |
|---|---|---|
| KEEP | 56 | 14.63 |
| MERGE | 24 | 12.08 |
| REWRITE-CHEAPER | 6 | 73.24 |
| DELETE | 28 | 62.62 |
| total | 114 | 162.57 |

### Recurring patterns

1. **Daemon tests that start their own 10s alpine container to repeat a lifecycle another container test already checks.** Most add nothing beyond `uname -m`/`echo`, or a URL mapping already unit-tested. Delete these. Move the few distinct checks (cgroup limit, verify not-found message, nonexistent binary) into containers that already exist. Examples: TestPlatformSpecificImagePulling, TestArchitectureDetectionInContainer, TestKiroCLIInstallationMocking (the "Mocking" test mocks nothing), TestContainerTimeout, TestExecWithOutput_ErrorHandling.
2. **Copy-pasted tests of the same pure function.** The same table appears two or three times. Examples: TestKiroCLIDownloadURL_SupportedPlatforms vs TestKiroCLIVersionedURL_PlatformMapping (and TestPlatformSpecificBinaries); TestValidateKiroCLI_Verification vs TestRuntimeVerification_DoesNotInstall; TestDetectHostArchitecture vs TestDetectHostArchitecture_Unsupported vs TestArchitectureIntegration_CurrentArchitectureConsistency.
3. **Tests that check the test's own scaffolding, a library, or fake data.** Examples: TestInstallationVerification_PermissionChecks (only os.Stat on its own temp files); TestTrendRestoresFlagVars (only the test helper); TestPodmanDiscovery_DefaultEndpointWins and TestSkipIfNoContainerDaemon_MessageNamesPodmanAndDocker (test-only daemon helper); TestAuditSkillsFSWiredFromTemplates (fs.Sub on an empty embed.FS).
4. **CLI tests that run the whole eval suite just to prove a flag is wired.** internal/eval already tests prompt_sha256, repeat counting and ErrThresholdFailed. The CLI only needs one filtered case (`evalFilter = name==stub-basic`, the same approach TestEvalFilterNarrowsRun uses). Examples: TestEvalPromptFileRecorded (11.4s), TestEvalRepeatPassedToRun (10.4s), TestEvalSelfTestFailExitsWithThresholdError (6.3s); TestEvalSelfTestSucceedsWithDefaults just repeats the first of these.
5. **Per-command "FlagsRegistered"/"Args"/"Help" tests that restate the declarations.** Fold defaults, required flags, which flags subcommands inherit, and argument counts into one table-driven CLI-surface test. Delete the help-text phrase checks. Examples: TestAuditFlagsRegistered, TestTrendFlagsRegistered, TestEvalRepeatFlagRegistered, TestAuditArgs/TestTrendArgs/TestVerifyLogMissingAgentIsUsageError, TestAuditHelpDescribesRubricRules.

Not a pattern but worth noting: the 18 fake-gh script tests (17 TestFakeGH_* plus TestFakeGHScript_NoExternalGHInvocation) and TestWorktreeCreate show zero or near-zero Go coverage only because they test shell scripts. They are real contract tests and are kept. TestWorktreeCreate needs parallel subtests and a shared fixture template (24s now).


## Group: tui-checks-inference

### Test audit verdicts: internal/tui, internal/checks, internal/inference

207 candidates, read by hand. Partners named are tests to merge into or the test that already makes the assertion.

### 1. Verdicts

| test | pkg | file:line | secs | verdict | partner | reason |
|---|---|---|---|---|---|---|
| TestKiroCLIAgentCommand | internal/inference | exec_test.go:15 | 0.57 | KEEP |  | Pins the kiro-cli argv (shared with the container path) and that the prompt never lands in argv; the 0.57s is package start-up noise, not the test. |
| TestKiroCLIAgentResponse | internal/inference | exec_test.go:46 | 0.05 | KEEP |  | ANSI stripping, model echo and usage estimation of the shared response builder. |
| TestRequestResponseJSONRoundTrip | internal/inference | exec_test.go:75 | 0.04 | KEEP |  | Catches a Request/Response field that cannot cross the container JSON wire (coverage-blind). |
| TestServeExec_StubReported | internal/inference | exec_test.go:128 | 0.04 | KEEP |  | Single-JSON-document envelope plus reported usage over the wire. |
| TestServeExec_StubEstimated | internal/inference | exec_test.go:155 | 0.04 | MERGE | TestServeExec_StubReported | Same ServeExec->Decode round trip as the partner, differing only in estimated vs reported usage. |
| TestServeExec_BackendErrorInEnvelope | internal/inference | exec_test.go:186 | 0.04 | KEEP |  | Backend errors travel in the envelope with the partial response, and are not ErrTimeout. |
| TestServeExec_UnknownBackend | internal/inference | exec_test.go:218 | 0.05 | KEEP |  | Also asserts nothing is written to stdout on error, which the container caller parses. |
| TestServeExec_BadInput | internal/inference | exec_test.go:229 | 0.05 | KEEP |  | Decode-error contract of the container exec helper, cheap unit-level. |
| TestDecodeExecResult_RoundTrip | internal/inference | exec_test.go:248 | 0.03 | DELETE | TestServeExec_StubReported | Duplicates the envelope decode already asserted by TestServeExec_StubReported/BackendErrorInEnvelope, and Response field passthrough is covered by TestRequestResponseJSONRoundTrip. |
| TestDecodeExecResult_Garbage | internal/inference | exec_test.go:286 | 0.04 | KEEP |  | Garbage or empty helper output must be an error, not a zero Response. |
| TestInferenceDoesNotImportEval | internal/inference | exec_test.go:297 | 0.14 | KEEP |  | Architectural boundary (inference must not link eval) ahead of the planned eval split; coverage can't see it. |
| TestKiroCLIAgentCommand_ResumeNeverTouchesPrompt | internal/inference | exec_turn_test.go:89 | 0.04 | MERGE | TestKiroCLIAgentCommand | Same prompt-never-in-argv property as the partner's third subtest, just with Turn:2; add it as a row. |
| TestNew_KnownBackends | internal/inference | inference_test.go:9 | 0.04 | KEEP |  | Registry wiring: each registered name constructs a backend reporting that name. |
| TestNew_UnknownBackend | internal/inference | inference_test.go:21 | 0.04 | KEEP |  | User-facing error naming the valid backends. |
| TestNames_Sorted | internal/inference | inference_test.go:35 | 0.04 | DELETE | TestNew_UnknownBackend | The sorted backend list is already pinned verbatim in TestNew_UnknownBackend's error message. |
| TestEstimateUsage | internal/inference | inference_test.go:43 | 0.06 | KEEP |  | The token estimate formula feeds cost reporting; small direct unit test. |
| TestTimeoutOrDefault | internal/inference | inference_test.go:56 | 0.05 | KEEP |  | Zero/negative-to-default rule is a real behaviour every backend call depends on. |
| TestKiroCLI_AvailableUsesPath | internal/inference | kirocli_test.go:83 | 0.04 | KEEP |  | PATH lookup for availability, both directions. |
| TestKiroCLI_StartupProbeRunsVersion | internal/inference | kirocli_test.go:94 | 0.14 | KEEP |  | Probe must run `--version` and report a positive duration. |
| TestKiroCLI_AgentArgvStdinAndANSI | internal/inference | kirocli_test.go:105 | 0.1 | KEEP |  | End-to-end agent invocation against a fake binary: argv, stdin prompt, ANSI strip, cwd. |
| TestKiroCLI_AgentCapturesStderrSeparately | internal/inference | kirocli_test.go:152 | 0.14 | KEEP |  | Guards that stderr never leaks into the agent Text that checks evaluate. |
| TestKiroCLI_AgentFailure | internal/inference | kirocli_test.go:166 | 0.17 | KEEP |  | Non-zero exit maps to error text, ExitCode, Stderr and empty Text, and is not ErrTimeout. |
| TestKiroCLI_AgentTimeout | internal/inference | kirocli_test.go:189 | 0.38 | REWRITE-CHEAPER |  | Valuable timeout/ErrTimeout contract, but the 300ms timeout against a sleeping fake could be 50ms (update the pinned message) to cut ~0.3s. |
| TestKiroCLI_AgentNotInstalled | internal/inference | kirocli_test.go:208 | 0.1 | KEEP |  | Missing-binary error path for the real backend; nothing else in the package covers it. |
| TestKiroCLI_JudgeArgvStdinRawOutput | internal/inference | kirocli_test.go:216 | 0.11 | KEEP |  | Judge argv/stdin and raw (non-ANSI-stripped) output are a distinct contract from the agent path. |
| TestKiroCLI_UnsupportedRole | internal/inference | kirocli_test.go:265 | 0.05 | KEEP |  | Role validation on the real backend; trivial cost. |
| TestKiroCLI_AgentConfigDirOverlay | internal/inference | kirocli_test.go:290 | 0.14 | KEEP |  | Covers the overlay copy, temp-dir cwd, cleanup and untouched source in one place. |
| TestKiroCLI_WorkDirSetsCmdDir | internal/inference | kirocli_workdir_test.go:14 | 0.1 | MERGE | TestKiroCLI_WorkDirTakesPrecedenceOverOverlay | Same cwd==WorkDir assertion the partner makes; fold in as a no-overlay row. |
| TestKiroCLI_WorkDirTakesPrecedenceOverOverlay | internal/inference | kirocli_workdir_test.go:29 | 0.11 | KEEP |  | Precedence rule (WorkDir beats overlay, overlay never written into WorkDir) is real and unique. |
| TestKiroCLI_EmptyWorkDirKeepsOverlayBehavior | internal/inference | kirocli_workdir_test.go:49 | 0.15 | DELETE | TestKiroCLI_AgentConfigDirOverlay | Same overlay-temp-dir-not-cwd assertion that TestKiroCLI_AgentConfigDirOverlay already makes (plus a temp-dir name prefix). |
| TestPriceTable_Cost_Known | internal/inference | pricing_test.go:18 | 0.05 | KEEP |  | Anchor for the per-MTok cost formula; becomes the table-driven Cost test. |
| TestPriceTable_Cost_UsesPerModelPrice | internal/inference | pricing_test.go:29 | 0.05 | MERGE | TestPriceTable_Cost_Known | Same Cost() calculation with another model; one row in a Cost table. |
| TestPriceTable_Cost_UnknownModel | internal/inference | pricing_test.go:36 | 0.04 | KEEP |  | Pins exact-id lookup (no case/space folding), which decides whether cost is reported as unknown. |
| TestPriceTable_Cost_NilAndEmptyTable | internal/inference | pricing_test.go:46 | 0.05 | MERGE | TestPriceTable_Cost_Known | Nil/empty table rows of the Cost table. |
| TestPriceTable_Cost_NoTokensIsKnownZero | internal/inference | pricing_test.go:55 | 0.04 | MERGE | TestPriceTable_Cost_Known | Zero-token rule is another input row of the same Cost table. |
| TestPriceTable_Cost_OneSidedTokensNeedPrice | internal/inference | pricing_test.go:66 | 0.04 | MERGE | TestPriceTable_Cost_Known | Another input row of the Cost table. |
| TestPriceTable_UnmarshalYAML_Map | internal/inference | pricing_test.go:114 | 0.04 | KEEP |  | Custom UnmarshalYAML rejects incomplete price entries; config-loading contract. |
| TestCallRecord_JSONFieldNames | internal/inference | record_test.go:9 | 0.04 | KEEP |  | Pins the on-disk call-record keys that reports/history consume; coverage is blind to struct tags. |
| TestCallRecord_RoundTrip | internal/inference | record_test.go:36 | 0.04 | DELETE |  | A Marshal/Unmarshal round trip over plain tagged fields only exercises encoding/json; field names are pinned elsewhere. |
| TestCallRecord_OmitEmptyOptionalFields | internal/inference | record_test.go:55 | 0.04 | KEEP |  | Pins which record fields are optional vs always present, which the legacy record shape depends on. |
| TestCallRecord_StepSourceOmittedWhenEmpty | internal/inference | record_test.go:81 | 0.05 | KEEP |  | Byte-exact legacy record shape guards compatibility with existing results on disk. |
| TestCallRecord_StepSourceMarshalWhenSet | internal/inference | record_test.go:94 | 0.04 | MERGE | TestCallRecord_JSONFieldNames | Another key-name check for optional fields; fold into the field-name table. |
| TestCallRecord_StepSourceRoundTrip | internal/inference | record_test.go:116 | 0.04 | DELETE |  | Round trip only exercises encoding/json, and it also asserts a constant (SourceWorkspace == "workspace"). |
| TestCallRecord_CostUnknown | internal/inference | record_test.go:138 | 0.03 | MERGE | TestCallRecord_OmitEmptyOptionalFields | cost_unknown omitempty/round trip is another optional-field row; its 11-field count duplicates TestCallRecord_JSONFieldNames. |
| TestCallRecord_LegacyJSONDecodesAsKnown | internal/inference | record_test.go:181 | 0.04 | DELETE |  | Asserts that a missing bool decodes as false, which is encoding/json behaviour. |
| TestCallRecord_TurnJSON | internal/inference | record_turn_test.go:11 | 0.03 | MERGE | TestCallRecord_OmitEmptyOptionalFields | Turn omitempty/presence is another row of the optional-field table. |
| TestStub_CommandsCreateFileInWorkDir | internal/inference | stub_commands_test.go:28 | 0.08 | DELETE | TestStub_TurnSelectionAndCommandsPerTurn | Partner already proves commands run in WorkDir and the turn text is returned. |
| TestStub_CommandsRunInOrder | internal/inference | stub_commands_test.go:46 | 0.08 | MERGE | TestStub_TurnSelectionAndCommandsPerTurn | Multiple commands per turn run in order; add a two-command turn to the partner. |
| TestStub_CommandsEmptyWorkDirRunsNothing | internal/inference | stub_commands_test.go:59 | 0.03 | KEEP |  | Safety: commands with no WorkDir must refuse and must not run in the caller's cwd. |
| TestStub_NoCommandsIgnoresEmptyWorkDir | internal/inference | stub_commands_test.go:79 | 0.03 | KEEP |  | The other half of the WorkDir rule: no commands means an empty WorkDir is fine. |
| TestStub_CommandFailureReturnsStderrAndNoText | internal/inference | stub_commands_test.go:89 | 0.05 | KEEP |  | Checks error text carries command+stderr, exit code passthrough and not-a-timeout; partner asserts none of these. |
| TestStub_FailedCommandStopsLaterCommands | internal/inference | stub_commands_test.go:113 | 0.05 | KEEP |  | Unique assertion that a later command never runs after a failing one (partner only checks denials). |
| TestStub_CommandTimeoutKillsProcessGroupPromptly | internal/inference | stub_commands_test.go:125 | 1.06 | REWRITE-CHEAPER |  | Process-group kill is worth testing, but a 1s timeout against sleep 3 costs ~1s; use a ~100ms timeout with the same <2s bound and update the pinned message. |
| TestRequestAndStubTurn_JSONRoundTripKeepsWorkDirAndCommands | internal/inference | stub_commands_test.go:164 | 0.04 | MERGE | TestRequestResponseJSONRoundTrip | Same Request JSON round-trip as the partner; add WorkDir and Commands to the partner's fixture. |
| TestStubTurn_CommandsOmittedWhenEmpty | internal/inference | stub_commands_test.go:207 | 0.04 | DELETE |  | Asserts omitempty on a StubTurn that is only decoded by the same struct (nil either way); no consumer depends on the key's absence. |
| TestNextJudgeAnswer_CyclesInOrder | internal/inference | stub_judge_test.go:45 | 0.04 | KEEP |  | Unit-level contract that scripted judge answers cycle; cheap and direct. |
| TestNextJudgeAnswer_EmptyJudge | internal/inference | stub_judge_test.go:55 | 0.03 | KEEP |  | The empty-script ("", false) return is the contract callers use to fall back; cheap and unique. |
| TestNextJudgeAnswer_ScriptsAreIndependent | internal/inference | stub_judge_test.go:62 | 0.04 | DELETE |  | Asserts that two struct values do not share a field (plain Go semantics), which no plausible change could break. |
| TestNextJudgeAnswer_Concurrent | internal/inference | stub_judge_test.go:71 | 0.04 | KEEP |  | Under -race this proves the cursor is mutex-protected for parallel checks. |
| TestStub_YesNoJudge_CursorPersistsAcrossInvokes | internal/inference | stub_judge_test.go:97 | 0.04 | KEEP |  | Proves the backend advances the script's own cursor across Invoke calls (not a copy), which multi-check cases rely on. |
| TestStub_YesNoJudge_Normalizes | internal/inference | stub_judge_test.go:135 | 0.04 | KEEP |  | Pins case/whitespace normalisation of scripted yes/no answers, which the judge parser relies on. |
| TestStub_YesNoJudge_OtherValueVerbatim | internal/inference | stub_judge_test.go:154 | 0.03 | KEEP |  | Lets downstream judge-parse-error tests script garbage; asserts it is not wrapped in delimiters. |
| TestStub_LegacyJudgeUnchangedWithoutYesNo | internal/inference | stub_judge_test.go:181 | 0.04 | KEEP |  | Guards the legacy score judge path and that it does not consume the yes/no cursor. |
| TestStubRoute_ReturnsRouteBySeq | internal/inference | stub_route_test.go:39 | 0.04 | KEEP |  | Core contract of the simulated-user routing stub: the route is chosen by seq and wrapped as stub/estimated. |
| TestStubRoute_ExhaustedIsError | internal/inference | stub_route_test.go:59 | 0.04 | KEEP |  | Out-of-range seq must fail closed with an indexed error; becomes the route error table. |
| TestStubRoute_NoScript | internal/inference | stub_route_test.go:74 | 0.03 | MERGE | TestStubRoute_ExhaustedIsError | Same 'stub.routes[n]' error assertion with nil/empty scripts; one error-case table. |
| TestStubRoute_Stateless | internal/inference | stub_route_test.go:96 | 0.03 | KEEP |  | Out-of-order/repeated seq returns the same route: a distinct property from by-seq lookup. |
| TestStubRoute_DoesNotTouchJudgeCursor | internal/inference | stub_route_test.go:118 | 0.05 | KEEP |  | Route calls must not advance the yes/no cursor: isolation between two stub scripts. |
| TestStubRoute_NonRouteJudgeUnchanged | internal/inference | stub_route_test.go:129 | 0.05 | MERGE | TestStubRoute_DoesNotTouchJudgeCursor | The reverse direction of the same route/judge isolation property; fold into one isolation test. |
| TestStubScript_RoutesYAML | internal/inference | stub_route_test.go:148 | 0.04 | KEEP |  | Pins the `routes:` YAML key that eval case files are written with (struct-tag contract, coverage-blind). |
| TestCallRecordPurposeOmittedWhenEmpty | internal/inference | stub_route_test.go:158 | 0.04 | MERGE | TestCallRecord_OmitEmptyOptionalFields | Same omitempty-shape check on CallRecord as its partner, just for the purpose field; fold into one field table. |
| TestStub_AvailabilityAndProbe | internal/inference | stub_test.go:19 | 0.04 | DELETE |  | Asserts the hard-coded `return nil` / `return 0` of stubBackend.Available/StartupProbe (constants). |
| TestStub_AgentReturnsScriptedResponseEstimated | internal/inference | stub_test.go:31 | 0.04 | KEEP |  | Pins the 4-chars-per-token estimate and empty Model for the scripted stub agent path. |
| TestStub_AgentReportedUsageAndModel | internal/inference | stub_test.go:54 | 0.04 | KEEP |  | Reported usage/model must override estimation; distinct from the estimated path. |
| TestStub_AgentMissingTurn | internal/inference | stub_test.go:77 | 0.04 | KEEP |  | Error-message contract for missing or out-of-range scripted turns. |
| TestStub_JudgeReturnsPassingDelimitedJSON | internal/inference | stub_test.go:107 | 0.05 | KEEP |  | Legacy judge output must parse as delimited passing JSON; contract with the judge parser. |
| TestStub_AgentIgnoresRequestModel | internal/inference | stub_test.go:151 | 0.04 | KEEP |  | Pins that the stub never echoes Request.Model (unlike kiro-cli), which cost tests rely on. |
| TestStub_ToolCallTrustedRunsInWorkDir | internal/inference | stub_toolcalls_test.go:32 | 0.05 | MERGE | TestStub_ToolCallDeniedIsSkippedAndRecorded | Positive row of the same trust-gate table (trusted call runs in WorkDir). |
| TestStub_ToolCallDeniedIsSkippedAndRecorded | internal/inference | stub_toolcalls_test.go:55 | 0.04 | KEEP |  | Security gate: proves an untrusted tool call's command does not run and is recorded with a reason (the wire test doesn't check the side effect). |
| TestStub_ToolCallNilTrustIsUnrestricted | internal/inference | stub_toolcalls_test.go:96 | 0.1 | MERGE | TestStub_ToolCallDeniedIsSkippedAndRecorded | nil-trust row of the same trust-gate table. |
| TestStub_ToolCallEmptyTrustDeniesEverything | internal/inference | stub_toolcalls_test.go:114 | 0.04 | MERGE | TestStub_ToolCallDeniedIsSkippedAndRecorded | Same gate with a trust-nothing input; fold into a trust-set table with the denied/trusted/nil cases. |
| TestStub_DeniedToolCallNeedsNoWorkDir | internal/inference | stub_toolcalls_test.go:166 | 0.04 | KEEP |  | Distinct rule: a denied call needs no WorkDir and must not error. |
| TestStub_UnreachedDenialsAfterFailureAreNotRecorded | internal/inference | stub_toolcalls_test.go:217 | 0.05 | KEEP |  | Ordering edge: denials after a halting failure must not be reported. |
| TestStubTurn_ToolCallsOmittedWhenEmpty | internal/inference | stub_toolcalls_test.go:253 | 0.05 | DELETE |  | Omitempty on an internal stub struct that only the same struct decodes; key absence is incidental. |
| TestToolTrust_CSVAndNames | internal/inference | trust_test.go:68 | 0.04 | KEEP |  | CSV feeds --trust-tools; also checks the Names copy and non-nil empty set. |
| TestKiroCLIAgentResponse_CommandReflectsToolTrust | internal/inference | trust_test.go:153 | 0.04 | KEEP |  | Pins the --trust-tools= argv when trust is restricted (security-relevant). |
| TestRequestResponse_ToolTrustJSONRoundTrip | internal/inference | trust_test.go:161 | 0.04 | KEEP |  | nil vs empty ToolTrust means unrestricted vs deny-all; that distinction has to survive the container wire. |
| TestCallRecord_TrustedToolsJSON | internal/inference | trust_test.go:240 | 0.06 | KEEP |  | Pins the meaningful nil-vs-[] distinction for trusted_tools (unrestricted vs trust-nothing) in persisted records. |
| TestChecksDocsCoversEveryCheckType | internal/checks | checks_docs_test.go:81 | 0.7 | KEEP |  | Contract that every implemented check type has a parseable YAML example in docs. |
| TestChecksDocsStatesKeyRules | internal/checks | checks_docs_test.go:141 | 0.04 | DELETE |  | Pins incidental phrases in docs/evaluation.md; passes even when the docs are wrong. |
| TestDocumentCheckMonthNinetyNineFailsWithParserReason | internal/checks | checks_document_test.go:71 | 0.05 | KEEP |  | The reason this check type exists: parser-backed failure where the regex would pass. |
| TestDocumentCheckFenceInsideBodyIsNotStripped | internal/checks | checks_document_test.go:126 | 0.06 | KEEP |  | Parser edge: an inner code fence must not be treated as the document envelope. |
| TestDocumentCheckValidate | internal/checks | checks_document_test.go:168 | 0.05 | KEEP |  | Field allow-list and turn bounds for output_document_valid through YAML. |
| TestDocumentCheckSpecAllowsOnlyTurn | internal/checks | checks_document_test.go:203 | 0.04 | DELETE | TestDocumentCheckValidate | Asserts the internal checkSpecs entry; TestDocumentCheckValidate proves the same rule through behaviour. |
| TestDocumentCheckResultShape | internal/checks | checks_document_test.go:216 | 0.05 | MERGE | TestJudgeCheckLabel | Index/Type are covered generically by ResultsKeepListOrderAndLabels; the remaining label assertions belong in the label table. |
| TestEvaluateChecks_CommandRunsInDirWithoutStdin | internal/checks | checks_eval_cmd_test.go:30 | 0.12 | KEEP |  | Command checks run in Dir with no stdin, so they cannot hang. |
| TestEvaluateChecks_CommandTimeoutKillsProcessGroup | internal/checks | checks_eval_cmd_test.go:81 | 0.45 | REWRITE-CHEAPER |  | Process-group kill is important, but checkCommandTimeout=400ms could be ~50ms (the sleep 60 child still proves the group kill), saving ~0.35s. |
| TestEvaluateChecks_NonCommandChecksRunBeforeCommands | internal/checks | checks_eval_cmd_test.go:103 | 0.08 | KEEP |  | File checks observe the pre-command tree, which evaluation order exists to guarantee. |
| TestEvaluateChecks_ChangedFilesClean | internal/checks | checks_eval_git_test.go:10 | 0.37 | KEEP |  | Clean-tree baseline and the Base-defaults-to-HEAD rule for changed_files (git setup is unavoidable). |
| TestEvaluateChecks_ChangedFilesIgnoresRepoConfigCommands | internal/checks | checks_eval_git_test.go:83 | 0.35 | KEEP |  | Security: agent-controlled git config (fsmonitor/diff.external) must not execute. |
| TestEvaluateChecks_FileExists | internal/checks | checks_eval_test.go:77 | 0.05 | KEEP |  | file_exists semantics, including directories and dangling symlinks. |
| TestEvaluateChecks_OutputAndGHLog | internal/checks | checks_eval_test.go:172 | 0.05 | KEEP |  | Pass/fail semantics and detail words of the four output/gh-log check types, including the empty log. |
| TestEvaluateChecks_ResultsKeepListOrderAndLabels | internal/checks | checks_eval_test.go:222 | 0.05 | KEEP |  | Result order, labels and the non-nil empty result for nil checks. |
| TestFactChecks_AskedNoDecisions | internal/checks | checks_fact_test.go:68 | 0.04 | KEEP |  | Empty-but-present sim log edge for fact_asked/fact_not_asked. |
| TestFactChecks_ValidateFields | internal/checks | checks_fact_test.go:127 | 0.04 | KEEP |  | Table of fact-check field rules through YAML; not in the partner table. |
| TestFactChecks_Label | internal/checks | checks_fact_test.go:160 | 0.04 | MERGE | TestJudgeCheckLabel | Same checkLabel(i, check) to string mapping for other types; one label table. |
| TestSimDecisionJSONKeys | internal/checks | checks_fact_test.go:173 | 0.05 | KEEP |  | Pins the sim-log JSON keys written by the simulated user and read by the fact checks. |
| TestGlobCompiledReuse | internal/checks | checks_glob_test.go:128 | 0.06 | DELETE | TestGlobMatch | Re-matches cases TestGlobMatch already covers through the same compileGlob/match path. |
| TestJudgeCheckVocabulary | internal/checks | checks_judge_test.go:28 | 0.04 | DELETE | TestJudgeCheckTurnFieldValidation | Asserts a constant and internal checkSpecs flags; the Validate tests already exercise the required/allowed behaviour. |
| TestJudgeCheckLabel | internal/checks | checks_judge_test.go:98 | 0.05 | KEEP |  | Result labels are user-visible in check/eval output, including the long-question shortening. |
| TestJudgeCheckYesPasses | internal/checks | checks_judge_test.go:109 | 0.06 | KEEP |  | Yes verdict passes with reasoning in the user-visible detail. |
| TestJudgeCheckNoFails | internal/checks | checks_judge_test.go:121 | 0.08 | MERGE | TestJudgeCheckYesPasses | Mirror of YesPasses; one verdict table. |
| TestJudgeCheckQueryCarriesEverything | internal/checks | checks_judge_test.go:136 | 0.05 | KEEP |  | Every query field and file reaches the judge. |
| TestJudgeCheckFileCaps | internal/checks | checks_judge_test.go:212 | 0.08 | KEEP |  | Per-file and total byte caps with the truncation marker. |
| TestJudgeCheckManyLargeFilesNoPanic | internal/checks | checks_judge_test.go:279 | 0.05 | MERGE | TestJudgeCheckFileCaps | Same total-budget assertion as FileCaps' "total" subtest with over-cap files; add as a row there. |
| TestJudgeCheckTruncatesOnRuneBoundaryAndSanitizesUTF8 | internal/checks | checks_judge_test.go:312 | 0.04 | KEEP |  | Mid-rune cut edge case, distinct from the invalid-bytes test. |
| TestJudgeCheckKeepsCompleteFinalRuneAtCap | internal/checks | checks_judge_test.go:416 | 0.04 | KEEP |  | Off-by-one edge: a complete rune at the cap is kept (the opposite edge from mid-rune truncation). |
| TestJudgeCheckJudgeErrorFails | internal/checks | checks_judge_test.go:439 | 0.06 | KEEP |  | Backend failure must fail the check with the cause in the detail. |
| TestJudgeCheckParseErrorFails | internal/checks | checks_judge_test.go:445 | 0.05 | KEEP |  | Parse errors are reported distinctly from call failures, without a doubled prefix. |
| TestJudgeCheckNilJudgeFails | internal/checks | checks_judge_test.go:457 | 0.04 | DELETE | TestJudgeCheckNilJudgeFailsEvenWithFiles | Identical assertion to the partner minus the file. |
| TestJudgeCheckRunsInNonCommandPassInListOrder | internal/checks | checks_judge_test.go:471 | 0.08 | KEEP |  | Judges run in list order before commands, and result order/index is preserved. |
| TestJudgeCheckFilesReadBeforeCommandChecks | internal/checks | checks_judge_test.go:504 | 0.08 | MERGE | TestJudgeCheckRunsInNonCommandPassInListOrder | Same ordering property (judge runs before commands); add the file-content assertion to the partner. |
| TestJudgeCheckEvaluatorStaysIndependent | internal/checks | checks_judge_test.go:529 | 0.05 | DELETE |  | Asserts that an output_contains check doesn't call the judge, which type dispatch makes trivially true. |
| TestJudgeCheckMultiTurnDefaultsToLastTurn | internal/checks | checks_judge_test.go:639 | 0.05 | KEEP |  | A multi-turn judge defaults to the last turn with the full transcript. |
| TestJudgeCheckMultiTurnSelectsEarlierTurn | internal/checks | checks_judge_test.go:655 | 0.04 | KEEP |  | An explicit earlier turn gets only that turn's transcript. |
| TestJudgeCheckOneElementTurnsHasNoTranscript | internal/checks | checks_judge_test.go:714 | 0.05 | MERGE | TestJudgeCheckClassicInputHasNoTranscript | Same no-transcript/out-of-range assertions with a one-element Turns input; one table. |
| TestChecksYAMLDecodesChecksList | internal/checks | checks_test.go:49 | 0.04 | KEEP |  | Pins the YAML keys case authors write (coverage-blind struct tags). |
| TestChecksYAMLAllowEmptyVersusMissing | internal/checks | checks_test.go:85 | 0.04 | KEEP |  | Validation depends on allow: [] vs missing decoding differently, and the validate table only builds structs. |
| TestChecksCheckTypeSet | internal/checks | checks_test.go:101 | 0.04 | DELETE | TestChecksDocsCoversEveryCheckType | Asserts string constants equal themselves and is already stale (missing fact/document types); the docs-coverage test enumerates the real set. |
| TestChecksValidateNilCriterionSkipsCrossCheck | internal/checks | checks_test.go:299 | 0.05 | MERGE | TestChecksValidateOptionalCriterion | Nil callback semantics belong with the partner's callback assertions. |
| TestChecksValidateEmptyList | internal/checks | checks_test.go:306 | 0.04 | MERGE | TestChecksValidateTable | An empty list is valid: one row in the validate table. |
| TestChecksValidateOptionalCriterion | internal/checks | checks_test.go:312 | 0.06 | KEEP |  | OptionalCriterion and callback semantics with the exact error message. |
| TestCheckTurnYAMLDecodes | internal/checks | checks_turn_test.go:22 | 0.05 | MERGE | TestChecksYAMLDecodesChecksList | Another YAML struct-tag decode; add turn to the partner's sample. |
| TestCheckTurnValidateAllowedTypes | internal/checks | checks_turn_test.go:32 | 0.04 | MERGE | TestCheckTurnValidateRejectedTypes | The positive half of the same turn allow-list table. |
| TestCheckTurnValidateRejectedTypes | internal/checks | checks_turn_test.go:43 | 0.09 | KEEP |  | Rejects turn: on every type that can't take it, through YAML decode; the partner table doesn't cover these. |
| TestCheckTurnValidateRejectsBelowOne | internal/checks | checks_turn_test.go:66 | 0.04 | MERGE | TestDocumentCheckValidate | Same turn >= 1 rule the partner's "turn below one" subtest checks; one table over check types. |
| TestCheckTurnEvalPerTurnGHLog | internal/checks | checks_turn_test.go:94 | 0.07 | KEEP |  | Per-turn scoping, including wrong-turn failure; anchor for the per-turn table. |
| TestCheckTurnEvalPerTurnOutput | internal/checks | checks_turn_test.go:109 | 0.07 | MERGE | TestCheckTurnEvalPerTurnGHLog | Same per-turn scoping over output instead of gh log; one table over evidence kind. |
| TestCheckTurnEvalDefaultsToLastTurn | internal/checks | checks_turn_test.go:117 | 0.08 | KEEP |  | No-turn checks read the last turn, not the top-level fields; distinct inputs prove it. |
| TestCheckTurnEvalEmptyTurnsUsesTopLevel | internal/checks | checks_turn_test.go:133 | 0.09 | KEEP |  | Legacy single-turn fallback when no per-turn evidence exists. |
| TestCheckTurnEvalOutOfRangeFailsWithDetail | internal/checks | checks_turn_test.go:141 | 0.05 | KEEP |  | Unit-level out-of-range across all four types with the turn count in the detail; the cmd-level partner covers only single-turn input. |
| TestCheckTurnLabel | internal/checks | checks_turn_test.go:169 | 0.12 | MERGE | TestJudgeCheckLabel | Label formatting with/without turn is another row of the label table. |
| TestNoHarnessDeps | internal/checks | deps_test.go:12 | 0.17 | KEEP |  | Architectural boundary (checks must not link eval); coverage can't see it. |
| TestHermeticGitEnv_StripsRepoSelectingVars | internal/checks | env_test.go:20 | 0.06 | KEEP |  | Security/hermeticity: repo-selecting GIT_* vars are stripped and config is disabled. |
| TestInferenceJudgeRequest | internal/checks | judge_infer_test.go:37 | 0.04 | KEEP |  | Pins the judge request (role, YesNo, 2m timeout, model, stub, prompt) sent to the backend. |
| TestInferenceJudgeUnpinnedNoStub | internal/checks | judge_infer_test.go:59 | 0.04 | MERGE | TestInferenceJudgeRequest | Same request-shape assertion with no model/stub; one table row. |
| TestInferenceJudgeOnCall | internal/checks | judge_infer_test.go:69 | 0.04 | KEEP |  | OnCall reporting of success, backend error and parse error, which feeds the cost/call records. |
| TestInferenceJudgeDebug | internal/checks | judge_infer_test.go:108 | 0.05 | KEEP |  | Per-call debug numbering lives here; the eval-package partner doesn't check it. |
| TestBuildYesNoPromptTranscriptOrderAndFences | internal/checks | judge_prompt_test.go:75 | 0.04 | KEEP |  | Pins the judge prompt structure (preamble, ordered turn fences, judged turn, file fences) that the LLM judge sees. |
| TestBuildYesNoPromptTranscriptOmitsSingleTurnBlocks | internal/checks | judge_prompt_test.go:107 | 0.05 | MERGE | TestBuildYesNoPromptTranscriptOrderAndFences | Same twoTurnQuery prompt; its negative assertions belong in the partner. |
| TestBuildYesNoPromptTranscriptTurnOneExcludesTurnTwo | internal/checks | judge_prompt_test.go:116 | 0.05 | KEEP |  | Prompt for an earlier turn must not leak later turns (different input from the partner). |
| TestBuildYesNoPromptTranscriptMarkerCount | internal/checks | judge_prompt_test.go:136 | 0.04 | MERGE | TestBuildYesNoPromptTranscriptOrderAndFences | Same input; the marker counts are two more assertions for the partner. |
| TestLoadFile_InvalidFixtureLoadsButFailsValidate | internal/checks | load_test.go:53 | 0.06 | KEEP |  | The load/validate split with ErrInvalid and the check index in the message. |
| TestLoadFile_EvidenceNeedsHiddenDirForInject | internal/checks | load_test.go:71 | 0.04 | DELETE | TestCheckEvidenceFlags | The same fixture and rule is asserted more strictly by the cmd-level TestCheckEvidenceFlags subtest. |
| TestLoadFile_FollowsSymlinkToRegularFile | internal/checks | load_test.go:158 | 0.06 | KEEP |  | Regression guard that symlinked check files load (the cmd-level partner tests something else). |
| TestFixtures_NoAbsolutePaths | internal/checks | load_test.go:190 | 0.04 | DELETE |  | Hygiene lint over test fixtures; no product behaviour, and a bad path would already break the fixture's consumers. |
| TestFixtures_Content | internal/checks | load_test.go:211 | 0.04 | DELETE |  | Asserts the bytes of the test's own fixture files; consumers already fail if the fixtures change. |
| TestFindTabByAgentID | internal/tui | agent_restoration_test.go:73 | 0.07 | KEEP |  | Direct unit of TabManager lookup, including the -1 miss case. |
| TestStatusRunningAgentsDeterministicOrder | internal/tui | agent_restoration_test.go:175 | 0.07 | DELETE |  | Asserts nothing: the only check is an empty `if` body, with no agents registered. |
| TestLayoutStabilityAcrossAllTabs | internal/tui | autocomplete_overlay_validation_test.go:224 | 0.1 | MERGE | TestFooterDropdownRendersExactly3LinesWithoutDropdown | Same footer line-count assertion; add the agent tab and autocomplete-active rows to the partner. |
| TestClassicPlanningCmdArgvCarriesPrompt | internal/tui | classic_launch_test.go:150 | 0.06 | MERGE | TestClassicNewSessionPromptHasDocumentPath | Partner already checks the args contain the path and request; add the prompt-is-last-arg assertion there. |
| TestClassicExecPlanningSeamDefaultIsSet | internal/tui | classic_launch_test.go:322 | 0.06 | KEEP |  | Fixtures override the seam, so only this test proves production doesn't nil-panic on classic planning. |
| TestAboutDialog | internal/tui | commands_test.go:32 | 0.06 | KEEP |  | The about overlay content (version line plus appended status) is what users see. |
| TestEnterKeyEmptyInput | internal/tui | enter_key_test.go:112 | 0.09 | MERGE | TestEnterKeyWhitespaceOnly | Empty input is the "" row of the whitespace table, and alone it only asserts its own setup. |
| TestEnterKeyAgentTabCommandExecution | internal/tui | enter_key_test.go:157 | 0.06 | DELETE | TestEnterKeyCommandExecutionInAllTabs | Identical to the partner's "agent tab footer focused" row. |
| TestEnterKeyPlanningTabForwarding | internal/tui | enter_key_test.go:193 | 0.04 | DELETE | TestEnterKeyCommandExecutionInAllTabs | Weaker duplicate of the partner's "planning tab content focused" row. |
| TestFooterRendersExactly3Lines | internal/tui | footer_test.go:11 | 0.05 | DELETE | TestFooterDropdownRendersExactly3LinesWithoutDropdown | Byte-for-byte identical body to the partner. |
| TestMultipleAgentsOutputCapture | internal/tui | integration_test.go:17 | 0.1 | DELETE | TestCaptureWriterConcurrentWrites | Concurrency of OutputCapture belongs in internal/agent (already tested there); this one sleeps 50ms and accepts 3 of 5 agents. |
| TestANSIStripping | internal/tui | integration_test.go:59 | 0.05 | KEEP |  | Only test of OutputCapture ANSI stripping (internal/agent has none); it belongs in internal/agent, and its `if i < len(lines)` guard should become a length assert. |
| TestViewManagerTransitions | internal/tui | integration_test.go:87 | 0.05 | DELETE | TestViewManager | Line-for-line duplicate of TestViewManager, and ViewManager has no production references (dead code). |
| TestHighVolumeOutput | internal/tui | integration_test.go:114 | 0.05 | DELETE | TestOutputCaptureRingBuffer | Ring-buffer eviction is already tested exactly in internal/agent, and this version's assertions are conditional. |
| TestOutputAlwaysAccumulates | internal/tui | integration_test.go:152 | 0.05 | DELETE | TestOutputCaptureAlwaysAccumulates | Tests agent.OutputCapture from the tui package, duplicating internal/agent's TestOutputCaptureAlwaysAccumulates. |
| TestSwitchToPlanningModePreservesConsoleState | internal/tui | mode_switching_test.go:47 | 0.05 | MERGE | TestConsoleStateRestorationAfterModeSwitch | Same console-state capture checked halfway through the round trip that the partner completes. |
| TestConsoleStateRestorationAfterModeSwitch | internal/tui | mode_switching_test.go:361 | 0.05 | KEEP |  | The only test asserting m.input.Value() is actually restored after the round trip. |
| TestPlanEmptyFocusesMessageInput | internal/tui | plan_focus_test.go:152 | 0.06 | KEEP |  | User-visible: bare `plan` lands focus in an empty message input, not the footer. |
| TestPlanDescriptionCursorAtEnd | internal/tui | plan_focus_test.go:213 | 0.05 | KEEP |  | User-visible: `plan <desc>` stages the text in the input (not history) with the cursor at the end. |
| TestInteractiveLaunchSitesUsePlannerAgentInteractive | internal/tui | planner_agent_test.go:12 | 0.05 | DELETE |  | Greps source files for a string; a convention lint, not behaviour (assert the launched argv's --agent instead if it matters). |
| TestRoutingStreamMessagesReachPlanningTab | internal/tui | planning_acp_handoff_test.go:180 | 0.05 | KEEP |  | Regression for a real routing bug where stream msgs never reached the tab. |
| TestACPFirstMessageAnnouncesDocumentPath | internal/tui | planning_acp_handoff_test.go:314 | 0.42 | REWRITE-CHEAPER |  | Core path-announcement contract, but it pays the fixed 60ms-per-hop pump wait; fix collectWithin. |
| TestACPMessageAfterReconnectAnnouncesAgain | internal/tui | planning_acp_handoff_test.go:365 | 0.78 | REWRITE-CHEAPER |  | Unique reconnect re-announce assertion; 0.78s is all fixed pump sleeps from two sends. |
| TestACPRestoredTabAddsResumedSentence | internal/tui | planning_acp_handoff_test.go:384 | 0.42 | REWRITE-CHEAPER |  | Valuable, but each pump hop in the acpFixture waits a fixed 60ms (collectWithin); return when the spawned cmds finish instead (WaitGroup) to drop ~0.4s. |
| TestACPSendFailureAnnouncesPathAgain | internal/tui | planning_acp_handoff_test.go:403 | 0.05 | KEEP |  | Distinct failure path (send error response) that must re-announce the document path to the agent. |
| TestHandoffCommittedProcessesOnceAndDeletesFile | internal/tui | planning_handoff_test.go:94 | 0.05 | KEEP |  | Off-UI-goroutine boundary call, request shape, attempt, deletion and console link; much stricter than the exec partner. |
| TestHandoffNoFileDoesNothing | internal/tui | planning_handoff_test.go:141 | 0.05 | KEEP |  | Unit contract of finishPlanning with no document (no cmd, no console noise); the partner only covers the exec wiring. |
| TestHandoffProcessErrorKeepsFile | internal/tui | planning_handoff_test.go:161 | 0.05 | KEEP |  | Boundary error keeps the file, shows the cause and clears the in-flight marker; the partner covers a different error. |
| TestHandoffRejectedWithoutFailureStillOpensDialog | internal/tui | planning_handoff_test.go:212 | 0.06 | KEEP |  | Edge: a rejected outcome with no failure context still opens the dialog and keeps the file. |
| TestPlanningDocumentPath | internal/tui | planning_handoff_test.go:572 | 0.05 | KEEP |  | Path-traversal rejection for session IDs (security) plus the path layout. |
| TestPlanningPromptNewSession | internal/tui | planning_handoff_test.go:607 | 0.05 | KEEP |  | The prompt layout is the contract with the planning agent; anchor for the prompt table. |
| TestPlanningPromptResumedSessionAddsSentence | internal/tui | planning_handoff_test.go:621 | 0.05 | MERGE | TestPlanningPromptNewSession | Same planningPrompt function with resumed=true; one prompt table. |
| TestPlanningPromptWithoutRequestIsJustThePath | internal/tui | planning_handoff_test.go:635 | 0.05 | MERGE | TestPlanningPromptNewSession | Empty-request row of the planningPrompt table. |
| TestPlanningResumePromptCarriesFailureAndPath | internal/tui | planning_handoff_test.go:648 | 0.05 | KEEP |  | Unit contract of the resume prompt content, separate from the ACP delivery test. |
| TestHandoffLabelYesAppliesConfiguredLabel | internal/tui | planning_label_test.go:107 | 0.06 | KEEP |  | The label is applied off the UI goroutine and both files are deleted on commit. |
| TestHandoffLabelUsesConfiguredLabelName | internal/tui | planning_label_test.go:136 | 0.05 | KEEP |  | The configured label name is applied and named in the console. |
| TestHandoffLabelYesIsCaseAndWhitespaceTolerant | internal/tui | planning_label_test.go:195 | 0.06 | KEEP |  | Normalising the user's label answer is user-facing behaviour. |
| TestHandoffLabelAnswerRewrittenDuringSendIsKept | internal/tui | planning_label_test.go:391 | 0.06 | KEEP |  | mtime-based edge: a rewritten answer for the next document survives. |
| TestHandoffLabelAnswerCreatedDuringSendIsKept | internal/tui | planning_label_test.go:420 | 0.06 | KEEP |  | Race edge: a label answer written during the send is neither applied nor deleted. |
| TestRejectionDialogShowsGuardExpectedActual | internal/tui | planning_rejection_test.go:49 | 0.05 | KEEP |  | Dialog content (guard, expected, actual, path, actions) is user-visible. |
| TestRejectionDialogDismissKeepsFile | internal/tui | planning_rejection_test.go:77 | 0.06 | KEEP |  | Both dismiss keys clear state, skip Resume and keep the file; more than the classic partner checks. |
| TestRejectionDialogResumeCallsFrontEnd | internal/tui | planning_rejection_test.go:108 | 0.05 | KEEP |  | Both resume keys call Resume once with the failure and propagate its command. |
| TestRejectionDialogWritesFullFailureToConsole | internal/tui | planning_rejection_test.go:218 | 0.05 | KEEP |  | The long failure is written in full to the console (not truncated like the dialog). |
| TestTabDataAccumulatesContinuouslyDuringPlannerSessions | internal/tui | tab_session_persistence_test.go:15 | 0.05 | DELETE | TestAgentDataIntegrityAfterMultiplePlannerCycles | Same capture/switch/verify scenario as the partner (which checks every line per agent across 5 cycles), so it adds no assertion. |
| TestAgentTabsRetainFullLogHistoryWhenSwitchingBack | internal/tui | tab_session_persistence_test.go:118 | 0.05 | DELETE | TestAgentDataIntegrityAfterMultiplePlannerCycles | Copy-paste variant of the partner's per-agent all-lines-retained check. |
| TestMainTUIWatcherLogsContinueAccumulating | internal/tui | tab_session_persistence_test.go:222 | 0.1 | DELETE | TestConsoleStateRestorationAfterModeSwitch | Activity preserved across a mode round trip is already asserted (more strictly, in order) by the partner and TestRapidModeSwitchingNoDataLoss. |
| TestMultipleRapidModeSwitchesWithContinuousData | internal/tui | tab_session_persistence_test.go:375 | 0.05 | DELETE | TestAgentDataIntegrityAfterMultiplePlannerCycles | Weaker copy of the partner (it only checks that each cycle has some data). |
| TestSessionDataPersistenceDuringMemoryPressure | internal/tui | tab_session_persistence_test.go:558 | 0.05 | DELETE | TestHistoryLimitsRespectedDuringPersistence | Ring-buffer limits and recency across a mode switch are what the partner asserts; the "memory pressure" is 800 lines. |
| TestConcurrentAgentDataDuringPlannerSessionToggle | internal/tui | tab_session_persistence_test.go:631 | 0.05 | DELETE | TestAgentDataIntegrityAfterMultiplePlannerCycles | Not concurrent at all: a sequential copy of the partner with more iterations. |
| TestTabInterfaceDefinition | internal/tui | tabs_interface_test.go:23 | 0.05 | DELETE |  | Asserts the hard-coded return values of a test-only mockTab. |

### 2. Counts

| verdict | count | total secs |
|---|---|---|
| KEEP | 124 | 8.82 |
| MERGE | 40 | 2.18 |
| REWRITE-CHEAPER | 6 | 3.51 |
| DELETE | 37 | 1.94 |
| total | 207 | 16.45 |

### 3. Recurring patterns

1. **TUI tests for logic that lives in another package.** `internal/tui/integration_test.go` and `tab_session_persistence_test.go` test `agent.OutputCapture` and the manager's ring buffer through the TUI. Several near-identical "capture lines, toggle planning mode, check the lines are still there" scenarios differ only in counts. `internal/agent` already tests the buffer directly, and `TestAgentDataIntegrityAfterMultiplePlannerCycles` / `TestHistoryLimitsRespectedDuringPersistence` cover the mode-switch case. Examples: TestConcurrentAgentDataDuringPlannerSessionToggle (not actually concurrent), TestMultipleRapidModeSwitchesWithContinuousData, TestHighVolumeOutput.
2. **Exact or row-level copy-paste duplicates.** Some candidates have the same body as another test, or repeat one row of an existing table test. Examples: TestFooterRendersExactly3Lines (same bytes as TestFooterDropdownRendersExactly3LinesWithoutDropdown), TestViewManagerTransitions (same as TestViewManager, for a type with no production use), TestEnterKeyAgentTabCommandExecution (one row of TestEnterKeyCommandExecutionInAllTabs).
3. **Tests that assert constants, test doubles, or nothing.** Examples: TestStatusRunningAgentsDeterministicOrder (its only check is an empty `if`), TestTabInterfaceDefinition (checks mockTab's hard-coded returns), TestChecksCheckTypeSet / TestStub_AvailabilityAndProbe (compare constants with themselves or with `return nil`).
4. **One-input-per-function tests that belong in one table.** Many small tests call the same function with a different input. They are cheap (about 0.04s each), so merging them saves little runtime, but each one is a separate copy to maintain. Examples: the six TestPriceTable_Cost_* tests, the CallRecord omit-empty JSON tests (Purpose/Turn/CostUnknown/StepSource), and the planningPrompt variants (NewSession/Resumed/WithoutRequest).
5. **Fixed sleeps or timeouts set much higher than needed.** These are the main runtime cost in this set. Examples: the ACP planning tests (the fixture's `collectWithin` always waits 60ms per message hop, 0.4-0.8s per test), TestStub_CommandTimeoutKillsProcessGroupPromptly (a 1s timeout), TestEvaluateChecks_CommandTimeoutKillsProcessGroup (a 400ms timeout). The assertions are worth keeping; lowering the waits keeps them and cuts the time.

**Coverage-blind tests that are real contracts (do not delete on the zero-coverage flag alone).** Struct-tag/YAML/JSON key tests for persisted or author-written formats (TestCallRecord_JSONFieldNames, TestChecksYAMLDecodesChecksList, TestStubScript_RoutesYAML) and the import-boundary tests (TestNoHarnessDeps, TestInferenceDoesNotImportEval) are KEEP. Zero-coverage tests that only exercise encoding/json or fixtures (TestCallRecord_RoundTrip, TestCallRecord_LegacyJSONDecodesAsKnown, TestFixtures_Content) are DELETE.

## Group: planner-plan-failure-prov-config

### Test audit verdicts: internal/planner, plan, failure, provenance, config

Every candidate was read in full, next to its flagged partner. Runtimes come from the measurement JSON (each test run alone). About 0.7-0.9s of a package's *first* test is binary startup, not test cost: TestLoad_EnableCopilotReviewDefaults, TestAutomatedManifest, TestCanonicalize_Removes..., TestDisallowedDirectImport (failure) and TestCompatibility_LegacySpecWithoutPlan. So most of the gain here is less maintenance and less brittleness, not wall-clock time.

### 1. Verdicts

| test | pkg | file:line | secs | verdict | partner | reason |
|---|---|---|---|---|---|---|
| TestAutomatedManifest | planner | automated_agent_test.go:45 | 0.80 | KEEP | - | Pins the enforceable agent config: name == AgentAutomated (what KiroArgs launches), prompt path, exact tools (read, shell) and resources. Its 0.8s is first-test startup. |
| TestAutomatedPromptRequiresPunchOut | planner | automated_agent_test.go:143 | 0.04 | KEEP | - | The automated half of the variant-permission contract (the skill defers to the prompt for which result variant is allowed). Consider loosening its phrase regexes. |
| TestBoundaryExplicitTitleWins | planner | boundary_test.go:319 | 0.05 | KEEP | - | The only test of the branch where the request title overrides the heading-derived title. |
| TestBoundaryPunchOutCommentsThenLabelsAndWritesNoBody | planner | boundary_test.go:390 | 0.04 | KEEP | - | Core boundary punch-out path: comment, then label, no store write, unresolved slots in the comment, intake 1-of-1, unrecoverable. |
| TestBoundaryPunchOutWithoutTargetIssueIsContractError | planner | boundary_test.go:449 | 0.04 | KEEP | - | A distinct error path: a punch_out with no target issue is ErrNoPunchOutTarget/ErrInvalid and writes nothing. |
| TestBoundaryProcessHasNoModeParameterAndNoGhOrHash | planner | boundary_test.go:732 | 0.06 | KEEP | - | The import/call ban (no os/exec, crypto, internal/github, provenance.Mark/Hash in boundary.go) enforces that only the Committer writes and hashes. Consider dropping the "mode"/"agent" name heuristics. |
| TestEvalDriverDryRun | planner | evaldriver_test.go:54 | 0.10 | KEEP | - | Real script behaviour: dry run prints exactly the two sandboxed eval commands with filters that must match the case names, and touches nothing. |
| TestEvalDriverOverlayAndResults | planner | evaldriver_test.go:202 | 0.42 | KEEP | - | Exercises scripts/eval-planner.sh with a fake kairon: the overlay manifest is planner-automated with only the name changed, no skills, and results are copied back. Real script behaviour. |
| TestPlannerDoesNotImportEval | planner | extract_test.go:188 | 0.06 | KEEP | - | Dependency boundary that the planned eval-harness split relies on; cheap go/parser check. |
| TestInteractiveManifest | planner | interactive_agent_test.go:32 | 0.06 | KEEP | - | Pins the enforceable agent config: name, prompt path, exact tools, resources and the write allowedPaths that confine the agent to its worktree and session files. |
| TestInteractivePromptWritesDocumentToGivenPath | planner | interactive_agent_test.go:135 | 0.04 | KEEP | - | The prompt must reference the exact "Document path:" line that tui/planning_handoff.go:225 sends; host for the path-token contract (trim the remaining wording regexes). |
| TestInteractivePromptNeverPunchesOut | planner | interactive_agent_test.go:323 | 0.04 | KEEP | - | The interactive half of the variant-permission contract: the shared skill defers to the prompt for which result variant is allowed, and punch_out must appear only in prohibitions. |
| TestInteractivePromptDoesNotPublishOrHash | planner | interactive_agent_test.go:342 | 0.04 | KEEP | - | Regression guard against the pre-#397 flow (gh issue create, --body-file, hash writing) coming back; the interactive agent has shell, so the prompt is the barrier that bypassing provenance would cross. |
| TestKiroArgsIsAutomatedAndNeverTrustsAllTools | planner | run_test.go:17 | 0.04 | KEEP | - | Security contract: the unattended argv names planner-automated and never includes a trust flag; the only place the exact argv is pinned. |
| TestAutomatedPromptNamesRepoIssueAndAttempt | planner | run_test.go:48 | 0.05 | KEEP | - | Code-generated prompt content: issue ref, gh issue view command, attempt N of M; host for the AutomatedPrompt table. |
| TestInvokeFailureNamesTheInvokeGuard | planner | run_test.go:87 | 0.04 | KEEP | - | The failure built when kiro-cli cannot run must name the invoke guard, carry the error and pass FailureContext.Validate, or the punch-out path fails. |
| TestStripANSI | planner | run_test.go:101 | 0.07 | KEEP | - | Real parsing of kiro-cli colored output (CSI and OSC sequences) before Extract. |
| TestSkillFrontMatter | planner | skill_test.go:37 | 0.06 | KEEP | - | Kiro loads skills by front-matter name and description; a broken front matter silently drops the skill. |
| TestSkillExamplesParseOnePerVariant | planner | skill_test.go:94 | 0.04 | KEEP | - | Executable skill examples: exactly one canonical and one punch_out YAML example must parse with the real stageoutput parser. |
| TestSkillCanonicalExampleFollowsOwnStandard | planner | skill_test.go:116 | 0.04 | KEEP | - | The skill's canonical example models what agents copy, so it must follow the body standard and the single-line Dependencies rule. Could run defaultGuards instead of substring checks. |
| TestSkillDependenciesRule | planner | skill_test.go:177 | 0.08 | KEEP | - | The skill must teach the exact formats code parses: "Dependencies: #12, #15", #{key} with the SubstituteDependencies key grammar, and no ## Dependencies heading (rejected by DependenciesForm). |
| TestTemplatesContainNoPlannerConventionsPath | planner | skill_test.go:304 | 0.05 | KEEP | - | planner-conventions is project-owned; shipping it in templates would let `kairon update` overwrite local guidance. sync:check does not catch this. |
| TestPlanningWorktreeScripts_ParallelRunsGetDistinctWorktrees | planner | worktree_scripts_test.go:14 | 0.88 | KEEP | - | A real regression test for the shell scripts (same-second worktree name collision under parallel intake) against a real git repo. Could add t.Parallel() to hide its 0.88s. |
| TestAutomatedManifestHasNoInjectedOrWriteCapabilities | planner | automated_agent_test.go:76 | 0.09 | MERGE | TestAutomatedManifest | Reads the same manifest; the write/web part is already implied by the exact tools list, so add only the creds-agent/mcpServers/--trust-all-tools absence check to TestAutomatedManifest. |
| TestAutomatedManifestShellIsReadOnly | planner | automated_agent_test.go:85 | 0.04 | MERGE | TestAutomatedManifest | A security setting (autoAllowReadonly) on the same parsed manifest; one more assertion in TestAutomatedManifest. |
| TestBoundaryReferencePunchOutReplyPunchesOut | planner | boundary_test.go:427 | 0.05 | MERGE | TestBoundaryReferenceCanonicalRepliesCommit | The same "reference eval reply through the real boundary" loop with a different expected outcome; one (case, want outcome) table. |
| TestDependenciesFormAcceptsBulletsOutsideTheDependenciesSection | planner | boundary_test.go:855 | 0.07 | MERGE | TestDependenciesFormAccepts | The same "admitBody(body).Pass" assertion; one more accepted-body row (TestDependenciesFormIgnoresFencedExamples belongs there too). |
| TestProcessChainResolvesDependenciesIntoSingleLineForm | planner | chain_test.go:31 | 0.04 | MERGE | TestProcessChainCommittedBodiesHaveNoBulletListAndValidHash | Identical setup (the same 3-item chain); fold both sets of assertions into one test. |
| TestInteractivePromptRecordsLabelAnswer | planner | interactive_agent_test.go:196 | 0.05 | MERGE | TestInteractivePromptWritesDocumentToGivenPath | Its real contract is the "Label answer path:" line (planning_handoff.go:226) and the yes/no answer (parsed at planning_handoff.go:137); keep those tokens in one test and drop the ~12 wording regexes. |
| TestInteractiveManifestHasNoInjectedCapabilities | planner | interactive_agent_test.go:259 | 0.05 | MERGE | TestInteractiveManifest | Reads the same manifest; add the creds-agent/mcpServers absence check to TestInteractiveManifest. |
| TestAutomatedPromptCarriesThePreviousFailure | planner | run_test.go:61 | 0.04 | MERGE | TestAutomatedPromptNamesRepoIssueAndAttempt | The same AutomatedPrompt -> contains/not-contains assertion with prev set; one (prev, want, notWant) table. |
| TestEvalDriverExitsNonZeroWhenEitherHalfFails | planner | evaldriver_test.go:253 | 0.91 | REWRITE-CHEAPER | - | 0.91s = three full driver runs run one after another; drop "both fail" (exit aggregation is already shown by each single failure) and mark the subtests t.Parallel(). |
| TestAutomatedPromptHasRTCCHeadings | planner | automated_agent_test.go:94 | 0.05 | DELETE | TestInteractivePromptHasRTCCHeadings | A prompt-style convention (Role/Task/Context/Constraints order) that no code or agent depends on. pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestAutomatedPromptNeverAsksOffersOrDrafts | planner | automated_agent_test.go:110 | 0.04 | DELETE | - | Four phrase regexes ("never writes a draft for approval"); pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestAutomatedPromptRequiresInferenceEvidence | planner | automated_agent_test.go:125 | 0.04 | DELETE | - | Phrase regexes ('coin-flip', 'competent engineer reading the same issue and repository'); pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestAutomatedPromptHasNoInteractiveOrModeText | planner | automated_agent_test.go:160 | 0.04 | DELETE | - | A 15-pattern forbidden-word lint (confirm, Other, recommend, mode); pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestAutomatedPromptDoesNotRestateSharedContract | planner | automated_agent_test.go:187 | 0.05 | DELETE | TestAutomatedManifest | A DRY lint over prompt terms; the skill is actually loaded through the manifest resources that TestAutomatedManifest pins. pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestAutomatedFilesMatchTemplates | planner | automated_agent_test.go:203 | 0.04 | DELETE | - | Duplicates `task sync:check` (Taskfile diff -rq, run in .github/workflows/ci.yml:33), which already byte-compares these live/template files. |
| TestPlannerManifestsListSameSharedSkill | planner | automated_agent_test.go:216 | 0.05 | DELETE | TestAutomatedManifest | TestAutomatedManifest and TestInteractiveManifest already DeepEqual resources to [conventions, shared]. |
| TestDependenciesFormRunsInTheChainAfterStructural | planner | boundary_test.go:966 | 0.04 | DELETE | TestBoundaryUnparseableDocumentIsRejectedByStructuralGuard | Asserts the slice layout of defaultGuards; the order is proven behaviourally (unparseable input is rejected by the structural guard, bad Dependencies by DependenciesForm, through Process). |
| TestSameContractReferenceRepliesParseToPlannerDocuments | planner | boundary_test.go:982 | 0.06 | DELETE | TestExtractReferenceDocuments | Extract+Parse over the reference replies duplicates TestExtractReferenceDocuments, outcomes are implied by the TestBoundaryReference* tests, and the reflect.TypeOf equality holds by Parse's signature. |
| TestSameContractBoundaryResultIsOneTypeForBothAgents | planner | boundary_test.go:1036 | 0.04 | DELETE | TestBoundaryReferenceCanonicalRepliesCommit | reflect.TypeOf of the same function's return value is always equal; the "both commit" outcome is covered by ReferenceCanonicalRepliesCommit. |
| TestExtractErrorsAreDistinct | planner | extract_test.go:108 | 0.05 | DELETE | - | Two errors.New sentinels are distinct by construction; asserts language behaviour. |
| TestInteractiveManifestAllowsSessionDocument | planner | interactive_agent_test.go:95 | 0.03 | DELETE | TestInteractiveManifest | TestInteractiveManifest already DeepEquals allowedPaths to the same three paths. |
| TestInteractiveTemplateManifestAllowedPathsMatchLive | planner | interactive_agent_test.go:104 | 0.03 | DELETE | TestInteractiveFilesMatchTemplates | Implied by byte-equality of the manifests; duplicates `task sync:check` (Taskfile diff -rq, run in .github/workflows/ci.yml:33), which already byte-compares these live/template files. |
| TestInteractiveTemplatePromptMatchesLive | planner | interactive_agent_test.go:112 | 0.05 | DELETE | TestInteractiveFilesMatchTemplates | A strict subset of TestInteractiveFilesMatchTemplates (same prompt pair, same bytes.Equal); duplicates `task sync:check` (Taskfile diff -rq, run in .github/workflows/ci.yml:33), which already byte-compares these live/template files. |
| TestInteractivePromptStatesSingleWriteException | planner | interactive_agent_test.go:174 | 0.04 | DELETE | TestInteractiveManifest | Pins the wording of one constraint line; the real write boundary is the manifest allowedPaths in TestInteractiveManifest. |
| TestInteractivePromptHasRTCCHeadings | planner | interactive_agent_test.go:268 | 0.04 | DELETE | - | A prompt-style convention (heading order) nothing depends on. pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestInteractivePromptEncodesElicitationFlow | planner | interactive_agent_test.go:285 | 0.04 | DELETE | - | 27 phrase regexes over the elicitation instructions; pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestInteractivePromptDoesNotRestateSharedContractOrSelectMode | planner | interactive_agent_test.go:369 | 0.05 | DELETE | - | A DRY and forbidden-word lint; pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestInteractiveFilesMatchTemplates | planner | interactive_agent_test.go:399 | 0.05 | DELETE | - | Duplicates `task sync:check` (Taskfile diff -rq, run in .github/workflows/ci.yml:33), which already byte-compares these live/template files. |
| TestNoModeRoutingInPlannerPackage | planner | run_test.go:135 | 0.04 | DELETE | - | A one-off proof of #397 AC5 using name-substring heuristics ("mode" also matches "model"); brittle, and TestKiroArgs/AutomatedCommand pin the actual agent selection. |
| TestAutomatedSiteNamesOnlyTheAutomatedAgent | planner | run_test.go:204 | 0.06 | DELETE | TestAutomatedCommandRunsPlannerAutomatedInDir | Greps run.go source for identifiers; the behaviour (argv names planner-automated) is asserted directly by AutomatedCommand/KiroArgs tests. |
| TestSkillConstants | planner | skill_test.go:24 | 0.04 | DELETE | TestAutomatedManifest | Asserts constants; their values are already cross-checked against the manifests (name == AgentAutomated/AgentInteractive) and the KiroArgs argv. |
| TestSkillSectionsInOrder | planner | skill_test.go:52 | 0.05 | DELETE | - | Pins heading names and order; no prompt or code references these sections. pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestSkillContractFields | planner | skill_test.go:75 | 0.07 | DELETE | TestSkillExamplesParseOnePerVariant | Substring checks for schema field names; ExamplesParseOnePerVariant proves the contract by parsing the examples with stageoutput.Parse. |
| TestSkillInvestigateClassifyCover | planner | skill_test.go:146 | 0.04 | DELETE | - | A 24-term word list; pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestSkillIssueBodyStandard | planner | skill_test.go:164 | 0.04 | DELETE | - | A word list ("70 characters", "6 In Scope", "8 acceptance criteria"); no code enforces these limits. pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestSkillRestrictions | planner | skill_test.go:201 | 0.05 | DELETE | TestAutomatedManifest | A 25-term word list; the enforceable restrictions are the manifest tools/allowedTools/shell settings. pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestSkillHardBars | planner | skill_test.go:220 | 0.05 | DELETE | - | Pins the vague-word list ("gracefully", "robust"); no guard enforces those words in code. pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestSkillHasNoModeSpecificText | planner | skill_test.go:235 | 0.06 | DELETE | - | A forbidden-word lint (question, Other, draft, confirm, recommend, mode); pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestSkillPointsToPromptForVariantPermission | planner | skill_test.go:265 | 0.04 | DELETE | - | A single-phrase regex ('your prompt'); pins incidental prompt/skill wording that no code parses; any rewording breaks it with no behaviour change (agent behaviour is what the planner eval suite measures). |
| TestTemplateSkillIsByteIdentical | planner | skill_test.go:272 | 0.05 | DELETE | TestSyncGate_SharedSkillTemplateInSync | Byte-identical to TestSyncGate_SharedSkillTemplateInSync, and both duplicates `task sync:check` (Taskfile diff -rq, run in .github/workflows/ci.yml:33), which already byte-compares these live/template files. |
| TestTemplateSkillDirHasOnlySkillFile | planner | skill_test.go:280 | 0.07 | DELETE | - | Pins directory layout; adding a legitimate second skill file would break it for no behavioural reason, and sync:check diffs the whole directory. |
| TestSyncGate_TaskfileDiffsSharedSkill | planner | syncgate_test.go:34 | 0.05 | DELETE | - | Pins an exact command string in Taskfile.yml; removing that line is a reviewed edit, and sync itself is enforced by sync:check. |
| TestSyncGate_TaskfileExcludesPlannerConventions | planner | syncgate_test.go:40 | 0.05 | DELETE | TestTemplatesContainNoPlannerConventionsPath | Pins the absence of a word in Taskfile.yml; the real property (not in templates) is checked by the templates walk. |
| TestSyncGate_TemplatesHaveNoPlannerConventionsSkill | planner | syncgate_test.go:46 | 0.05 | DELETE | TestTemplatesContainNoPlannerConventionsPath | The same templates walk with the same assertion. |
| TestSyncGate_BuilderConventionsListsSharedSkill | planner | syncgate_test.go:62 | 0.04 | DELETE | - | Pins a table row and command string in builder-conventions/SKILL.md; drift itself is caught by sync:check. |
| TestSyncGate_SharedSkillTemplateInSync | planner | syncgate_test.go:74 | 0.06 | DELETE | - | Duplicates `task sync:check` (Taskfile diff -rq, run in .github/workflows/ci.yml:33), which already byte-compares these live/template files. |
| TestSyncGate_DriftDetected | planner | syncgate_test.go:85 | 0.05 | DELETE | - | Proves that Go string == detects an appended byte, using a helper in the test file; asserts language behaviour and the test's own setup. |
| TestCompatibility_TopologicalSortStability | plan | compatibility_test.go:488 | 0.05 | KEEP | - | Deterministic, ID-sorted layer order is what `kairon plan parse` emits; host for the TopologicalSort table. |
| TestParsePlanFromMarkdown_ValidPlan | plan | parser_test.go:9 | 0.04 | KEEP | - | Happy-path parse with prose before and after; host for the parse-success table. |
| TestParsePlanFromMarkdown_NoPlan | plan | parser_test.go:53 | 0.04 | KEEP | - | (nil, nil) is the contract cmd/kairon/cmd/plan.go maps to {"status":"no_plan"} for legacy specs. |
| TestParsePlanFromMarkdown_MultiplePlans | plan | parser_test.go:77 | 0.04 | KEEP | - | Multiple blocks -> error is what cmd plan parse turns into validation_failed; host for the parse-error table. |
| TestDiscoverAgents_MissingLeadConfig | plan | registry_test.go:72 | 0.04 | KEEP | - | A missing krew-lead.json must be a hard error; host for a DiscoverAgents error table with InvalidJSON and EmptyTrustedList. |
| TestValidatorWithMissingDependency | plan | validator_test.go:83 | 0.04 | KEEP | - | The only precise check that a dangling dependency is reported by ID (MultipleErrors uses a weak \|\| check). |
| TestCompatibility_SpecWithNonPlanCodeBlocks | plan | compatibility_test.go:48 | 0.05 | MERGE | TestParsePlanFromMarkdown_NoPlan | Adds yaml/bash fences to the no-plan case; one more input to NoPlan. |
| TestTopologicalSortWithSimpleChain | plan | executor_test.go:12 | 0.05 | MERGE | TestCompatibility_TopologicalSortStability | A row of the TopologicalSort table. |
| TestTopologicalSortWithParallelTasks | plan | executor_test.go:63 | 0.03 | MERGE | TestCompatibility_TopologicalSortStability | A row of the TopologicalSort table. |
| TestTopologicalSortWithDiamondPattern | plan | executor_test.go:103 | 0.04 | MERGE | TestCompatibility_TopologicalSortStability | Plan -> expected layers; chain, parallel, diamond and stability are one table (assert exact IDs per layer, not just counts). |
| TestTopologicalSortWithNilPlan | plan | executor_test.go:181 | 0.03 | MERGE | TestTopologicalSortWithEmptyPlan | nil and empty plans both give zero layers; two rows. |
| TestParsePlanFromMarkdown_UnclosedBlock | plan | parser_test.go:112 | 0.04 | MERGE | TestParsePlanFromMarkdown_MultiplePlans | A row of the parse-error table. |
| TestParsePlanFromMarkdown_EmptyBlock | plan | parser_test.go:134 | 0.05 | MERGE | TestParsePlanFromMarkdown_MultiplePlans | A row of the parse-error table. |
| TestParsePlanFromMarkdown_InvalidYAML | plan | parser_test.go:152 | 0.04 | MERGE | TestParsePlanFromMarkdown_MultiplePlans | The same "(nil, error)" assertion with a different malformed block; one error table. |
| TestParsePlanFromMarkdown_MultipleTasksPlan | plan | parser_test.go:175 | 0.05 | MERGE | TestParsePlanFromMarkdown_ValidPlan | A success-table row that also checks dependency lists decode. |
| TestParsePlanFromMarkdown_WhitespaceHandling | plan | parser_test.go:249 | 0.04 | MERGE | TestParsePlanFromMarkdown_ValidPlan | A row of the success table (trailing spaces after kiro-plan, blank lines). |
| TestParsePlanFromMarkdown_TildeFence | plan | parser_test.go:347 | 0.04 | MERGE | TestParsePlanFromMarkdown_ValidPlan | A regression case (~~~ fence), but one row of the success table. |
| TestParsePlanFromMarkdown_ClosingFenceBoundary | plan | parser_test.go:379 | 0.06 | MERGE | TestParsePlanFromMarkdown_ValidPlan | A real regression (a shorter inner fence must not close), but the same parse-succeeds assertion; one row of the success table. |
| TestPlanValidate_InvalidVersion | plan | types_test.go:45 | 0.04 | MERGE | TestPlanValidate_NilPlan | A row of the sentinel-error table. |
| TestPlanValidate_EmptyPlan | plan | types_test.go:66 | 0.04 | MERGE | TestPlanValidate_NilPlan | Plan -> sentinel error; nil, empty, invalid version and missing ID are one errors.Is table. |
| TestValidatorWithUnknownAgent | plan | validator_test.go:47 | 0.04 | MERGE | TestValidatorRejectsUntrustedAgents | The same "not a trusted delegatable agent" rejection; add the "error names the agent" check to that table. |
| TestValidatorDetectsSelfCycle | plan | validator_test.go:224 | 0.05 | MERGE | TestValidatorDetectsSimpleCycle | Plan -> "circular dependency" error; self, simple and complex cycles are one table. |
| TestCompatibility_LegacySpecWithoutPlan | plan | compatibility_test.go:9 | 0.48 | DELETE | TestParsePlanFromMarkdown_NoPlan | The same (nil, nil) no-plan assertion as NoPlan; 0.48s is first-test startup. |
| TestCompatibility_MixedPlanAndLegacyContent | plan | compatibility_test.go:88 | 0.05 | DELETE | TestParsePlanFromMarkdown_ValidPlan | ValidPlan already has prose before and after the block, with the same assertions. |
| TestCompatibility_LegacyWorkflowSimulation | plan | compatibility_test.go:333 | 0.04 | DELETE | - | Asserts the test's own if/else ("simulate krew-lead logic"); the only real call is a duplicate of the NoPlan parse. |
| TestCompatibility_PlanBasedWorkflowSimulation | plan | compatibility_test.go:366 | 0.05 | DELETE | TestParsePlanFromMarkdown_ValidPlan | Asserts the test's own if/else; the parse is a duplicate of ValidPlan. |
| TestCompatibility_RegistryWithNoAgents | plan | compatibility_test.go:415 | 0.05 | DELETE | TestDiscoverAgents_MissingLeadConfig | Its assertion passes whenever an error is returned, so it checks nothing beyond MissingLeadConfig. |
| TestExecutorWithSuccessfulTasks | plan | executor_test.go:192 | 0.07 | DELETE | - | Covers dead code: internal/plan Executor (NewExecutor/ExecutePlan) is never used outside tests (cmd/kairon/cmd/plan.go uses only Parse/DiscoverAgents/Validator/TopologicalSort); delete it with executor.go. |
| TestExecutorWithFailedTask | plan | executor_test.go:240 | 0.06 | DELETE | - | Covers dead code: internal/plan Executor (NewExecutor/ExecutePlan) is never used outside tests (cmd/kairon/cmd/plan.go uses only Parse/DiscoverAgents/Validator/TopologicalSort); delete it with executor.go. |
| TestExecutorParallelExecution | plan | executor_test.go:356 | 0.10 | DELETE | - | Covers dead code: internal/plan Executor (NewExecutor/ExecutePlan) is never used outside tests (cmd/kairon/cmd/plan.go uses only Parse/DiscoverAgents/Validator/TopologicalSort); delete it with executor.go. |
| TestExecutorWithEmptyPlan | plan | executor_test.go:410 | 0.07 | DELETE | - | Covers dead code: internal/plan Executor (NewExecutor/ExecutePlan) is never used outside tests (cmd/kairon/cmd/plan.go uses only Parse/DiscoverAgents/Validator/TopologicalSort); delete it with executor.go. |
| TestExecutorWithNilPlan | plan | executor_test.go:432 | 0.07 | DELETE | - | Covers dead code: internal/plan Executor (NewExecutor/ExecutePlan) is never used outside tests (cmd/kairon/cmd/plan.go uses only Parse/DiscoverAgents/Validator/TopologicalSort); delete it with executor.go. |
| TestExecutorLayerSequencing | plan | executor_test.go:449 | 0.11 | DELETE | - | Covers dead code: internal/plan Executor (NewExecutor/ExecutePlan) is never used outside tests (cmd/kairon/cmd/plan.go uses only Parse/DiscoverAgents/Validator/TopologicalSort); delete it with executor.go. |
| TestExecutorTaskResultMapping | plan | executor_test.go:519 | 0.05 | DELETE | - | Covers dead code: internal/plan Executor (NewExecutor/ExecutePlan) is never used outside tests (cmd/kairon/cmd/plan.go uses only Parse/DiscoverAgents/Validator/TopologicalSort); delete it with executor.go. |
| TestIntegration_ParallelExecution | plan | integration_test.go:11 | 0.14 | DELETE | - | Covers dead code: internal/plan Executor (NewExecutor/ExecutePlan) is never used outside tests (cmd/kairon/cmd/plan.go uses only Parse/DiscoverAgents/Validator/TopologicalSort); delete it with executor.go. |
| TestIntegration_SequentialExecution | plan | integration_test.go:65 | 0.19 | DELETE | - | Covers dead code: internal/plan Executor (NewExecutor/ExecutePlan) is never used outside tests (cmd/kairon/cmd/plan.go uses only Parse/DiscoverAgents/Validator/TopologicalSort); delete it with executor.go. |
| TestIntegration_MixedDependencies | plan | integration_test.go:113 | 0.20 | DELETE | - | Covers dead code: internal/plan Executor (NewExecutor/ExecutePlan) is never used outside tests (cmd/kairon/cmd/plan.go uses only Parse/DiscoverAgents/Validator/TopologicalSort); delete it with executor.go. |
| TestNewAgentRegistry | plan | registry_test.go:10 | 0.04 | DELETE | - | A constructor returns an empty map; Count() has no production caller. |
| TestDiscoverAgents_NonexistentDirectory | plan | registry_test.go:80 | 0.04 | DELETE | TestDiscoverAgents_MissingLeadConfig | Hits the same os.ReadFile error branch as MissingLeadConfig. |
| TestAgentRegistry_Contains | plan | registry_test.go:105 | 0.05 | DELETE | TestValidatorRejectsUntrustedAgents | A map lookup on hand-filled internals; every validator test exercises Contains true and false. |
| TestAgentRegistry_GetConfigPath | plan | registry_test.go:128 | 0.04 | DELETE | - | Covers dead code: GetConfigPath has no production caller. |
| TestAgentRegistry_GetAgentNames | plan | registry_test.go:152 | 0.05 | DELETE | - | Lists map keys; its only production use is formatting an error message, and the order is random anyway. |
| TestAgentRegistry_Count | plan | registry_test.go:185 | 0.05 | DELETE | - | Covers dead code: Count() has no production caller. |
| TestPlanValidate_ValidPlan | plan | types_test.go:8 | 0.04 | DELETE | TestPlanValidate_TaskIDGrammar | The TaskIDGrammar valid/* subtests already assert Validate()==nil on complete plans; Plan.Validate ignores dependencies. |
| TestPlanValidate_DuplicateTaskID | plan | types_test.go:81 | 0.04 | DELETE | TestPlanValidate_AccumulatesErrors | AccumulatesErrors already asserts errors.Is(err, ErrDuplicateTaskID), and TestCompatibility_SchemaValidationErrors checks the message. |
| TestPlanValidate_MissingAgent | plan | types_test.go:132 | 0.03 | DELETE | TestPlanValidate_AccumulatesErrors | AccumulatesErrors already asserts errors.Is(err, ErrMissingAgent). |
| TestPlanGetTaskIDs_EmptyPlan | plan | types_test.go:218 | 0.05 | DELETE | - | Covers dead code: Plan.GetTaskIDs has no production caller. |
| TestValidatorWithValidPlan | plan | validator_test.go:9 | 0.03 | DELETE | TestValidatorWithValidDAG | Same nil-error assertion on a smaller valid DAG. |
| TestNoForbiddenImports | failure | deps_test.go:50 | 0.14 | KEEP | - | Enforces the package boundary (no os/exec/net, only guard/stageoutput/config) through go list; a real architectural contract. |
| TestNewRunID_ValidAndUnique | failure | marker_test.go:8 | 0.12 | KEEP | - | Run IDs are the punch-out idempotency key; uniqueness and validity of generated IDs are load-bearing. |
| TestValidRunID | failure | marker_test.go:22 | 0.04 | KEEP | - | Run IDs are embedded in HTML comments on GitHub; the rejection of -->, >, whitespace, non-ASCII and over-long IDs is security-relevant input validation. |
| TestMarker_Format | failure | marker_test.go:45 | 0.05 | KEEP | - | Pins the exact on-GitHub marker string; old comments must keep parsing, so a format change must be deliberate. |
| TestMarker_RoundTrip | failure | marker_test.go:51 | 0.05 | KEEP | - | Marker/ParseMarker agreement, including CRLF bodies; this is the persisted punch-out idempotency format. |
| TestDecide_InvalidAttemptFailsToPunchOut | failure | policy_test.go:107 | 0.04 | KEEP | - | An invalid attempt must fail toward punch-out, never toward an unbounded retry; host for the Decide table. |
| TestLabels | failure | policy_test.go:125 | 0.04 | KEEP | - | Pins the user-facing <label>-failed/-done names that README documents and that production builds Targets from (intake.go uses FailedLabel). |
| TestPunchOut_CommentBeforeLabel | failure | punchout_test.go:27 | 0.05 | KEEP | - | Comment-before-label ordering is the central failure-contract invariant. |
| TestReconcile_AppliesMissingLabelOnly | failure | punchout_test.go:164 | 0.04 | KEEP | - | The only test asserting that Reconcile adds only the label (no second comment) through the exact call sequence. |
| TestRenderComment_ContainsStageAttemptsExpectedActualRemedy | failure | render_test.go:22 | 0.05 | KEEP | - | The core content contract of the failure comment (stage, guard, attempts, expected, actual, remedy, label). |
| TestRenderComment_DefaultRemedy | failure | render_test.go:55 | 0.04 | KEEP | - | The only test of the empty-remedy fallback text that tells a human what to do. |
| TestRoute_TerminalRecordsDispositionAndRemedy | failure | route_test.go:25 | 0.04 | KEEP | - | Terminal records Unrecoverable plus the remedy; host for the Route table. |
| TestRunner_UnrecoverableSkipsRetries | failure | runner_test.go:88 | 0.04 | KEEP | - | The core tier-2 rule: a terminal failure punches out on attempt 1 with the route's remedy in the comment. |
| TestParseMarker_RejectsNonFirstLine | failure | marker_test.go:67 | 0.04 | MERGE | TestParseMarker_RejectsMalformed | The same "ParseMarker(b) must be false" loop over different bodies; merge the two input lists. |
| TestDecide_UnrecoverableAlwaysPunchesOut | failure | policy_test.go:82 | 0.05 | MERGE | TestDecide_InvalidAttemptFailsToPunchOut | The four Decide tests are one (attempt, disposition) -> decision table. |
| TestDecide_RecoverableRetriesWhileAttemptsRemain | failure | policy_test.go:90 | 0.04 | MERGE | TestDecide_InvalidAttemptFailsToPunchOut | A row of the Decide table. |
| TestDecide_RecoverablePunchesOutAtMax | failure | policy_test.go:98 | 0.04 | MERGE | TestDecide_InvalidAttemptFailsToPunchOut | A row of the Decide table. |
| TestPunchOut_StateErrorWritesNothing | failure | punchout_test.go:153 | 0.04 | MERGE | TestPunchOut_InvalidInputWritesNothing | Same "error -> zero GitHub writes" assertion; add a failNext("State") row. |
| TestRenderComment_AttemptsReflectContext | failure | render_test.go:47 | 0.07 | MERGE | TestRenderComment_ContainsStageAttemptsExpectedActualRemedy | Useful only because the main test uses Current==Max (4 of 4); change that fixture to 2 of 3 and drop this test. |
| TestRoute_RecoverableRecordsContext | failure | route_test.go:10 | 0.04 | MERGE | TestRoute_TerminalRecordsDispositionAndRemedy | Same handler-records-context assertion for the other disposition; one table over {Recoverable, Terminal, none}. |
| TestRoute_ResultIsSuccessWhenNoFailureRecorded | failure | route_test.go:43 | 0.04 | MERGE | TestRoute_TerminalRecordsDispositionAndRemedy | The "nothing recorded" row of the Route table. |
| TestRunner_RefusesWhileTerminalPending | failure | runner_test.go:157 | 0.05 | MERGE | TestRunner_RefusesWhileLabeled | The same "halted, stage not called" assertion with a different seed (marker comment versus label); add it as a third subtest. |
| TestRunner_OverwritesStageAttempt | failure | runner_test.go:216 | 0.06 | MERGE | TestRunner_ExhaustionPunchesOut | ExhaustionPunchesOut already shows the runner overwrites the stage-reported 99/99 in Failure; move the "comment says N of N, not 99" check there. |
| TestClassify_NoMarkerIsEligible | failure | state_test.go:23 | 0.04 | MERGE | TestClassify_DoneAndFailed | A row of the Classify table. |
| TestClassify_LabelRemovedBeforeMarkerStaysPending | failure | state_test.go:47 | 0.05 | MERGE | TestClassify_DoneAndFailed | A row of the Classify table. |
| TestClassify_NewMarkerAfterRestartIsPending | failure | state_test.go:76 | 0.04 | MERGE | TestClassify_DoneAndFailed | IssueState -> Selection with different comment/label timing; one Classify table. |
| TestClassify_UsesNewestMarkerRegardlessOfOrder | failure | state_test.go:89 | 0.04 | MERGE | TestClassify_DoneAndFailed | A row of the Classify table (same state as NewMarkerAfterRestart with comments reversed). |
| TestClassify_IgnoresMarkerNotByViewer | failure | state_test.go:100 | 0.05 | MERGE | TestClassify_DoneAndFailed | A security-relevant case (an untrusted marker is ignored), but one row of the Classify table. |
| TestDisallowedDirectImport | failure | deps_test.go:26 | 0.73 | DELETE | TestNoForbiddenImports | Tests a helper defined in the test file (the test's own setup), not package code. |
| TestDocs_NamesRequiredTerms | failure | docs_test.go:20 | 0.04 | DELETE | - | Pins 20 words and issue numbers in docs/failure-contract.md; no code or agent reads that doc. |
| TestDocs_HasNoYAMLFence | failure | docs_test.go:36 | 0.04 | DELETE | - | A one-off acceptance criterion from issue #379 (spec line 507); nothing parses the doc. |
| TestPolicyFromRetries | failure | policy_test.go:21 | 0.05 | DELETE | TestPolicyFor_FollowsConfig | PolicyFor_FollowsConfig and TestRunner_RetryCountFollowsConfig already pin retries -> retries+1 and negatives -> 1. |
| TestPolicyAttempt | failure | policy_test.go:32 | 0.05 | DELETE | TestRunner_FreshRunStartsAtAttemptOne | Policy.Attempt(n)={n,Max} is checked through the runner's seen-attempt assertions; Valid() is guard's behaviour. |
| TestDispositionZeroValueIsRecoverable | failure | policy_test.go:118 | 0.03 | DELETE | - | Asserts iota order; production never relies on the zero value (Route always sets the disposition explicitly). |
| TestPunchOut_NewRunIDPostsFreshComment | failure | punchout_test.go:94 | 0.04 | DELETE | TestRunner_FreshRunStartsAtAttemptOne | FreshRunStartsAtAttemptOne clears the label, punches out with run2, and asserts posted IDs [run1 run2]: the same path. |
| TestRenderComment_FirstLineIsOnlyMarker | failure | render_test.go:200 | 0.05 | DELETE | TestRenderComment_ContainsStageAttemptsExpectedActualRemedy | The main render test already asserts the first line == Marker(run) and that ParseMarker round-trips. |
| TestRoute_ResultDoesNotAliasRecordedContext | failure | route_test.go:61 | 0.05 | DELETE | - | The handler takes FailureContext by value, so Go semantics make the caller's mutation invisible; the test cannot fail. |
| TestRunner_NoAutoRetryAfterTerminal | failure | runner_test.go:194 | 0.05 | DELETE | TestRunner_ExhaustionPunchesOut | ExhaustionPunchesOut already re-runs after the punch-out and asserts halted with no extra stage call. |
| TestRunner_StageAttemptArgumentCountsFromOne | failure | runner_test.go:316 | 0.04 | DELETE | TestRunner_FreshRunStartsAtAttemptOne | FreshRunStartsAtAttemptOne already DeepEquals the attempts passed to the stage ({1,2},{2,2}). |
| TestSelectionExcluded | failure | state_test.go:157 | 0.08 | DELETE | TestClassify_DoneAndFailed | Excluded() for every Selection is already asserted inline by TerminalPendingIsExcluded, LabelClearedAfterMarkerIsEligible and DoneAndFailed. |
| TestCanonicalize_RemovesMarkerNormalizesEndingsStripsTrailingWhitespace | provenance | canonical_test.go:5 | 0.78 | KEEP | - | The canonicalization table that defines the hash input; its 0.78s is first-test startup. |
| TestCommitChain_AdmitSeesSubstitutedDocument | provenance | chain_test.go:166 | 0.04 | KEEP | - | The guard must admit the substituted text that will actually be written, and the caller's item must stay unmutated; a security-relevant ordering. |
| TestCommitChain_UnknownPlaceholderWritesNothing | provenance | chain_test.go:201 | 0.05 | KEEP | - | A forward reference stops the chain after item 1 with ErrUnknownPlaceholder; a distinct stop path. |
| TestCommitChain_StopsOnRejection | provenance | chain_test.go:227 | 0.04 | KEEP | - | Rejection mid-chain keeps earlier commits and never processes later items; core chain semantics. |
| TestCommit_WritesV1MarkerAsLastLine | provenance | commit_test.go:53 | 0.05 | KEEP | - | End-to-end exact committed body and hash against an independent sha256. |
| TestCommit_DefaultClockIsTimeNow | provenance | commit_test.go:94 | 0.06 | KEEP | - | Both production Committers (tui/planning_handoff.go, agent/intake.go) leave Now nil, so the default-clock branch is the live path. |
| TestCommit_RefusesWithoutAllPass | provenance | commit_test.go:148 | 0.05 | KEEP | - | The admission gate: no store or router call unless guards report ready, for create and update. |
| TestCommit_ConcurrentEditAbortsWrite | provenance | commit_test.go:261 | 0.04 | KEEP | - | Optimistic-concurrency rule: a revision change routes to intake and never overwrites a human edit. |
| TestNoForbiddenImports | provenance | deps_test.go:61 | 0.22 | KEEP | - | Purity boundary (no os/net/hmac, only guard/stageoutput internally) through go list; a real architectural contract. |
| TestDocs_ExamplesClassifyAsLabelled | provenance | docs_test.go:42 | 0.05 | KEEP | - | Executable docs: every labelled example body in docs/provenance.md must classify as its label under the real Classify. |
| TestHash_IsSHA256OfCanonicalBody | provenance | marker_test.go:16 | 0.04 | KEEP | - | Pins the hash algorithm against an independent sha256; the hash is persisted on GitHub issues. |
| TestMarker_Format | provenance | marker_test.go:37 | 0.04 | KEEP | - | Pins the persisted v1 marker string; host for the marker-format rows. |
| TestMark_MarkerIsLastLineWithNoTrailingNewline | provenance | marker_test.go:57 | 0.05 | KEEP | - | Pins the committed-body layout (canonical body, then the marker as the last line, no trailing newline). |
| TestMark_MarkingTwiceYieldsSameHash | provenance | marker_test.go:75 | 0.05 | KEEP | - | Re-marking is idempotent (one marker, same hash); host for the marker-excluded-from-hash cases. |
| TestSubstituteDependencies_OnlyDependenciesLine | provenance | substitute_test.go:84 | 0.05 | KEEP | - | Placeholder-like text outside a Dependencies: line (Ruby #{x}) must be left alone; host for the "body unchanged" table. |
| TestCanonicalize_EquivalentBodiesMatch | provenance | canonical_test.go:37 | 0.05 | MERGE | TestCanonicalize_RemovesMarkerNormalizesEndingsStripsTrailingWhitespace | More inputs to the same Canonicalize table; carry over its idempotency check (Canonicalize(got)==got) as a per-row assertion. |
| TestCommit_DoesNotMutateDocument | provenance | commit_test.go:432 | 0.07 | MERGE | TestCommit_WritesV1MarkerAsLastLine | Same setup (commit commitTestBody); add the doc.Body-unchanged assertion there. |
| TestHash_ExcludesMarker | provenance | marker_test.go:25 | 0.05 | MERGE | TestMark_MarkingTwiceYieldsSameHash | Both assert Hash(Mark(b))==Hash(b) regardless of timestamp; fold into one test. |
| TestMarker_ConvertsToUTC | provenance | marker_test.go:49 | 0.04 | MERGE | TestMarker_Format | A non-UTC time input is one more row of the marker-format check. |
| TestMark_EmptyBodyReturnsMarkerAlone | provenance | marker_test.go:101 | 0.05 | MERGE | TestMark_MarkerIsLastLineWithNoTrailingNewline | Mark(input) -> exact output; the empty body is one more row. |
| TestSubstituteDependencies_NoPlaceholderUnchanged | provenance | substitute_test.go:159 | 0.06 | MERGE | TestSubstituteDependencies_OnlyDependenciesLine | The same "output == input, no error" loop; merge the two input lists. |
| TestSubstituteDependencies_SingleLineOutput | provenance | substitute_test.go:184 | 0.05 | MERGE | TestSubstituteDependencies_ReplacesPlaceholders | Asserts an exact want string, so it is one more ReplacesPlaceholders row; the bullet and #{ checks are implied by the exact match. |
| TestCommit_MarkedInputBodyKeepsSingleMarker | provenance | commit_test.go:444 | 0.05 | DELETE | TestMark_MarkingTwiceYieldsSameHash | Single marker after re-marking is asserted by MarkingTwice, and stale-marker removal by the Canonicalize "every marker removed" case. |
| TestDisallowedDirectImport | provenance | deps_test.go:33 | 0.04 | DELETE | TestNoForbiddenImports | Tests a helper defined in the test file, not package code. |
| TestDocs_NamesRequiredTerms | provenance | docs_test.go:29 | 0.05 | DELETE | - | Pins words and issue numbers (#377, #381, #382) in docs/provenance.md; nothing reads them. |
| TestDocs_StatesHashIsNotContentTrust | provenance | docs_test.go:70 | 0.04 | DELETE | - | Pins two phrases in docs/provenance.md; this is a doc-review concern, not executable behaviour. |
| TestDefaultEvalsConfigReturnsFreshSlice | config | evals_test.go:52 | 0.06 | KEEP | - | The doc comment promises callers may mutate AllowedModels, and Load decodes over it; guards a shared-slice regression. |
| TestLoadEvals_PartialBlockKeepsOtherDefaults | config | evals_test.go:103 | 0.04 | KEEP | - | Partial block keeps the other defaults; host for the LoadEvals (yaml -> EvalsConfig) table. |
| TestLoadEvals_TrimsWhitespaceAndDropsEmptyEntries | config | evals_test.go:132 | 0.05 | KEEP | - | The only test of EvalsConfig.normalize trimming and dropping blank allowlist entries. |
| TestLoad_EvalsPartialBlockAndNoModelRejection | config | evals_test.go:240 | 0.06 | KEEP | - | Pins the design rule that Load does not enforce model values (enforcement happens at eval run time); host for the Load-evals table. |
| TestConfigFilesDocumentEvalsBlock | config | evals_test.go:286 | 0.04 | KEEP | - | Live/template config.yaml evals blocks must match and stay all-comment; task sync:check does not diff config.yaml, so nothing else enforces this. |
| TestConfigFilesEvalsBlockMatchesDefaults | config | evals_test.go:317 | 0.04 | KEEP | - | Uncommenting the documented block must give exactly DefaultEvalsConfig; a real docs-to-code contract. |
| TestShippedConfigFiles_PricingBlockPricesEverySonnetModel | config | pricing_files_test.go:49 | 0.03 | KEEP | - | Parses the raw shipped YAML (project and template), so it fails when the active pricing block is removed, which LoadPricing-based tests cannot detect. |
| TestDefaultPricing_SonnetAt3And15NoHaiku | config | pricing_test.go:20 | 0.03 | KEEP | - | The built-in price table drives cost reporting; pinning $3/$15 for Sonnet, no Haiku price, and Validate()==nil is a deliberate product decision, not trivia. |
| TestDefaultPricing_ReturnsFreshTable | config | pricing_test.go:43 | 0.03 | KEEP | - | Load hands this map to callers who may mutate it; this guards a likely refactor (hoisting the literal to a package var) that would silently share state. |
| TestLoadPricing_PresentKeyReplacesDefaults | config | pricing_test.go:90 | 0.03 | KEEP | - | Core "present key replaces, never merges" rule that lets an operator make a model cost unknown; host for the present-key table. |
| TestLoad_SandboxConfigParsing | config | sandbox_test.go:53 | 0.06 | KEEP | - | Only test that a partial or empty sandbox block keeps the other defaults through Load; host for the sandbox table. |
| TestLoad_EnableCopilotReviewDefaults | config | config_test.go:9 | 0.71 | MERGE | TestLoad_UserField | The same scalar-field decode with copied boilerplate; its 0.71s is first-test-in-binary startup, not the test itself. |
| TestLoad_UserField | config | config_test.go:134 | 0.05 | MERGE | TestLoad_BaseBranch, TestLoad_EnableCopilotReviewDefaults | Three copies of one "decode a scalar field" test, each with about 30 lines of chdir boilerplate; replace with one (yaml, field, want) table that uses writeProjectConfig. |
| TestLoad_BaseBranch | config | config_test.go:193 | 0.05 | MERGE | TestLoad_UserField | The same scalar-field decode with copied boilerplate; the "only in a comment" case tests yaml.v3, not Kairon. |
| TestLoadEvals_MissingFileReturnsDefaults | config | evals_test.go:61 | 0.04 | MERGE | TestLoadPricing_MissingFileReturnsDefaults | The same ErrNotExist-gives-defaults branch, duplicated per loader; one table row each. |
| TestLoadEvals_NoEvalsKeyReturnsDefaults | config | evals_test.go:72 | 0.06 | MERGE | TestLoadPricing_NoPricingKeyReturnsDefaultsWithoutRepo | Both sub-loaders share the "absent input gives defaults, no repo key needed" contract; one table over {missing file, no key, null key} x {LoadEvals, LoadPricing}. |
| TestLoadEvals_AllowedModelsReplacesDefault | config | evals_test.go:120 | 0.05 | MERGE | TestLoadEvals_PartialBlockKeepsOtherDefaults | "Replace, not append" is one more YAML-in, EvalsConfig-out row. |
| TestLoadEvals_ExplicitEmptyJudgeModelStaysEmpty | config | evals_test.go:158 | 0.04 | MERGE | TestLoadEvals_PartialBlockKeepsOtherDefaults | A YAML-in, EvalsConfig-out case (explicit "" overrides the default); the CheckModel("") rejection is already a TestCheckModel case. |
| TestLoadEvals_ExplicitEmptyAllowedModels | config | evals_test.go:172 | 0.04 | MERGE | TestLoadEvals_PartialBlockKeepsOtherDefaults | "allowed_models: []" is one more YAML-in, EvalsConfig-out row. |
| TestLoad_ExposesEvalsDefaults | config | evals_test.go:229 | 0.04 | MERGE | TestLoad_AllDefaultValues | A repo-only config exposes Evals defaults; one more assertion in TestLoad_AllDefaultValues. |
| TestLoad_EvalsAllowedModelsReplacesDefault | config | evals_test.go:259 | 0.04 | MERGE | TestLoad_EvalsPartialBlockAndNoModelRejection | Both decode an evals block through Load (separate path from LoadEvals) and compare cfg.Evals; fold into one Load-evals table. |
| TestLoadEvals_TrustToolsDefaultsNil | config | evals_test.go:343 | 0.04 | MERGE | TestLoadEvals_PartialBlockKeepsOtherDefaults | nil versus empty TrustTools changes sandbox trust behaviour, so keep the case, but as a table row. |
| TestLoadPricing_MissingFileReturnsDefaults | config | pricing_test.go:56 | 0.03 | MERGE | TestLoadEvals_MissingFileReturnsDefaults | A row of the shared absent-input-gives-defaults table. |
| TestLoadPricing_NoPricingKeyReturnsDefaultsWithoutRepo | config | pricing_test.go:67 | 0.04 | MERGE | TestLoadEvals_NoEvalsKeyReturnsDefaults | A row of the shared absent-input-gives-defaults table. |
| TestLoadPricing_NullKeyReturnsDefaults | config | pricing_test.go:79 | 0.04 | MERGE | TestLoadEvals_NoEvalsKeyReturnsDefaults | A distinct branch (!!null node), but the same absent-gives-defaults assertion; belongs as a row of the shared defaults table. |
| TestLoadPricing_EmptyMappingReplacesDefaultsWithEmptyTable | config | pricing_test.go:106 | 0.04 | MERGE | TestLoadPricing_PresentKeyReplacesDefaults | "pricing: {}" gives an empty table, versus one entry; same present-key-replaces rule, so two rows of one table. |
| TestLoad_NoPricingKeyUsesDefaults | config | pricing_test.go:232 | 0.04 | MERGE | TestLoad_AllDefaultValues | Same "Load a repo-only config, check a default" shape; pricing defaults should be one more assertion in TestLoad_AllDefaultValues. |
| TestLoad_SandboxConfigDefaults | config | sandbox_test.go:10 | 0.04 | MERGE | TestLoad_SandboxConfigParsing | "No sandbox key" is one more row next to the "empty sandbox section" case. |
| TestLoadThemeReturnsErrorForMissingTheme | config | themes_test.go:103 | 0.04 | MERGE | TestLoadThemeInvalidName | Same assertion (error and nil theme) with a different bad name; add "nonexistent-theme" to the InvalidName list. |
| TestDefaultEvalsConfig | config | evals_test.go:39 | 0.05 | DELETE | TestLoadEvals_PartialBlockKeepsOtherDefaults | Restates the DefaultEvalsConfig literal; PartialBlockKeepsOtherDefaults already asserts JudgeModel==DefaultJudgeModel and AllowedModels==wantDefaultAllowed, and TestConfigFilesEvalsBlockMatchesDefaults ties the defaults to the docs. |
| TestShippedConfigFiles_PricingBlockIsDocumented | config | pricing_files_test.go:86 | 0.04 | DELETE | TestShippedConfigFiles_PricingBlockPricesEverySonnetModel | The "active pricing: line" check duplicates PricesEverySonnetModel (raw.Pricing != nil); the rest pins comment words ("exact", "per million", "replace") that nothing reads. |
| TestLoad_PresentPricingKeyReplacesDefaults | config | pricing_test.go:243 | 0.05 | DELETE | TestLoadPricing_PresentKeyReplacesDefaults | Load calls the same pricingFromYAML that TestLoadPricing_PresentKeyReplacesDefaults covers; TestLoad_InvalidPricingRejectedNamingModel and TestShippedConfigFiles_Loadable already prove Load wires pricing in. |
| TestLoad_DurationParsing | config | sandbox_test.go:238 | 0.06 | DELETE | TestLoad_SandboxConfigParsing | Five duration formats test yaml.v3/time.ParseDuration, not Kairon; the "timeout: 10m" case in SandboxConfigParsing already proves the wiring. |

### 2. Counts

| verdict | count | total secs |
|---|---|---|
| KEEP | 68 | 6.21 |
| MERGE | 64 | 3.61 |
| REWRITE-CHEAPER | 1 | 0.91 |
| DELETE | 84 | 5.61 |
| **total** | **217** | **16.34** |

Per package:

| pkg | KEEP | MERGE | REWRITE-CHEAPER | DELETE |
|---|---|---|---|---|
| planner | 23 | 8 | 1 | 38 |
| plan | 6 | 16 | 0 | 26 |
| failure | 13 | 15 | 0 | 12 |
| provenance | 15 | 7 | 0 | 4 |
| config | 11 | 18 | 0 | 4 |

### 3. Recurring patterns

1. **Prompt and skill wording pins (planner, about 22 DELETE).** These are word lists, forbidden-word lints and phrase regexes over `.kiro/agents/*-prompt.md` and `.kiro/skills/planner-shared/SKILL.md`. No code parses that text, and some patterns are broad enough (`confirm`, `Other`, `recommend`, `\bmodes?\b`) to trip on harmless edits. Examples: TestInteractivePromptEncodesElicitationFlow (27 regexes), TestSkillHasNoModeSpecificText, TestAutomatedPromptHasNoInteractiveOrModeText, TestSkillIssueBodyStandard, the two *HasRTCCHeadings tests. The prompt tests worth keeping pin text that code or the loader depends on: the `Document path:`/`Label answer path:` lines and the yes/no answer (written and parsed in tui/planning_handoff.go), the Dependencies line format and `#{key}` grammar (DependenciesForm, SubstituteDependencies), skill front matter (the Kiro loader), parseable YAML examples, and the variant-permission statements (punch_out allowed for automated, forbidden for interactive).
2. **Live/template byte-equality checks that `task sync:check` already does in CI (ci.yml:33).** TestAutomatedFilesMatchTemplates, TestInteractiveFilesMatchTemplates, TestTemplateSkillIsByteIdentical and TestSyncGate_SharedSkillTemplateInSync (the last two are literally the same test), plus subset checks such as TestInteractiveTemplatePromptMatchesLive. If the maintainer wants drift caught inside `task test`, keep one Go test that runs the sync:check diff across all synced trees. Exception: TestConfigFilesDocumentEvalsBlock is the only check on config.yaml and stays.
3. **Tests of dead code (internal/plan).** `Executor`/`ExecutePlan` and `GetTaskIDs` (which gains a caller in PR #416 and stays), `GetTaskByID`, `GetConfigPath`, `Count`, `ParsePlanFromFile`, `ValidatePlanWithRegistry` have no caller outside tests; `kairon plan parse` uses only Parse, DiscoverAgents, Validator and TopologicalSort. Candidates: TestExecutorWithSuccessfulTasks, TestIntegration_ParallelExecution, TestExecutorLayerSequencing (10 executor candidates in all). Delete the Executor code (not the whole of executor.go: `TopologicalSort`, defined in the same file, is called by `kairon plan parse` at cmd/kairon/cmd/plan.go:77 and stays with its tests), and with it the non-candidate executor tests: TestExecutorWithPartialFailure, TestExecutorRespectsConcurrencyLimit, TestNewExecutorWithLimit_NonPositiveFallsBackToDefault, TestIntegration_EndToEnd, TestIntegration_TaskFailureHandling.
4. **Copy-paste variants that belong in one table.** Examples: TestPlanValidate_InvalidVersion/_EmptyPlan with TestPlanValidate_NilPlan; TestParsePlanFromMarkdown_EmptyBlock/_InvalidYAML/_UnclosedBlock with _MultiplePlans; TestLoadEvals_MissingFileReturnsDefaults with TestLoadPricing_MissingFileReturnsDefaults, NoKey and NullKey (one absent-input-gives-defaults table across both sub-loaders); TestLoad_UserField/_BaseBranch/_EnableCopilotReviewDefaults (each carries about 30 lines of chdir boilerplate that writeProjectConfig replaces); the four TestDecide_* tests; the six TestClassify_* candidates.
5. **Tests that cannot fail, or test only themselves.** TestDisallowedDirectImport (failure and provenance) tests a helper defined in the test file. TestSyncGate_DriftDetected proves Go `==` notices an appended byte. TestRoute_ResultDoesNotAliasRecordedContext checks pass-by-value. TestExtractErrorsAreDistinct checks that two `errors.New` sentinels differ. TestSameContractBoundaryResultIsOneTypeForBothAgents compares `reflect.TypeOf` of one function's return type. TestCompatibility_LegacyWorkflowSimulation/PlanBasedWorkflowSimulation assert their own if/else. TestCompatibility_RegistryWithNoAgents passes on any error.

Side finding (not a verdict): TestEvalDriverExitsNonZeroWhenEitherHalfFails (0.91s) is the only REWRITE-CHEAPER. TestPlanningWorktreeScripts_ParallelRunsGetDistinctWorktrees (0.88s, KEEP) and the eval-driver tests don't call t.Parallel(), although most of the planner package does.

## Group: rest

### Verdicts: rest packages

| test | pkg | file:line | secs | verdict | partner | reason |
|---|---|---|---|---|---|---|
| TestAdmit_ReadsFromGitHubThenClassifiesBeforeAnyResult | internal/boundary | boundary_test.go:29 | 0.74 | KEEP |  | Only test asserting ReadIssue is called once with the right repo/issue/fail label before Classify; the partner never inspects reader args or call order. |
| TestAdmit_ValidBodyYieldsInputDocument | internal/boundary | boundary_test.go:60 | 0.05 | KEEP |  | Asserts every field of the admitted InputDocument (canonical body, hash of raw, marker stripped, header), which the partner does not. |
| TestAdmit_WithoutIntakeActualSaysSo | internal/boundary | boundary_test.go:175 | 0.06 | MERGE | TestAdmit_NonValidBodiesNeverAdmitted | Same nil-intake punch-out path as the table test; its one extra assertion (Actual says no automated intake is configured) belongs as one more check in that table's loop. |
| TestAdmit_NonValidStartsIntakeAndSkips | internal/boundary | boundary_test.go:189 | 0.04 | KEEP |  | Asserts the none/mismatch-with-intake path yields Skipped/IntakeStarted with exact ReadIssue->Classify->Intake order and no re-read. |
| TestAdmit_IntakeSpentPunchesOut | internal/boundary | boundary_test.go:230 | 0.05 | KEEP |  | Only in-package test of the IntakeSpent branch (body changed after intake committed) and its remedy text naming `validate <n>`. |
| TestAdmit_ValidHashIsNotAContentVerdict | internal/boundary | boundary_test.go:505 | 0.06 | MERGE | TestAdmit_ValidBodyYieldsInputDocument | Admitting an injection-bearing valid body is the same path as the valid-body test; the 'no leak into Repo/Title' checks are trivially true from fixtures, so fold the injection body in as a second case. |
| TestDisallowedDirectImport | internal/boundary | deps_test.go:38 | 0.04 | DELETE |  | Tests the test file's own helper predicate, not production code; TestNoForbiddenImports already fails loudly if the helper misclassifies a real import. |
| TestNoForbiddenImports | internal/boundary | deps_test.go:68 | 0.21 | KEEP |  | Enforces the package's import boundary (no os/exec/net, no github/agent/watcher deps) via go list, a real architectural contract coverage cannot see. |
| TestDocs_NamesRequiredTerms | internal/boundary | docs_test.go:27 | 0.04 | DELETE |  | Greps docs/boundary.md for 14 words and issue numbers; pins incidental wording, not whether the doc is correct. |
| TestDocs_InputExamplesParse | internal/boundary | docs_test.go:41 | 0.05 | KEEP |  | Executes every yaml-input example in docs/boundary.md through ParseInputDocument and checks the hash, a real doc-to-code contract. |
| TestMarshal_RoundTripsBodiesByteForByte | internal/boundary | document_test.go:22 | 0.05 | KEEP |  | Byte-for-byte round trip and Marshal stability over YAML-hostile and injection bodies is the core wire contract of the input document. |
| TestMarshal_WireForm | internal/boundary | document_test.go:59 | 0.05 | KEEP |  | Pins the agent-facing wire form: untrusted-data header, key order, literal block body, and absence of stage/source/inference keys. |
| TestDefaultScreen_Accepts | internal/boundary | screen_test.go:8 | 0.04 | KEEP |  | Accept-side boundaries of the screen (empty, tabs/newlines, unicode, exactly MaxBodyBytes) complement the reject table; without it an over-strict screen passes. |
| TestDefaultScreen_ErrorNamesTheProblemNotTheContent | internal/boundary | screen_test.go:47 | 0.05 | KEEP |  | Security property (screen error must not echo the untrusted body) that TestDefaultScreen_Rejects never checks. |
| TestSelectIssues_ExcludesTerminalPending | internal/github | adapter_test.go:110 | 0.75 | KEEP |  | Core classification of a viewer-marked issue as TerminalPending (vs eligible) and no events fetch for unmarked issues; the partner only checks Eligible. |
| TestSelectIssues_ExcludesDoneAndFailed | internal/github | adapter_test.go:134 | 0.05 | KEEP |  | Asserts no events API call for a labeled issue that carries a marker and empty TerminalPending; the partner only checks the eligible list. |
| TestIssueState_ByViewerAndLabels | internal/github | adapter_test.go:353 | 0.04 | KEEP |  | Only test of the trust rule that an empty viewer matches no author (ByViewer false), a security-relevant edge the partner never exercises. |
| TestCLI_EventsCallNeverCarriesLabelText | internal/github | adapter_test.go:504 | 0.05 | KEEP |  | Injection guard: a hostile label string must never reach gh args or stdin; no other test checks this. |
| TestIssueStoreImplementsProvenanceStore | internal/github | issuestore_test.go:31 | 0.05 | REWRITE-CHEAPER |  | No runtime assertion; replace with a package-level `var _ provenance.IssueStore = IssueStore{}` (as read_test.go already does for boundary.Reader), ideally in issuestore.go so `go build` fails, not just `go test`. |
| TestCLI_ReadIssue_ExactArgs | internal/github | read_test.go:27 | 0.05 | KEEP |  | Pins the exact gh argv and a single call, the external CLI contract; the partner checks parsed output only. |
| TestCLI_ReadIssue_OpenOnlyForStateOpen | internal/github | read_test.go:78 | 0.05 | KEEP |  | Table over OPEN/CLOSED/open/''/MERGED pins the case-sensitive Open mapping; the partner only feeds OPEN. |
| TestRun_StopsAtFirstFailure | internal/guard | chain_test.go:13 | 0.14 | KEEP |  | Short-circuit contract: later guards never run after a rejection, with exact admit/fail log order. |
| TestRun_AllPassReadyToCommit | internal/guard | chain_test.go:29 | 0.04 | KEEP |  | Asserts ReadyToCommit outcome, each guard admitted once, no OnFailure, and that the runner adds no log entries. |
| TestRun_FailureRoutedToOwningGuardOnce | internal/guard | chain_test.go:51 | 0.04 | MERGE | TestRun_StopsAtFirstFailure | Identical one/two(reject)/three chain; StopsAtFirstFailure checks order, this checks the routed FailureContext, so combine into one test. |
| TestRun_ResultFailureIsACopy | internal/guard | chain_test.go:75 | 0.04 | KEEP |  | Only aliasing check: Result.Failure must not be the guard's pointer. |
| TestRun_FailureErrPassedThrough | internal/guard | chain_test.go:85 | 0.03 | KEEP |  | Only test asserting the OnFailure handler's error is surfaced as Result.FailureErr (assert.Same). |
| TestRun_PassesInputToEveryGuard | internal/guard | chain_test.go:96 | 0.04 | MERGE | TestRun_AllPassReadyToCommit | Same all-pass chain setup; add the `admitted == []Input{in}` check to the all-pass test. |
| TestDefaultChain_Order | internal/guard | chain_test.go:201 | 0.04 | DELETE |  | Pins len==1 and type of a one-element chain; TestStructural_InChain already runs DefaultChain and asserts the structural guard fires. |
| TestDisallowedDirectImport | internal/guard | deps_test.go:34 | 0.04 | DELETE |  | Tests the test file's own helper predicate, not production code. |
| TestNoForbiddenImports | internal/guard | deps_test.go:54 | 0.17 | KEEP |  | Enforces guard's purity boundary (stdlib + stageoutput only, no os/exec/net) via go list. |
| TestDocs_NamesRequiredTerms | internal/guard | docs_test.go:34 | 0.04 | DELETE |  | Word grep over docs/guards.md; pins wording, not correctness, and the YAML example tests already tie the doc to code. |
| TestDocs_ValidYAMLExamplesAreAdmitted | internal/guard | docs_test.go:49 | 0.05 | KEEP |  | Executes yaml examples in docs/guards.md through Structural.Admit; real doc contract. |
| TestDocs_InvalidYAMLExamplesAreRejected | internal/guard | docs_test.go:63 | 0.06 | KEEP |  | Executes yaml-invalid examples in docs/guards.md through Structural.Admit and requires at least one; real doc contract. |
| TestGuard_InterfaceShape | internal/guard | guard_test.go:57 | 0.05 | DELETE |  | Reflection re-states the Guard interface declaration; the compiler already enforces it for every implementer and any change is deliberate. |
| TestInput_HasNoMutableFields | internal/guard | guard_test.go:70 | 0.04 | KEEP |  | Reflection enforces that Input holds only strings/ints/structs (guards cannot mutate shared state), an invariant the compiler cannot check. |
| TestAttempt_Valid | internal/guard | guard_test.go:167 | 0.05 | KEEP |  | Direct boundary cases for Attempt.Valid (1/1, -1, zero Max) that no Input/FailureContext validation table covers; microseconds. |
| TestInput_Validate | internal/guard | guard_test.go:176 | 0.04 | DELETE |  | Its four invalid inputs are exactly the four TestRun_RejectsBadChain feeds through Run with the same ErrInvalid assertion; duplicates TestRun_RejectsBadChain. |
| TestInput_Document | internal/guard | guard_test.go:199 | 0.04 | KEEP |  | Asserts Input.Document decodes the body, returns a fresh copy per call, and errors on bad raw; not asserted elsewhere. |
| TestErrors_EmptyChainMatchesInvalid | internal/guard | guard_test.go:228 | 0.04 | DELETE |  | ErrorIs(ErrEmptyChain, ErrInvalid) is already asserted by TestRun_RejectsBadChain; the remaining check asserts a sentinel constant's wrapping. |
| TestStructural_MissingVerifyNamesCriterionNumbers | internal/guard | structural_test.go:254 | 0.04 | MERGE | TestStructural_Admit | Overlaps the table's 'second criterion lacks Verify' row; move the exact message and the 1,2 case into table rows. |
| TestStructural_MissingSectionMessages | internal/guard | structural_test.go:267 | 0.05 | MERGE | TestStructural_Admit | Same input as the table's 'multiple missing sections' row; add exact Expected/Actual strings to that row instead of a separate test. |
| TestStructural_RejectsEmptyLaterAcceptanceCriteriaSection | internal/guard | structural_test.go:275 | 0.06 | MERGE | TestStructural_Admit | Distinct input (a later AC section with no list items) but identical assertion shape; one more table row. |
| TestStructural_ChecksEveryAcceptanceCriteriaSection | internal/guard | structural_test.go:289 | 0.05 | MERGE | TestStructural_Admit | Distinct input (second AC section missing Verify) with the table's assertion shape; one table row. |
| TestStructural_AcceptanceCriteriaSectionEndsAtNextHeading | internal/guard | structural_test.go:303 | 0.04 | MERGE | TestStructural_Admit | Single-input wantPass case (list item under Dependencies is not an AC) that fits the existing Admit table as one row. |
| TestStructural_OnFailure | internal/guard | structural_test.go:372 | 0.04 | KEEP |  | Asserts the handler gets the exact context, its error is returned, and a nil handler yields ErrNoFailureHandler directly on the guard. |
| TestStructural_VerifyCheckRunsWhenAcceptanceCriteriaOptional | internal/guard | structural_test.go:415 | 0.05 | MERGE | TestStructural_CustomRequiredSections | Same custom RequiredSections=[Problem] setup; the Verify-still-runs case is a third assertion in that test. |
| TestIsKaironContext | internal/hotkey | detector_test.go:8 | 0.61 | DELETE |  | Line-for-line duplicate of TestHotkeyErrorHandling (unset/set KAIRON_WATCHER_PID). |
| TestIsCtrlOptionP | internal/hotkey | detector_test.go:25 | 0.04 | DELETE |  | Same tautology as TestIsCtrlOptionPDetection: compares literals, never calls the function under test. |
| TestIsCtrlOptionPDetection | internal/hotkey | error_handling_test.go:22 | 0.04 | DELETE |  | Compares string literals to "ctrl+alt+p" inside the test and never calls IsCtrlOptionP; it asserts only its own setup. |
| TestHotkeyIntegrationEndToEnd | internal/hotkey | integration_test.go:57 | 0.05 | DELETE |  | Drives a test-local mockTUIModel; HandleKeyMsg gets an empty KeyPressMsg (returns nil) so every assertion checks the mock's own switch. |
| TestHotkeyValidation | internal/hotkey | integration_test.go:314 | 0.04 | DELETE |  | Re-asserts IsKaironContext (dup of TestHotkeyErrorHandling) and that a locally built HotkeyErrorMsg has a non-nil Err; asserts its own setup. |
| TestCompleteWorkflow | internal/hotkey | integration_test.go:360 | 0.05 | DELETE |  | Mock-model mode flips plus SessionManager save/load already tested in internal/session; asserts the mock and duplicates session tests. |
| TestGenerateFilename | internal/incidents | logger_test.go:66 | 0.05 | DELETE |  | Prefix/suffix check is implied by TestIncidentLogger, which writes via generateFilename and parses issue 123/attempt 4 back through ListIncidents. |
| TestNewFileHandler | internal/logging | file_handler_test.go:12 | 0.57 | DELETE |  | Its file-created and debug-*.log checks duplicate TestFileHandler_FilenameFormat and TestFileHandler_DirectoryCreation. |
| TestFileHandler_Write | internal/logging | file_handler_test.go:53 | 0.06 | DELETE |  | A single-write subset of TestFileHandler_MultipleWrites (byte count, size, content). |
| TestFileHandler_FilenameFormat | internal/logging | file_handler_test.go:279 | 0.05 | DELETE |  | Pins debug-YYYY-MM-DD-HHMM.log, a format nothing in cmd/, internal/ or docs/ parses; incidental. |
| TestFileHandler_ConcurrentWrites | internal/logging | file_handler_test.go:324 | 0.06 | KEEP |  | Under -race this is the only check that FileHandler.Write is goroutine-safe; tighten the size assertion to the exact byte total. |
| TestFileHandler_RotationSequenceNumbers | internal/logging | file_handler_test.go:374 | 0.06 | MERGE | TestFileHandler_Rotation | Same-minute filename collision avoidance is the sequence-suffix code Rotation already exercises; fold in as a second case and assert exactly 3 files instead of >=2. |
| TestAddAndGet | internal/logging | ring_buffer_test.go:42 | 0.03 | KEEP |  | Basic insert-order/level/metadata contract of the ring buffer; TestConcurrentAccess covers the lines but not these values. |
| TestMetadataHandling | internal/logging | ring_buffer_test.go:385 | 0.04 | DELETE |  | Asserts a map[string]any stores string/int/bool/float unchanged (language behaviour); TestAddAndGet already checks metadata storage. |
| TestOddKeyVals | internal/logging | ring_buffer_test.go:417 | 0.05 | MERGE | TestNonStringKeys | Same keyvals parsing edge case table as TestNonStringKeys. |
| TestNonStringKeys | internal/logging | ring_buffer_test.go:438 | 0.05 | MERGE | TestOddKeyVals | Same keyvals-to-metadata parsing with a different malformed input; fold both into one table. |
| TestCleanupOnExit | internal/session | error_handling_test.go:99 | 0.04 | KEEP |  | Guards against CleanupOnExit (called from tui on exit) deleting fresh sessions; the covering test lives in internal/hotkey and should be the one removed. |
| TestSessionManagerCRUD | internal/session | manager_test.go:9 | 0.04 | KEEP |  | Core Create/Load/Save/List/Delete contract in-package; the subsuming test is the hotkey package's duplicate. |
| TestSessionManagerCleanup | internal/session | manager_test.go:74 | 0.04 | KEEP |  | Only in-package test that age-based Cleanup deletes a 25h-old session (via Chtimes) and keeps a fresh one; the 'partner' is in internal/hotkey. |
| TestSessionStateJSONMarshaling | internal/session | types_test.go:7 | 0.03 | MERGE | TestSessionManagerCRUD | CRUD already round-trips through Save/Load; add the type and role/content field checks there. |
| TestSessionTypes | internal/session | types_test.go:37 | 0.04 | DELETE |  | Asserts two string constants equal their literal values. |
| TestModeSwitchLogging | internal/session | validation_focused_test.go:331 | 0.04 | DELETE |  | LogModeSwitch is a documented no-op with no production caller (dead code); ValidateSessionFlow is also exercised by TestSessionDataIntegrityDuringModeSwitch. |
| TestNoInternalDeps | internal/stageoutput | deps_test.go:12 | 0.76 | KEEP |  | Keeps stageoutput a leaf package (no internal deps) to prevent import cycles; go list contract. |
| TestDocs_NamesRequiredTerms | internal/stageoutput | docs_test.go:25 | 0.04 | DELETE |  | Word grep over docs/stage-output.md (20 terms); pins wording. |
| TestDocs_ValidYAMLExamplesParse | internal/stageoutput | docs_test.go:42 | 0.05 | KEEP |  | Executes yaml examples in docs/stage-output.md and requires both canonical and punch_out examples. |
| TestDocs_InvalidExamplesAreRejected | internal/stageoutput | docs_test.go:62 | 0.04 | KEEP |  | Executes yaml-invalid doc examples through Parse; add a len(blocks)>0 check, since it currently passes vacuously with zero blocks. |
| TestMarshal_CanonicalKeyOrderAndBlockScalar | internal/stageoutput | marshal_test.go:13 | 0.04 | KEEP |  | Byte-exact canonical golden: key order, quoting, literal block, inferences. |
| TestMarshal_PunchOutShape | internal/stageoutput | marshal_test.go:42 | 0.06 | KEEP |  | Byte-exact golden for punch_out including no body key; the only punch_out wire pin. |
| TestMarshal_TopLevelKeyOrder | internal/stageoutput | marshal_test.go:63 | 0.05 | MERGE | TestMarshal_PunchOutShape | Only adds unresolved-before-inferences ordering; add an inference to the punch_out golden and drop the yaml.Node walk. |
| TestMarshal_MultilineBodyIsLiteralBlock | internal/stageoutput | marshal_test.go:80 | 0.04 | DELETE |  | `body: |` literal block is asserted byte-exact by the TestMarshal_CanonicalKeyOrderAndBlockScalar golden. |
| TestMarshal_BodyDecodesVerbatim | internal/stageoutput | marshal_test.go:119 | 0.04 | KEEP |  | Round-trip edge bodies (no trailing newline, trailing blank lines, trailing spaces) that the golden and RoundTrip fixtures don't include. |
| TestMarshal_QuotesStringsThatLookLikeOtherTypes | internal/stageoutput | marshal_test.go:142 | 0.03 | DELETE |  | `ref: "142"` and quoted generated are asserted byte-exact by the TestMarshal_CanonicalKeyOrderAndBlockScalar golden. |
| TestParse_RejectsUnknownKey | internal/stageoutput | parse_test.go:54 | 0.05 | KEEP |  | Strict KnownFields at every nesting level (source, inferences[i], unresolved[i]) and error classification, the schema's main safety property. |
| TestParse_BodyVerbatim | internal/stageoutput | parse_test.go:104 | 0.04 | KEEP |  | Parse-side check that YAML-looking body lines and indented/blank lines survive the block scalar verbatim. |
| TestParse_CanonicalExampleFromSpec | internal/stageoutput | parse_test.go:281 | 0.04 | MERGE | TestParse_PunchOutExampleFromSpec | Both parse a yaml fence from the live research doc; combine into one table that asserts parse success and Result, dropping exact-prose field pins that break on doc edits. |
| TestParse_PunchOutExampleFromSpec | internal/stageoutput | parse_test.go:296 | 0.04 | MERGE | TestParse_CanonicalExampleFromSpec | Same spec-fence parse pattern; its long exact Reason strings pin research-doc prose. |
| TestParse_UnquotedScalars | internal/stageoutput | parse_test.go:312 | 0.05 | KEEP |  | Tolerates agent output with unquoted int ref and timestamp, coercing them to strings; real LLM-output robustness. |
| TestEnumValid | internal/stageoutput | validate_test.go:229 | 0.05 | DELETE |  | TestValidate_Envelope already rejects empty/unknown stage, result and source kind and accepts every valid value. |
| TestUnknownVersionError | internal/stageoutput | validate_test.go:238 | 0.05 | DELETE |  | errors.Is/As and the Version field are asserted by TestParse_UnknownVersion on a real parse error. |
| TestVersionParsedFromJSON | internal/version | version_test.go:10 | 0.66 | KEEP |  | Only check that the embedded version.json parses into valid semver (catches a broken release bump). |
| TestPrereleaseConcatenation | internal/version | version_test.go:19 | 0.03 | DELETE |  | Same as TestEmptyPrerelease: tests a copy of the logic. If prerelease formatting matters, extract format(versionInfo) and table-test it. |
| TestEmptyPrerelease | internal/version | version_test.go:38 | 0.04 | DELETE |  | Re-implements init()'s concatenation inside the test and checks json.Marshal/Unmarshal plus string +; never runs production code. |
| TestParseDependencies | internal/watcher | dependencies_test.go:13 | 0.49 | KEEP |  | Core dependency-syntax table (formats, case, dedupe, malformed); the fenced-code test covers the lines but not these formats. |
| TestStripFencedCode | internal/watcher | dependencies_test.go:313 | 0.04 | KEEP |  | Direct helper contract (sentinel replacement, line count preserved, CRLF bytes intact, unclosed fence) that the parser-level fenced tests don't assert. |
| TestValidateIssueCyclicDependencyDoesNotFetchChain | internal/watcher | dependencies_test.go:586 | 0.05 | MERGE | TestValidateIssue | Same assertions (lookups, IsValid, unresolved, no circular) as TestValidateIssue's table; add the two cycle bodies as rows. |
| TestCheckIssues_NotAdmittedUntracksAndRefundsRetryCount | internal/watcher | watcher_boundary_test.go:30 | 0.05 | MERGE | TestCheckIssues_AdmissionUnavailableUntracksAndRefunds | Same assertions (untracked, refunded to 0, file removed) with a different sentinel error; one table over {ErrNotAdmitted, ErrAdmissionUnavailable}. |
| TestCheckIssues_SuccessfulSpawnStaysTracked | internal/watcher | watcher_boundary_test.go:92 | 0.04 | MERGE | TestCheckIssues_OtherSpawnErrorStaysTracked | Identical newSpawnWatcher setup and assertions (tracked=[7], retry=1); differs only in spawn error, so one table. |
| TestReconcilePending_CallsReconcileAndResetsRetryCount | internal/watcher | watcher_reconcile_test.go:71 | 0.04 | KEEP |  | Asserts reconcile is called per pending issue with repo/fail label and that retry files are removed; the partner never inspects these. |

### Counts

| verdict | count | total secs |
|---|---|---|
| KEEP | 45 | 5.59 |
| MERGE | 20 | 0.95 |
| REWRITE-CHEAPER | 1 | 0.05 |
| DELETE | 28 | 2.29 |
| total | 94 | 8.88 |

Note on secs: in each package, one test shows 0.5-0.76s while its neighbours show about 0.04s (TestAdmit_ReadsFromGitHub..., TestSelectIssues_ExcludesTerminalPending, TestIsKaironContext, TestNewFileHandler, TestNoInternalDeps, TestVersionParsedFromJSON, TestParseDependencies). The test bodies don't account for this. It looks like per-binary warm-up in the measurement, so deleting these tests won't save that time. Only the go-list deps tests do real extra work (about 0.15-0.2s each).

### Recurring patterns

1. **Tests that never call the code under test.** They assert literals, mocks, or a copy of the production logic written into the test. Examples: TestIsCtrlOptionP and TestIsCtrlOptionPDetection compare `"ctrl+alt+p" == "ctrl+alt+p"`. TestPrereleaseConcatenation and TestEmptyPrerelease re-implement init(). TestHotkeyIntegrationEndToEnd and TestCompleteWorkflow drive a test-local mockTUIModel. As a result, the real hotkey.HandleKeyMsg (used in tui.go) has no test at all. One real test with a ctrl+alt+p KeyPressMsg, in and out of context, would replace all 6 hotkey candidates.
2. **Docs "required terms" greps.** TestDocs_NamesRequiredTerms in boundary, guard and stageoutput. They pin wording, while the sibling docs tests that run YAML examples (TestDocs_InputExamplesParse, TestDocs_ValidYAMLExamplesAreAdmitted, TestDocs_ValidYAMLExamplesParse) are the real doc contract. Separately, TestDocs_InvalidExamplesAreRejected in stageoutput passes vacuously when there are no blocks.
3. **Single-case tests that belong in an existing table.** The guard Structural cluster (TestStructural_MissingSectionMessages, TestStructural_ChecksEveryAcceptanceCriteriaSection, TestStructural_AcceptanceCriteriaSectionEndsAtNextHeading and others) maps onto TestStructural_Admit. The watcher spawn outcomes (TestCheckIssues_SuccessfulSpawnStaysTracked, TestCheckIssues_NotAdmittedUntracksAndRefundsRetryCount) differ only in the spawn error. TestValidateIssueCyclicDependencyDoesNotFetchChain maps onto TestValidateIssue.
4. **Restating declarations or constants.** TestGuard_InterfaceShape (reflection on an interface), TestSessionTypes (string constants), TestErrors_EmptyChainMatchesInvalid and TestUnknownVersionError (sentinel wrapping already asserted on real errors), TestDefaultChain_Order (len==1). For TestIssueStoreImplementsProvenanceStore, a package-level `var _ I = T{}` is the right form.
5. **Substring checks already covered by a byte-exact golden or round trip.** TestMarshal_MultilineBodyIsLiteralBlock and TestMarshal_QuotesStringsThatLookLikeOtherTypes are covered by the canonical golden. TestFileHandler_Write and TestNewFileHandler are covered by MultipleWrites, FilenameFormat and DirectoryCreation. TestGenerateFilename is covered by TestIncidentLogger's write-then-parse round trip.

Caveat on the coverage flag: several `subsumed-by` partners are in another package or are a broader end-to-end test, for example the session tests flagged against internal/hotkey's TestSessionIntegration. In those cases the in-package unit test is the one to keep, and the cross-package duplicate in internal/hotkey is the one to remove.
