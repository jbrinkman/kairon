package inference

// CallRecord is one inference call, in the shape shared by eval runs and
// (Stage 4) production workflow audit trails.
type CallRecord struct {
	Role         string  `json:"role"`                // "agent" | "judge"
	Model        string  `json:"model"`               // served model when the backend reports one, else the pinned (requested) model
	Agent        string  `json:"agent,omitempty"`     // agent calls
	Criterion    string  `json:"criterion,omitempty"` // judge calls
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Estimated    bool    `json:"estimated"` // true unless the usage source is "reported"
	DurationMS   int64   `json:"duration_ms"`
	PromptSHA256 string  `json:"prompt_sha256,omitempty"` // agent calls only
	Error        string  `json:"error,omitempty"`         // call failed; the record is still written
	// TrustedTools is the whole-tool trust set the call ran with. A pointer so
	// a restricted-to-nothing set ([]) is distinct from unrestricted (absent).
	TrustedTools *[]string `json:"trusted_tools,omitempty"`
	// ToolDenials lists tool calls refused by the trust gate (stub backend only).
	ToolDenials []ToolDenial `json:"tool_denials,omitempty"`
}
