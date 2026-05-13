# Technical Design: Knowledge Work Orchestrator Extensions

**Document owner:** Alan Chester
**Last updated:** 2026-05-12
**Status:** Draft
**Linear milestone:** v0.8.0 — Knowledge Work Orchestrator

---

## Scope

This document describes the technical design for extending claude-teams-operator with knowledge work primitives: output routing, pipeline stages, scheduled teams, result delivery, OCI skills, and event triggers. All extensions are backward-compatible with existing AgentTeam CRs.

## Architecture Changes from Current State

The current architecture (documented in ARCHITECTURE.md) has a single reconciler (`AgentTeamReconciler`) managing the full lifecycle of AgentTeam CRs. The knowledge work extensions add:

1. New fields on the existing `AgentTeamSpec` (output routing, pipelines, delivery)
2. A new reconciler for `AgentTeamSchedule`
3. A new reconciler for `AgentTeamTrigger`
4. A delivery subsystem (`internal/delivery/`)
5. An OCI skill pull mechanism (init container)

```
                        ┌──────────────────────────────┐
                        │     claude-teams-operator     │
                        ├──────────────────────────────┤
                        │ AgentTeamReconciler           │ ← existing + output routing
                        │   + pipeline stage logic      │   + pipeline stages
                        │   + artifact tracking         │   + delivery dispatch
                        │   + delivery dispatch         │
                        ├──────────────────────────────┤
                        │ AgentTeamScheduleReconciler   │ ← new
                        │   + cron evaluation           │
                        │   + run creation              │
                        │   + history garbage collection│
                        ├──────────────────────────────┤
                        │ AgentTeamTriggerReconciler    │ ← new (v0.9.0)
                        │   + webhook server            │
                        │   + payload injection         │
                        ├──────────────────────────────┤
                        │ internal/delivery/            │ ← new
                        │   webhook.go                  │
                        │   slack.go                    │
                        │   email.go                    │
                        │   gdrive.go                   │
                        └──────────────────────────────┘
```

---

## 1. Output Routing

### CRD Types

```go
// OutputSpec declares a file this teammate will produce.
type OutputSpec struct {
    // Path is the absolute path where the output will be written.
    Path string `json:"path"`
    // Description is a human-readable description of the artifact.
    // +optional
    Description string `json:"description,omitempty"`
}

// InputSpec declares a dependency on another teammate's output.
type InputSpec struct {
    // From is the name of the upstream teammate.
    From string `json:"from"`
    // Artifact is the filename to consume from the upstream output.
    Artifact string `json:"artifact"`
    // MountPath is where the artifact will be available in this pod.
    MountPath string `json:"mountPath"`
}

// ArtifactStatus records a produced artifact.
type ArtifactStatus struct {
    Name       string      `json:"name"`
    ProducedBy string      `json:"producedBy"`
    Size       string      `json:"size"`
    ProducedAt metav1.Time `json:"producedAt"`
    Path       string      `json:"path"`
}
```

### Reconciler Behavior

In `reconcileRunning`, after detecting a teammate pod has Succeeded:

```
1. List declared outputs for the completed teammate
2. For each output:
   a. Exec into a utility pod (or the lead pod) to stat the file
   b. Record in status.artifacts: {name, producedBy, size, producedAt, path}
3. For each other teammate with inputs[].from == completed teammate:
   a. Copy (or symlink) the artifact to inputs[].mountPath
   b. Mark the input as satisfied
4. Check if downstream teammates have all inputs satisfied AND dependsOn met
5. If yes, spawn the downstream teammate
```

### File Operations

The operator needs to inspect and copy files on the shared PVC. Two approaches:

**Option A: kubectl exec into lead pod.** The lead pod is always running during the team lifecycle. The operator can exec `cp` and `stat` commands. Pro: no additional pods. Con: couples file operations to lead pod health.

**Option B: Short-lived utility Job.** Spawn a lightweight busybox job that mounts the PVC, copies files, and exits. Pro: decoupled from agent pods. Con: latency of Job creation.

**Recommendation:** Option A for v0.8.0 (simpler), with Option B as fallback if the lead has terminated.

---

## 2. Pipeline Stages

### CRD Types

