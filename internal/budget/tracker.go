package budget

import "sync"

// Cost rates for April 2026, sourced from doc.go. Rates are USD per million
// tokens, split between input and output as billed by the Anthropic API.
const (
	opusInputPerMillion    = 5.0
	opusOutputPerMillion   = 25.0
	sonnetInputPerMillion  = 3.0
	sonnetOutputPerMillion = 15.0
)

// Session heuristic: ~50K input + ~5K output tokens per minute of active
// session. These numbers are deliberately rough — real usage varies heavily
// by task — but they let the operator produce cost estimates without a
// feedback channel from Claude Code itself.
const (
	inputTokensPerMinute  int64 = 50_000
	outputTokensPerMinute int64 = 5_000
	totalTokensPerMinute        = inputTokensPerMinute + outputTokensPerMinute

	modelOpus   = "opus"
	modelSonnet = "sonnet"
)

type costRate struct {
	inputPerMillion, outputPerMillion float64
}

func rateFor(model string) costRate {
	switch model {
	case modelOpus:
		return costRate{inputPerMillion: opusInputPerMillion, outputPerMillion: opusOutputPerMillion}
	default:
		// Sonnet is the fallback for any unrecognized model so operators get a
		// conservative estimate rather than a zero.
		return costRate{inputPerMillion: sonnetInputPerMillion, outputPerMillion: sonnetOutputPerMillion}
	}
}

// EstimateTokens returns the estimated total (input + output) tokens for a
// session of the given duration. Non-positive durations return 0. The model
// argument is accepted for API symmetry with EstimateCost; the heuristic is
// model-agnostic.
func EstimateTokens(model string, durationSeconds int64) int64 {
	_ = model
	if durationSeconds <= 0 {
		return 0
	}
	return totalTokensPerMinute * durationSeconds / 60
}

// EstimateCost returns the USD cost for explicit input and output token counts
// at the given model's rates. Unknown models fall back to Sonnet pricing.
func EstimateCost(model string, inputTokens, outputTokens int64) float64 {
	r := rateFor(model)
	return float64(inputTokens)/1_000_000*r.inputPerMillion +
		float64(outputTokens)/1_000_000*r.outputPerMillion
}

// Tracker accumulates session costs across agents in a team and compares the
// running total against a configured budget limit. Tracker is safe for
// concurrent use.
type Tracker struct {
	mu          sync.Mutex
	budgetLimit float64
	totalCost   float64
}

// NewTracker returns a Tracker with the given USD budget limit. A limit of
// zero (or negative) disables the over-budget check — IsOverBudget always
// returns false.
func NewTracker(budgetLimit float64) *Tracker {
	return &Tracker{budgetLimit: budgetLimit}
}

// RecordSession adds the estimated cost of a session of the given duration
// for the named model to the tracker's running total. Non-positive durations
// are a no-op. Tokens are split between input and output using the
// 50K/5K-per-minute heuristic.
func (t *Tracker) RecordSession(model string, durationSeconds int64) {
	if durationSeconds <= 0 {
		return
	}
	input := inputTokensPerMinute * durationSeconds / 60
	output := outputTokensPerMinute * durationSeconds / 60
	cost := EstimateCost(model, input, output)

	t.mu.Lock()
	defer t.mu.Unlock()
	t.totalCost += cost
}

// GetTotalCost returns the accumulated USD cost across all recorded sessions.
func (t *Tracker) GetTotalCost() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.totalCost
}

// IsOverBudget reports whether the accumulated cost has reached or exceeded
// the configured budget limit. Returns false when no budget is configured
// (limit <= 0).
func (t *Tracker) IsOverBudget() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.budgetLimit <= 0 {
		return false
	}
	return t.totalCost >= t.budgetLimit
}
