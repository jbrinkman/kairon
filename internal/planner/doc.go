// Package planner holds the planner's identity and its output boundary.
//
// The planner is two single-mode agents, not one agent with a mode switch:
// AgentInteractive elicits requirements from a person and AgentAutomated
// canonicalizes an existing issue with nobody to ask. Mode is a fact the
// caller already knows, so the caller picks the agent name and neither agent
// prompt branches on it. Everything the two agents share (the output
// contract, restrictions, the issue-body standard and the hard bars) lives in
// one Kairon-managed skill, SharedSkill, which both agent manifests load
// through their resources.
//
// The agent never writes to GitHub. It returns a stage-output document and a
// deterministic boundary validates, hashes and commits it. That boundary is
// glue over internal/stageoutput, internal/guard, internal/provenance and
// internal/failure; this package adds no contract of its own.
package planner