```go
// PipelineSpec defines a multi-stage workflow.
type PipelineSpec struct {
    // Stages defines the ordered stages of the pipeline.
    // +kubebuilder:validation:MinItems=2
    // +kubebuilder:validation:MaxItems=20
    Stages []StageSpec `json:"stages"`
}

// StageSpec defines a single pipeline stage.
type StageSpec struct {
    // Name is the unique identifier for this stage.
    Name string `json:"name"`
    // Teammates lists the teammate names that execute in this stage.
    // +kubebuilder:validation:MinItems=1
    Teammates []string `json:"teammates"`
    // DependsOn lists stage names that must complete before this stage starts.
    // +optional
    DependsOn []string `json:"dependsOn,omitempty"`
    // Fan controls how teammates in this stage are started.
    // +kubebuilder:validation:Enum=parallel;merge
    // +kubebuilder:default="parallel"
    Fan string `json:"fan,omitempty"`
    // ApprovalRequired blocks this stage until a human approves.
    // +optional
    ApprovalRequired bool `json:"approvalRequired,omitempty"`
}

// PipelineStatus reports pipeline execution state.
type PipelineStatus struct {
    CurrentStage    string        `json:"currentStage,omitempty"`
    StagesCompleted int           `json:"stagesCompleted"`
    StagesTotal     int           `json:"stagesTotal"`
    Stages          []StageStatus `json:"stages,omitempty"`
}

// StageStatus reports a single stage's state.
type StageStatus struct {
    Name            string      `json:"name"`
    Phase           string      `json:"phase"` // Pending, Waiting, Running, Completed, Failed
    StartedAt       *metav1.Time `json:"startedAt,omitempty"`
    CompletedAt     *metav1.Time `json:"completedAt,omitempty"`
    DurationSeconds int64       `json:"durationSeconds,omitempty"`
    TeammatesReady  string      `json:"teammatesReady,omitempty"` // "2/3"
}
```

### Stage Transition Logic

```
func evaluateStages(team, currentStatus):
    for each stage in spec.pipeline.stages:
        if stage already Completed: continue

        // Check dependencies
        allDepsMet = true
        for dep in stage.dependsOn:
            depStage = findStageStatus(dep)
            if stage.fan == "merge":
                // ALL teammates in dep stage must be Succeeded
                if depStage.phase != "Completed": allDepsMet = false
            else:
                // At least one teammate completed (for data availability)
                if depStage.phase not in ["Running", "Completed"]: allDepsMet = false

        if not allDepsMet: mark stage Pending, continue

        // Check approval gate
        if stage.approvalRequired:
            annotation = "approved.claude.amcheste.io/stage-{stage.name}"
            if annotation not present: mark stage Waiting, continue

        // Deploy teammates for this stage
        for teammate in stage.teammates:
            if not alreadyDeployed(teammate):
                deployTeammatePod(teammate)

        mark stage Running
        break  // Only one stage transitions per reconcile loop
```

### Validation

A validating webhook rejects CRs where:
- `spec.pipeline` is set AND any teammate has `dependsOn` set
- A stage references a teammate name not in `spec.teammates`
- Stage dependencies form a cycle
- A stage depends on itself

---

## 3. AgentTeamSchedule

### CRD Types

```go
// AgentTeamScheduleSpec defines a scheduled team pattern.
type AgentTeamScheduleSpec struct {
    // Schedule is a cron expression (5-field, no seconds).
    Schedule string `json:"schedule"`
    // TemplateRef references an AgentTeamTemplate.
    TemplateRef TemplateReference `json:"templateRef"`
    // Auth configures API authentication.
    Auth AuthSpec `json:"auth"`
    // Workspace or Repository configuration.
    // +optional
    Workspace *WorkspaceSpec `json:"workspace,omitempty"`
    // +optional
    Repository *RepositorySpec `json:"repository,omitempty"`
    // Lifecycle overrides.
    // +optional
    Lifecycle *LifecycleSpec `json:"lifecycle,omitempty"`
    // HistoryLimit is the number of completed runs to retain.
    // +kubebuilder:default=5
    HistoryLimit int32 `json:"historyLimit,omitempty"`
}

// AgentTeamScheduleStatus reports schedule state.
type AgentTeamScheduleStatus struct {
    LastScheduledAt *metav1.Time `json:"lastScheduledAt,omitempty"`
    NextScheduledAt *metav1.Time `json:"nextScheduledAt,omitempty"`
    ActiveRun       string       `json:"activeRun,omitempty"`
    TotalRuns       int32        `json:"totalRuns"`
    Conditions      []metav1.Condition `json:"conditions,omitempty"`
}
```

### Reconciler

```
func (r *AgentTeamScheduleReconciler) Reconcile(ctx, req):
    1. Fetch AgentTeamSchedule CR
    2. Parse cron schedule with robfig/cron
    3. Compute nextScheduledAt from lastScheduledAt
    4. If now >= nextScheduledAt AND no activeRun:
       a. Create AgentTeamRun from templateRef + overrides
       b. Set status.activeRun = run name
       c. Set status.lastScheduledAt = now
       d. Increment status.totalRuns
    5. If activeRun exists, check if it's terminal:
       a. If terminal, clear activeRun
    6. Garbage-collect: list runs owned by this schedule,
       delete oldest beyond historyLimit
    7. Compute and set status.nextScheduledAt
    8. Requeue at nextScheduledAt
```

### Dependencies

- `robfig/cron/v3` for cron parsing
- Owner references from Schedule -> Run for garbage collection

---

## 4. Result Delivery

### Package Structure

```
internal/delivery/
    delivery.go     // Interface + dispatcher
    webhook.go      // HTTP POST with artifact
    slack.go        // Slack webhook or API
    email.go        // SMTP
    gdrive.go       // Google Drive API
```

### Interface

