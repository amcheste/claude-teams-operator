# Product Requirements Document: Knowledge Work Orchestrator

**Document owner:** Alan Chester
**Last updated:** 2026-05-12
**Status:** Draft
**Linear milestone:** v0.8.0 — Knowledge Work Orchestrator
**Target date:** 2026-08-15

---

## Overview

This PRD defines the requirements for transforming claude-teams-operator from a coding-focused agent orchestrator into a Kubernetes-native knowledge work platform. All features described here are additive and backward-compatible with existing AgentTeam CRs.

## Personas

**Platform Engineer (Pat).** Manages K8s clusters, deploys operators, configures RBAC and observability. Pat cares about: Helm values, CRD stability, Prometheus metrics, resource limits, and upgrade paths. Pat evaluates this operator the same way they evaluate any K8s operator.

**Operations Lead (Olivia).** Defines recurring business processes and ensures they run reliably. Olivia cares about: scheduling, cost visibility, approval workflows, and delivery confirmation. Olivia interacts primarily through templates and kubectl/dashboard.

**Knowledge Worker (Kim).** Consumes AI-generated deliverables. Kim does not use kubectl. Kim receives reports in Slack, reviews drafts via approval links, and finds documents in Google Drive. Kim's experience is the approval gate notification and the delivered artifact.

---

## Feature Requirements

### F1. README and Positioning Reframe

**Linear:** AMC-129
**Priority:** High
**Persona:** All (first impression)

**Requirement:** The project's public-facing materials position knowledge work orchestration as the primary use case, with coding as a supported secondary mode.

**User stories:**
- As Pat evaluating the project, I understand within 30 seconds that this is a knowledge work platform, not just a coding tool.
- As Olivia reading the README, the first example I see matches my use case (reports, analysis, communications), not a coding task.

**Acceptance criteria:**
- README leads with Cowork example
- Repo description updated
- No functional code changes

---

### F2. Output Routing Between Teammates

**Linear:** AMC-130
**Priority:** High
**Persona:** Olivia, Pat

**Requirement:** Teammates can declare output artifacts and downstream teammates can declare input dependencies on those artifacts. The operator automatically routes outputs to inputs when upstream teammates complete.

**User stories:**
- As Olivia, I define a pipeline where a researcher produces findings.md and a writer consumes it, without manually copying files between pods.
- As Pat, I can see in the AgentTeam status which artifacts have been produced and by whom.

**Acceptance criteria:**
- TeammateSpec supports `outputs []OutputSpec` and `inputs []InputSpec`
- Operator copies outputs to downstream input mount paths on completion
- `status.artifacts` tracks produced artifacts with metadata
- Missing declared outputs surface as an error in teammate status
- Sample CR demonstrates the pattern

**Non-functional:**
- File copy latency under 5 seconds for artifacts under 100MB
- Graceful handling of large artifacts (log warning over 500MB)

---

### F3. Pipeline Stages

**Linear:** AMC-131
**Priority:** High
**Persona:** Olivia, Pat

**Requirement:** An alternative to flat `dependsOn` that models multi-stage workflows with explicit fan-out (parallel) and merge (wait-for-all) semantics.

**User stories:**
- As Olivia, I define a 4-stage pipeline (research, analysis, synthesis, distribution) where the analysis stage fans out three analysts in parallel and the synthesis stage waits for all three to complete.
- As Pat, I can see in `kubectl describe` which pipeline stage is currently active and how many stages are complete.

**Acceptance criteria:**
- `spec.pipeline` with `stages []StageSpec` supported
- `fan: parallel` starts all teammates in stage simultaneously
- `fan: merge` waits for all teammates in dependent stages to Succeed
- `approvalRequired` on a stage blocks the entire stage until annotation
- `spec.pipeline` and `spec.teammates[].dependsOn` are mutually exclusive (validation)
- `status.pipeline` tracks current stage and per-stage timing

**Non-functional:**
- Stage transition latency under 30 seconds after all dependencies met
- Maximum 20 stages per pipeline (validation)

---

### F4. Scheduled Teams (AgentTeamSchedule)

**Linear:** AMC-132
**Priority:** High
**Persona:** Olivia, Pat

**Requirement:** A new CRD that creates AgentTeam instances on a cron schedule, enabling recurring knowledge work.

**User stories:**
- As Olivia, I define a weekly report team that runs every Monday at 6am without manual intervention.
- As Pat, I can see the schedule status (lastScheduledAt, nextScheduledAt) and garbage-collect old completed runs.

**Acceptance criteria:**
- `AgentTeamSchedule` CRD with cron `schedule` field
- Reconciler creates AgentTeam/Run on schedule
- `historyLimit` garbage-collects completed runs
- Idempotent: no duplicate runs for same window
- Status tracks schedule metadata and run history

**Non-functional:**
- Schedule drift under 60 seconds from target time
- `robfig/cron` for parsing (standard K8s ecosystem library)

---

