// Package webhook sends event notifications to external services (Slack, etc.).
//
// TODO: Implement the following:
//
// notifier.go — Webhook notifications
//   - NewNotifier(url string, events []string) → *Notifier
//   - SendEvent(ctx, eventType string, payload map[string]interface{}) → error
//
// Event types:
//   - "team.started"     — Team has been created and is initializing
//   - "team.running"     — All agents are deployed and working
//   - "task.completed"   — A task from the shared list was completed
//   - "teammate.error"   — A teammate pod crashed or entered error state
//   - "teammate.idle"    — A teammate has no more work to claim
//   - "budget.warning"   — 80% of budget consumed
//   - "budget.exceeded"  — Budget limit hit, team terminated
//   - "team.completed"   — All work done, PR created if configured
//   - "team.failed"      — Team failed (quality gates, errors, etc.)
//   - "team.timedout"    — Team exceeded timeout
//
// Payload format (JSON):
//   {
//     "event": "task.completed",
//     "team": "auth-refactor",
//     "namespace": "dev-agents",
//     "timestamp": "2026-04-03T14:30:00Z",
//     "data": { ... event-specific fields ... }
//   }
package webhook