```go
type Deliverer interface {
    Deliver(ctx context.Context, spec DeliverySpec, artifactPath string) error
    Name() string
}

type Dispatcher struct {
    deliverers map[string]Deliverer
}

func (d *Dispatcher) DeliverAll(ctx, specs []DeliverySpec, outputPVCPath string) []DeliveryStatus {
    var results []DeliveryStatus
    for _, spec := range specs {
        deliverer := d.deliverers[spec.Type]
        err := deliverer.Deliver(ctx, spec, filepath.Join(outputPVCPath, spec.ArtifactPath))
        status := DeliveryStatus{
            Type:   spec.Type,
            Target: spec.targetDescription(),
            Phase:  "Delivered",
        }
        if err != nil {
            status.Phase = "Failed"
            status.Error = err.Error()
        }
        results = append(results, status)
    }
    return results
}
```

### Delivery Execution

Delivery runs as a phase between "all teammates Succeeded" and "team Completed":

```
reconcileRunning:
    ...
    if allTeammatesSucceeded AND qualityGatesPassed:
        if spec.lifecycle.onComplete == "deliver":
            // Run delivery as a Job that mounts the output PVC
            createDeliveryJob(team)
            team.status.phase = "Delivering"
        else:
            team.status.phase = "Completed"

reconcileDelivering:
    // Check delivery Job status
    if job Succeeded:
        // Read delivery results from Job output
        team.status.delivery = parseDeliveryResults(job)
        team.status.phase = "Completed"
    if job Failed:
        team.status.delivery = [{phase: "Failed", error: "..."}]
        team.status.phase = "Completed"  // Still completed, delivery just failed
```

This adds a `Delivering` phase to the state machine between `Running` and `Completed`.

---

## 5. OCI Skill Distribution

### Pull Mechanism

When a teammate declares a skill with `source.oci`, the operator adds an init container to the pod spec:

```go
func addOCISkillInitContainer(podSpec *corev1.PodSpec, skill SkillSpec) {
    initContainer := corev1.Container{
        Name:  fmt.Sprintf("pull-skill-%s", skill.Name),
        Image: "ghcr.io/oras-project/oras:v1.2.0",
        Command: []string{"sh", "-c", fmt.Sprintf(
            "oras pull %s -o /skills/%s/",
            skill.Source.OCI, skill.Name,
        )},
        VolumeMounts: []corev1.VolumeMount{{
            Name:      "skills",
            MountPath: "/skills",
        }},
    }
    podSpec.InitContainers = append(podSpec.InitContainers, initContainer)
}
```

The main container mounts the same `skills` emptyDir volume and the entrypoint copies to `~/.claude/skills/`.

### Private Registry Support

If `spec.imagePullSecrets` is set, the init container receives registry credentials via:
- Mounting the pull secret as a Docker config
- Setting `DOCKER_CONFIG` env var

---

## 6. AgentTeamTrigger (v0.9.0)

### Webhook Server

The operator exposes an additional HTTP server (separate from metrics/health) for trigger webhooks:

```
Operator Pod
  :8080  ← metrics
  :8081  ← health/ready
  :9090  ← trigger webhooks (new)
```

Each `AgentTeamTrigger` CR registers a path on the webhook server. Incoming POSTs are validated, the payload is stored as a ConfigMap, and an AgentTeamRun is created.

The webhook server is optional and gated on `triggers.enabled` in the Helm values.

---

## Migration Path

All changes are additive. Existing AgentTeam CRs work without modification:

- `spec.pipeline` is optional. If absent, flat `dependsOn` works as before.
- `outputs/inputs` are optional. If absent, the shared PVC is a flat namespace as before.
- `onComplete: deliver` is a new enum value. Existing values (`create-pr`, `notify`, etc.) are unchanged.
- `AgentTeamSchedule` and `AgentTeamTrigger` are new CRDs with no impact on existing resources.

### CRD Versioning

All new types are added to `v1alpha1`. If breaking changes are needed before v1.0.0, we version to `v1alpha2` with a conversion webhook. The expectation is that the types defined here are stable enough to ship in `v1alpha1`.

---

## Testing Strategy

| Feature | Unit Tests | Integration Tests | Acceptance Tests |
|---------|-----------|-------------------|------------------|
| Output routing | Artifact copy logic, input satisfaction checks | Reconciler creates artifacts in status | Two-pod pipeline on Kind: researcher -> writer |
| Pipeline stages | Stage transition logic, cycle detection, validation | Reconciler deploys stages in order | 3-stage pipeline with fan-out/merge on Kind |
| AgentTeamSchedule | Cron parsing, idempotency, GC | Schedule reconciler creates runs | Schedule fires and produces a completed run |
| Result delivery | Each delivery type with mock targets | Dispatcher dispatches to correct type | Webhook delivery to httpbin on Kind |
| OCI skills | Init container spec generation | Skill mount path correctness | Pull from ghcr.io and verify in pod |

All new reconciler logic should have >80% unit test coverage. Integration tests use envtest. Acceptance tests run on Kind with the single-node RWO fallback.

---

## Related Documents

- [Product Vision](./product-vision.md)
- [Knowledge Work PRD](./knowledge-work-prd.md)
- [ARCHITECTURE.md](../ARCHITECTURE.md)
- [TESTING.md](../TESTING.md)
