// Package claude provides helpers for interacting with Claude Code sessions,
// mailbox files, task lists, and git worktrees inside Kubernetes pods.
//
// TODO: Implement the following:
//
// session.go — Session lifecycle management
//   - BuildSessionArgs(model, prompt, permissionMode) → []string
//   - WaitForSessionComplete(ctx, podName) → error
//   - GetSessionLogs(ctx, podName) → string
//
// mailbox.go — Mailbox JSON I/O (reads/writes ~/.claude/teams/{team}/inboxes/{agent}.json)
//   - ReadInbox(teamName, agentName) → []Message
//   - SendMessage(teamName, from, to, text) → error
//   - WatchInbox(ctx, teamName, agentName) → <-chan Message
//
// tasklist.go — Task list JSON I/O (reads/writes ~/.claude/tasks/{team}/)
//   - ReadTaskList(teamName) → []Task
//   - GetTaskSummary(teamName) → TaskSummary
//   - IsTeamComplete(teamName) → bool
//   - WatchTasks(ctx, teamName) → <-chan TaskEvent
//
// worktree.go — Git worktree management
//   - CloneRepo(url, branch, destPath) → error
//   - CreateWorktree(repoPath, teammateName, baseBranch) → string
//   - MergeWorktrees(repoPath, worktrees, targetBranch) → error
//   - CleanupWorktrees(repoPath) → error
package claude
