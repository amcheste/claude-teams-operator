// Package metrics exposes Prometheus metrics for Claude Agent Teams.
//
// TODO: Implement the following:
//
// prometheus.go — Metrics registration and collection
//   - RegisterMetrics() — register all metrics with prometheus
//   - RecordTeamStart(teamName, namespace string)
//   - RecordTeamComplete(teamName, namespace string, durationSec float64)
//   - RecordTeamFailed(teamName, namespace string)
//   - RecordTokenUsage(teamName, teammate, model string, tokens int64)
//   - RecordCost(teamName, namespace string, costUSD float64)
//   - RecordTaskCompleted(teamName, teammate string)
//   - RecordTeammateRestart(teamName, teammate string)
//   - SetBudgetRemaining(teamName, namespace string, remaining float64)
//   - SetActiveTeams(count int)
//
// Metrics:
//
//	claude_team_active_total              (Gauge)
//	claude_team_duration_seconds          (Histogram)   labels: team, namespace
//	claude_teammate_tokens_total          (Counter)     labels: team, teammate, model
//	claude_team_cost_usd                  (Gauge)       labels: team, namespace
//	claude_team_tasks_completed_total     (Counter)     labels: team, teammate
//	claude_teammate_restarts_total        (Counter)     labels: team, teammate
//	claude_team_budget_remaining_usd      (Gauge)       labels: team, namespace
//	claude_teammate_idle_seconds          (Histogram)   labels: team, teammate
package metrics
