package inference

// SourceWorkspace is the CallRecord.Source of a record imported from an agent
// workspace (e.g. .eval/calls.jsonl) rather than made by the harness itself.
const SourceWorkspace = "workspace"

// CallRecord is one inference call, in the shape shared by eval runs and
// (Stage 4) production workflow audit trails.
type CallRecord struct {
	Role      string `json:"role"`                // "agent" | "judge"
	Model     string `json:"model"`               // served model when the backend reports one, else the pinned (requested) model
	Agent     string `json:"agent,omitempty"`     // agent calls
	Criterion string `json:"criterion,omitempty"` // judge calls
	// Purpose says why a judge call was made when it is not a criterion check
	// (e.g. "simulated_user" for routing). Omitted when empty.
	Purpose string `json:"purpose,omitempty"`
	// Turn is the 1-based user turn of a multi-turn agent call (Request.Turn+1).
	// Zero, and therefore omitted, for single-turn calls and judge calls.
	Turn int `json:"turn,omitempty"`
	// Step names the workflow step an imported record belongs to (e.g.
	// "design"). Omitted when empty.
	Step string `json:"step,omitempty"`
	// Source says who wrote the record: SourceWorkspace for records imported
	// from the agent workspace, empty (omitted) for records the harness made.
	Source       string  `json:"source,omitempty"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	// CostUnknown is true when CostUSD is 0 only because the model has no
	// configured price. Such a CostUSD must not be summed as a real price.
	// Omitted when false, so records written before this field read as known.
	CostUnknown  bool   `json:"cost_unknown,omitempty"`
	Estimated    bool   `json:"estimated"` // true unless the usage source is "reported"
	DurationMS   int64  `json:"duration_ms"`
	PromptSHA256 string `json:"prompt_sha256,omitempty"` // agent calls only
	Error        string `json:"error,omitempty"`         // call failed; the record is still written
	// TrustedTools is the whole-tool trust set the call ran with. A pointer so
	// a restricted-to-nothing set ([]) is distinct from unrestricted (absent).
	TrustedTools *[]string `json:"trusted_tools,omitempty"`
	// ToolDenials lists tool calls refused by the trust gate (stub backend only).
	ToolDenials []ToolDenial `json:"tool_denials,omitempty"`
}
