// Package budget provides token usage estimation and cost tracking for agent teams.
//
// TODO: Implement the following:
//
// tracker.go — Token usage + cost estimation
//   - NewTracker(budgetLimit float64) → *Tracker
//   - EstimateTokens(model string, durationSeconds int64) → int64
//   - EstimateCost(model string, inputTokens, outputTokens int64) → float64
//   - RecordSession(model string, durationSeconds int64)
//   - GetTotalCost() → float64
//   - IsOverBudget() → bool
//
// Cost rates (as of April 2026):
//
//	Opus 4.6:   $5/M input,  $25/M output
//	Sonnet 4.6: $3/M input,  $15/M output
//
// Estimation heuristic:
//   - Assume ~50K input tokens per minute of active session (model loading context)
//   - Assume ~5K output tokens per minute (code generation)
//   - These are rough estimates; real usage varies heavily by task
package budget