### F5. Result Delivery

**Linear:** AMC-133
**Priority:** Medium
**Persona:** Olivia, Kim

**Requirement:** Extend `onComplete` to support delivering artifacts to Slack, email, Google Drive, and webhooks.

**User stories:**
- As Olivia, I configure a team to deliver its final report to a Slack channel and upload it to Google Drive when complete.
- As Kim, I receive the Q3 report in my Slack DMs without logging into any other system.

**Acceptance criteria:**
- `onComplete: deliver` with `delivery []DeliverySpec`
- Webhook delivery working
- Slack delivery via webhook URL
- Email delivery via SMTP
- Google Drive delivery via service account
- Delivery failures reported in status but don't fail the team
- Each delivery type implemented in separate package under `internal/delivery/`

**Non-functional:**
- Delivery attempts with 3 retries and exponential backoff
- Delivery timeout of 60 seconds per target

---

### F6. OCI Skill Distribution

**Linear:** AMC-134
**Priority:** Medium
**Persona:** Pat

**Requirement:** Pull skills from OCI registries instead of (or in addition to) ConfigMaps, enabling a marketplace model for skill distribution.

**User stories:**
- As Pat, I pull a published financial-analysis skill from ghcr.io without manually creating ConfigMaps.
- As Pat, I can use private registry skills via imagePullSecrets.

**Acceptance criteria:**
- `source.oci` field on SkillSourceSpec
- Init container pulls OCI artifact and mounts to skill path
- ConfigMap skills still work (backward compatible)
- Private registry support via imagePullSecrets
- Skill caching (skip re-pull if present)

**Non-functional:**
- Skill pull timeout of 120 seconds
- Total skill size limit of 50MB per teammate

---

### F7. Event-Triggered Teams (AgentTeamTrigger)

**Linear:** AMC-135
**Priority:** Medium
**Persona:** Olivia, Pat

**Requirement:** A new CRD that creates AgentTeam instances in response to webhook events.

**User stories:**
- As Olivia, I configure a team that fires when a new deal closes, automatically generating onboarding documents.
- As Pat, I can see trigger history and enforce concurrency limits.

**Acceptance criteria:**
- `AgentTeamTrigger` CRD with webhook trigger type
- HMAC validation for webhook security
- Trigger payload injected as ConfigMap input
- `concurrencyPolicy` enforced (Allow, Forbid, Replace)
- Status tracks trigger history

**Non-functional:**
- Webhook response within 5 seconds (create CR async, respond 202)
- Maximum 10 concurrent triggered runs per trigger

---

### F8. Pipeline-Aware Observability

**Linear:** AMC-136
**Priority:** Low
**Persona:** Pat

**Requirement:** Add pipeline stage metrics, artifact tracking, and delivery metrics to Prometheus and the operator dashboard.

**User stories:**
- As Pat, I see pipeline stage duration distributions in Grafana to identify bottleneck stages.
- As Pat, I see artifact production counts and delivery success rates.

**Acceptance criteria:**
- New Prometheus metrics for stages, artifacts, and delivery
- Dashboard pipeline visualization (stage progress bar)
- `kubectl describe` shows pipeline summary
- Artifact list in team detail view

---

### F9. Product Documentation

**Linear:** AMC-137
**Priority:** High
**Persona:** All

**Requirement:** Create Product Vision, PRD, and Technical Design documents in `docs/` before implementation begins.

**Acceptance criteria:**
- Three documents committed to `docs/`
- Cross-referenced with Linear issues
- ARCHITECTURE.md updated to reference new docs

---

## Phasing

**Ships in v0.8.0:**
- F1 (positioning reframe)
- F2 (output routing)
- F3 (pipeline stages)
- F4 (AgentTeamSchedule)
- F5 (result delivery)
- F9 (documentation)

**Ships in v0.9.0 or later:**
- F6 (OCI skills) — depends on OCI tooling maturity
- F7 (AgentTeamTrigger) — depends on webhook server design
- F8 (pipeline observability) — depends on F3 being stable

## Dependencies and Risks

| Risk | Mitigation |
|------|------------|
| Pipeline complexity: stages + output routing + approval gates interact | Implement F2 (output routing) before F3 (pipelines). Output routing is useful standalone. |
| AgentTeamSchedule adds a second reconciler | Follow CronJob controller pattern exactly. Reuse existing AgentTeam/Run creation. |
| Delivery targets require external credentials (SMTP, Drive, Slack) | Each delivery type is optional and independently configured. Webhook is the simplest and ships first. |
| OCI skill pulls add init container complexity | Ship ConfigMap skills first (already working). OCI is additive. |
| Backward compatibility with existing AgentTeam CRs | All new fields are optional. Existing CRs must work without modification. Validation tests cover this. |

## Related Documents

- [Product Vision](./product-vision.md)
- [Knowledge Work Design](./knowledge-work-design.md)
- [ARCHITECTURE.md](../ARCHITECTURE.md)
