package planner

// Agent names and the shared skill. These are the only mode-dependent values
// in the package: a call site that knows its mode passes the matching name,
// and nothing here maps a mode to an agent.
const (
	// AgentInteractive is the agent a person runs to plan a new issue.
	AgentInteractive = "planner"
	// AgentAutomated is the agent the watcher runs to canonicalize an
	// existing issue without a human present.
	AgentAutomated = "planner-automated"
	// SharedSkill is the Kairon-managed skill both agents load. It holds
	// everything that is the same for both: the output contract,
	// restrictions, the issue-body standard and the hard bars.
	SharedSkill = "planner-shared"
)
