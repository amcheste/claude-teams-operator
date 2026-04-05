package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	claudev1alpha1 "github.com/amcheste/claude-teams-operator/api/v1alpha1"
)

// --- Test Helpers ---

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = claudev1alpha1.AddToScheme(s)
	_ = batchv1.AddToScheme(s)
	return s
}

func newReconciler(objs ...client.Object) *AgentTeamReconciler {
	s := testScheme()
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&claudev1alpha1.AgentTeam{}).
		Build()
	return &AgentTeamReconciler{Client: c, Scheme: s}
}

// fetch retrieves a fresh copy of the AgentTeam from the fake client.
func fetch(t *testing.T, r *AgentTeamReconciler, name string) *claudev1alpha1.AgentTeam {
	t.Helper()
	var team claudev1alpha1.AgentTeam
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, &team))
	return &team
}

func minimalTeam(name string) *claudev1alpha1.AgentTeam {
	return &claudev1alpha1.AgentTeam{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: claudev1alpha1.AgentTeamSpec{
			Auth: claudev1alpha1.AuthSpec{APIKeySecret: "my-secret"},
			Lead: claudev1alpha1.LeadSpec{Model: "opus", Prompt: "Lead the team"},
			Teammates: []claudev1alpha1.TeammateSpec{
				{Name: "worker", Model: "sonnet", Prompt: "Do work"},
			},
		},
	}
}

func withRepo(team *claudev1alpha1.AgentTeam) *claudev1alpha1.AgentTeam {
	team.Spec.Repository = &claudev1alpha1.RepositorySpec{
		URL:    "https://github.com/example/repo",
		Branch: "main",
	}
	return team
}

func withWorkspace(team *claudev1alpha1.AgentTeam) *claudev1alpha1.AgentTeam {
	team.Spec.Workspace = &claudev1alpha1.WorkspaceSpec{
		Output: &claudev1alpha1.WorkspaceOutputSpec{Size: "5Gi"},
	}
	return team
}

func withLifecycle(team *claudev1alpha1.AgentTeam, timeout, budget string) *claudev1alpha1.AgentTeam {
	team.Spec.Lifecycle = &claudev1alpha1.LifecycleSpec{
		Timeout:     timeout,
		BudgetLimit: &budget,
	}
	return team
}

func startedAt(team *claudev1alpha1.AgentTeam, ago time.Duration) *claudev1alpha1.AgentTeam {
	t := metav1.NewTime(time.Now().Add(-ago))
	team.Status.StartedAt = &t
	team.Status.Phase = "Running"
	return team
}

func succeededPod(name, namespace, teamName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"claude.amcheste.io/team": teamName},
		},
		Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
	}
}

func failedPod(name, namespace, teamName string) *corev1.Pod {
	p := succeededPod(name, namespace, teamName)
	p.Status.Phase = corev1.PodFailed
	return p
}

func runningPod(name, namespace, teamName string) *corev1.Pod {
	p := succeededPod(name, namespace, teamName)
	p.Status.Phase = corev1.PodRunning
	return p
}

func completedJob(name, namespace string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Status:     batchv1.JobStatus{Succeeded: 1},
	}
}

func failedJob(name, namespace string) *batchv1.Job {
	limit := int32(3)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       batchv1.JobSpec{BackoffLimit: &limit},
		Status:     batchv1.JobStatus{Failed: 3},
	}
}

// --- reconcilePending ---

func TestReconcilePending_CodingMode_CreatesPVCsAndInitJob(t *testing.T) {
	team := withRepo(minimalTeam("coding-team"))
	r := newReconciler(team)
	team = fetch(t, r, "coding-team")
	ctx := context.Background()

	result, err := r.reconcilePending(ctx, team)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter)
	assert.Equal(t, "Initializing", team.Status.Phase)
	assert.NotNil(t, team.Status.StartedAt)

	var pvc corev1.PersistentVolumeClaim

	// team-state PVC is ReadWriteMany.
	require.NoError(t, r.Get(ctx, types.NamespacedName{Name: "coding-team-team-state", Namespace: "default"}, &pvc))
	assert.Equal(t, corev1.ReadWriteMany, pvc.Spec.AccessModes[0])

	// repo PVC created.
	require.NoError(t, r.Get(ctx, types.NamespacedName{Name: "coding-team-repo", Namespace: "default"}, &pvc))

	// init Job created with a container that references both PVCs.
	var job batchv1.Job
	require.NoError(t, r.Get(ctx, types.NamespacedName{Name: "coding-team-init", Namespace: "default"}, &job))
	assert.Len(t, job.Spec.Template.Spec.Volumes, 2)
}

func TestReconcilePending_CoworkMode_CreatesOutputPVC(t *testing.T) {
	team := withWorkspace(minimalTeam("cw-team"))
	r := newReconciler(team)
	team = fetch(t, r, "cw-team")
	ctx := context.Background()

	_, err := r.reconcilePending(ctx, team)
	require.NoError(t, err)

	// output PVC created.
	var pvc corev1.PersistentVolumeClaim
	require.NoError(t, r.Get(ctx, types.NamespacedName{Name: "cw-team-output", Namespace: "default"}, &pvc))

	// no repo PVC or init Job for Cowork teams.
	err = r.Get(ctx, types.NamespacedName{Name: "cw-team-repo", Namespace: "default"}, &pvc)
	assert.True(t, errors.IsNotFound(err), "repo PVC should not exist in Cowork mode")

	var job batchv1.Job
	err = r.Get(ctx, types.NamespacedName{Name: "cw-team-init", Namespace: "default"}, &job)
	assert.True(t, errors.IsNotFound(err), "init Job should not exist in Cowork mode")
}

func TestReconcilePending_Idempotent(t *testing.T) {
	team := withRepo(minimalTeam("idem-team"))
	r := newReconciler(team)
	ctx := context.Background()

	team = fetch(t, r, "idem-team")
	_, err := r.reconcilePending(ctx, team)
	require.NoError(t, err)

	// Re-fetch (first call updated status) and call again.
	team = fetch(t, r, "idem-team")
	_, err = r.reconcilePending(ctx, team)
	require.NoError(t, err, "second reconcilePending call must not error on pre-existing resources")
}

// --- reconcileInitializing ---

func TestReconcileInitializing_WaitsForInitJob(t *testing.T) {
	// Init Job exists but has not yet succeeded.
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "wait-team-init", Namespace: "default"},
		Status:     batchv1.JobStatus{Active: 1},
	}
	team := withRepo(minimalTeam("wait-team"))
	r := newReconciler(team, job)
	team = fetch(t, r, "wait-team")
	ctx := context.Background()

	result, err := r.reconcileInitializing(ctx, team)
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, result.RequeueAfter)
	assert.Equal(t, "", team.Status.Phase, "phase should not advance while init job is running")
}

func TestReconcileInitializing_InitJobFailed_SetsFailedPhase(t *testing.T) {
	team := withRepo(minimalTeam("fail-team"))
	job := failedJob("fail-team-init", "default")
	r := newReconciler(team, job)
	team = fetch(t, r, "fail-team")
	ctx := context.Background()

	_, err := r.reconcileInitializing(ctx, team)
	require.NoError(t, err)
	assert.Equal(t, "Failed", team.Status.Phase)
}

func TestReconcileInitializing_DeploysPods(t *testing.T) {
	team := withRepo(minimalTeam("deploy-team"))
	job := completedJob("deploy-team-init", "default")
	r := newReconciler(team, job)
	team = fetch(t, r, "deploy-team")
	ctx := context.Background()

	_, err := r.reconcileInitializing(ctx, team)
	require.NoError(t, err)
	assert.Equal(t, "Running", team.Status.Phase)

	// Lead pod created.
	var pod corev1.Pod
	require.NoError(t, r.Get(ctx, types.NamespacedName{Name: "deploy-team-lead", Namespace: "default"}, &pod))
	assert.Equal(t, "lead", pod.Labels["claude.amcheste.io/role"])

	// Teammate pod created (no dependencies).
	require.NoError(t, r.Get(ctx, types.NamespacedName{Name: "deploy-team-worker", Namespace: "default"}, &pod))
	assert.Equal(t, "teammate", pod.Labels["claude.amcheste.io/role"])
}

func TestReconcileInitializing_DependsOnBlocks(t *testing.T) {
	team := withRepo(minimalTeam("dep-team"))
	team.Spec.Teammates = []claudev1alpha1.TeammateSpec{
		{Name: "first", Model: "sonnet", Prompt: "First"},
		{Name: "second", Model: "sonnet", Prompt: "Second", DependsOn: []string{"first"}},
	}
	job := completedJob("dep-team-init", "default")
	r := newReconciler(team, job)
	team = fetch(t, r, "dep-team")
	ctx := context.Background()

	_, err := r.reconcileInitializing(ctx, team)
	require.NoError(t, err)

	// "first" spawned (no deps).
	var pod corev1.Pod
	require.NoError(t, r.Get(ctx, types.NamespacedName{Name: "dep-team-first", Namespace: "default"}, &pod))

	// "second" NOT spawned (depends on first which hasn't succeeded).
	err = r.Get(ctx, types.NamespacedName{Name: "dep-team-second", Namespace: "default"}, &pod)
	assert.True(t, errors.IsNotFound(err), "second teammate should be blocked by dependsOn")
}

func TestReconcileInitializing_ApprovalGateBlocksTeammate(t *testing.T) {
	team := withRepo(minimalTeam("gate-team"))
	team.Spec.Lifecycle = &claudev1alpha1.LifecycleSpec{
		ApprovalGates: []claudev1alpha1.ApprovalGateSpec{
			{Event: "spawn-worker", Channel: "none"},
		},
	}
	job := completedJob("gate-team-init", "default")
	r := newReconciler(team, job)
	team = fetch(t, r, "gate-team")
	ctx := context.Background()

	_, err := r.reconcileInitializing(ctx, team)
	require.NoError(t, err)

	// Lead spawned (no gate on lead).
	var pod corev1.Pod
	require.NoError(t, r.Get(ctx, types.NamespacedName{Name: "gate-team-lead", Namespace: "default"}, &pod))

	// Worker NOT spawned (gated).
	err = r.Get(ctx, types.NamespacedName{Name: "gate-team-worker", Namespace: "default"}, &pod)
	assert.True(t, errors.IsNotFound(err), "worker should be blocked by approval gate")

	// PendingApproval set on teammate status.
	assert.Equal(t, "spawn-worker", team.Status.Teammates[0].PendingApproval)
}

func TestReconcileInitializing_ApprovalGrantedViaAnnotation(t *testing.T) {
	team := withRepo(minimalTeam("approved-team"))
	team.Annotations = map[string]string{
		"approved.claude.amcheste.io/spawn-worker": "true",
	}
	team.Spec.Lifecycle = &claudev1alpha1.LifecycleSpec{
		ApprovalGates: []claudev1alpha1.ApprovalGateSpec{
			{Event: "spawn-worker", Channel: "none"},
		},
	}
	job := completedJob("approved-team-init", "default")
	r := newReconciler(team, job)
	team = fetch(t, r, "approved-team")
	ctx := context.Background()

	_, err := r.reconcileInitializing(ctx, team)
	require.NoError(t, err)

	// Worker spawned because approval annotation is set.
	var pod corev1.Pod
	require.NoError(t, r.Get(ctx, types.NamespacedName{Name: "approved-team-worker", Namespace: "default"}, &pod))
}

// --- reconcileRunning ---

func TestReconcileRunning_Timeout_SetsTimedOut(t *testing.T) {
	team := withLifecycle(minimalTeam("timeout-team"), "1m", "100.00")
	startedAt(team, 2*time.Minute) // started 2 minutes ago, timeout is 1 minute
	r := newReconciler(team)
	team = fetch(t, r, "timeout-team")
	team.Status.StartedAt = &metav1.Time{Time: time.Now().Add(-2 * time.Minute)}
	team.Status.Phase = "Running"
	ctx := context.Background()

	_, err := r.reconcileRunning(ctx, team)
	require.NoError(t, err)
	assert.Equal(t, "TimedOut", team.Status.Phase)
}

func TestReconcileRunning_BudgetExceeded_SetsBudgetExceeded(t *testing.T) {
	budget := "0.01" // tiny budget, will be exceeded immediately
	team := minimalTeam("budget-team")
	team.Spec.Lifecycle = &claudev1alpha1.LifecycleSpec{BudgetLimit: &budget}
	team.Status.Phase = "Running"
	startTime := metav1.NewTime(time.Now().Add(-60 * time.Minute))
	team.Status.StartedAt = &startTime
	r := newReconciler(team)
	team = fetch(t, r, "budget-team")
	team.Status.Phase = "Running"
	team.Status.StartedAt = &startTime
	ctx := context.Background()

	_, err := r.reconcileRunning(ctx, team)
	require.NoError(t, err)
	assert.Equal(t, "BudgetExceeded", team.Status.Phase)
}

func TestReconcileRunning_AllPodsSucceeded_SetsCompleted(t *testing.T) {
	team := minimalTeam("done-team")
	leadPod := succeededPod("done-team-lead", "default", "done-team")
	workerPod := succeededPod("done-team-worker", "default", "done-team")
	startTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))

	r := newReconciler(team, leadPod, workerPod)
	team = fetch(t, r, "done-team")
	team.Status.Phase = "Running"
	team.Status.StartedAt = &startTime
	ctx := context.Background()

	_, err := r.reconcileRunning(ctx, team)
	require.NoError(t, err)
	assert.Equal(t, "Completed", team.Status.Phase)
}

func TestReconcileRunning_PodFailed_SetsFailedPhase(t *testing.T) {
	team := minimalTeam("broken-team")
	leadPod := failedPod("broken-team-lead", "default", "broken-team")
	startTime := metav1.NewTime(time.Now().Add(-1 * time.Minute))

	r := newReconciler(team, leadPod)
	team = fetch(t, r, "broken-team")
	team.Status.Phase = "Running"
	team.Status.StartedAt = &startTime
	ctx := context.Background()

	_, err := r.reconcileRunning(ctx, team)
	require.NoError(t, err)
	assert.Equal(t, "Failed", team.Status.Phase)
}

func TestReconcileRunning_SpawnsNewlyUnblockedTeammate(t *testing.T) {
	team := minimalTeam("unblock-team")
	team.Spec.Teammates = []claudev1alpha1.TeammateSpec{
		{Name: "first", Model: "sonnet", Prompt: "First"},
		{Name: "second", Model: "sonnet", Prompt: "Second", DependsOn: []string{"first"}},
	}
	startTime := metav1.NewTime(time.Now().Add(-1 * time.Minute))
	// lead and first are succeeded; second should now be spawnable.
	leadPod := succeededPod("unblock-team-lead", "default", "unblock-team")
	firstPod := succeededPod("unblock-team-first", "default", "unblock-team")

	r := newReconciler(team, leadPod, firstPod)
	team = fetch(t, r, "unblock-team")
	team.Status.Phase = "Running"
	team.Status.StartedAt = &startTime
	ctx := context.Background()

	_, err := r.reconcileRunning(ctx, team)
	require.NoError(t, err)

	// "second" should now be spawned.
	var pod corev1.Pod
	require.NoError(t, r.Get(ctx, types.NamespacedName{Name: "unblock-team-second", Namespace: "default"}, &pod))
}

// --- buildAgentPod ---

func TestBuildAgentPod_CodingMode(t *testing.T) {
	team := withRepo(minimalTeam("pod-test"))
	r := newReconciler(team)

	pod := r.buildAgentPod(team, "worker", "sonnet", "do work", "auto-accept", false,
		corev1.ResourceRequirements{}, nil, nil, nil)

	assert.Equal(t, "pod-test-worker", pod.Name)
	assert.Equal(t, corev1.RestartPolicyNever, pod.Spec.RestartPolicy)

	env := envMap(pod)
	assert.Equal(t, "1", env["CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS"])
	assert.Equal(t, "pod-test", env["CLAUDE_CODE_TEAM_NAME"])
	assert.Equal(t, "worker", env["CLAUDE_CODE_AGENT_NAME"])
	assert.Equal(t, "worktrees/worker", env["WORKTREE_PATH"], "teammate should get per-worktree path")

	volNames := volumeNames(pod)
	assert.Contains(t, volNames, "team-state")
	assert.Contains(t, volNames, "repo")
}

func TestBuildAgentPod_LeadHasNoWorktreePath(t *testing.T) {
	team := withRepo(minimalTeam("lead-test"))
	r := newReconciler(team)

	pod := r.buildAgentPod(team, "lead", "opus", "lead prompt", "auto-accept", true,
		corev1.ResourceRequirements{}, nil, nil, nil)

	env := envMap(pod)
	_, hasWorktree := env["WORKTREE_PATH"]
	assert.False(t, hasWorktree, "lead should not get a worktree path")
	assert.Equal(t, "lead", pod.Labels["claude.amcheste.io/role"])
}

func TestBuildAgentPod_WithSkills(t *testing.T) {
	team := minimalTeam("skill-test")
	r := newReconciler(team)
	skills := []claudev1alpha1.SkillSpec{
		{Name: "web-research", Source: claudev1alpha1.SkillSource{ConfigMap: "web-research-skill"}},
		{Name: "report-writer", Source: claudev1alpha1.SkillSource{ConfigMap: "report-writer-skill"}},
	}

	pod := r.buildAgentPod(team, "worker", "sonnet", "do work", "auto-accept", false,
		corev1.ResourceRequirements{}, nil, skills, nil)

	volNames := volumeNames(pod)
	assert.Contains(t, volNames, "skill-web-research")
	assert.Contains(t, volNames, "skill-report-writer")

	mounts := mountPaths(pod)
	assert.Contains(t, mounts, "/var/claude-skills/web-research")
	assert.Contains(t, mounts, "/var/claude-skills/report-writer")
}

func TestBuildAgentPod_WithMCPServers(t *testing.T) {
	team := minimalTeam("mcp-test")
	r := newReconciler(team)
	mcpServers := []claudev1alpha1.MCPServerSpec{
		{Name: "gmail", URL: "https://gmail.mcp.example.com/mcp"},
	}

	pod := r.buildAgentPod(team, "worker", "sonnet", "do work", "auto-accept", false,
		corev1.ResourceRequirements{}, nil, nil, mcpServers)

	volNames := volumeNames(pod)
	assert.Contains(t, volNames, "mcp-config")

	mounts := mountPaths(pod)
	assert.Contains(t, mounts, "/var/claude-mcp")
}

func TestBuildAgentPod_CoworkMode(t *testing.T) {
	team := withWorkspace(minimalTeam("cowork-test"))
	team.Spec.Workspace.Inputs = []claudev1alpha1.WorkspaceInputSpec{
		{ConfigMap: "quarterly-data", MountPath: "/workspace/data"},
	}
	r := newReconciler(team)

	pod := r.buildAgentPod(team, "worker", "sonnet", "do work", "auto-accept", false,
		corev1.ResourceRequirements{}, nil, nil, nil)

	volNames := volumeNames(pod)
	assert.Contains(t, volNames, "workspace-output")
	assert.Contains(t, volNames, "workspace-input-0")

	// No repo volume in Cowork mode.
	assert.NotContains(t, volNames, "repo")

	env := envMap(pod)
	_, hasWorktree := env["WORKTREE_PATH"]
	assert.False(t, hasWorktree, "Cowork pods should not have a worktree path")
}

func TestBuildAgentPod_ScopeEnvVars(t *testing.T) {
	team := minimalTeam("scope-test")
	r := newReconciler(team)
	scope := &claudev1alpha1.ScopeSpec{
		IncludePaths: []string{"internal/", "api/"},
		ExcludePaths: []string{"vendor/"},
	}

	pod := r.buildAgentPod(team, "worker", "sonnet", "do work", "auto-accept", false,
		corev1.ResourceRequirements{}, scope, nil, nil)

	env := envMap(pod)
	assert.Equal(t, "internal/:api/", env["SCOPE_INCLUDE_PATHS"])
	assert.Equal(t, "vendor/", env["SCOPE_EXCLUDE_PATHS"])
}

// --- estimateCost ---

func TestEstimateCost_ZeroWhenNoStartTime(t *testing.T) {
	team := minimalTeam("cost-test")
	assert.Equal(t, "0.00", estimateCost(team))
}

func TestEstimateCost_OpusMoreExpensiveThanSonnet(t *testing.T) {
	opusTeam := minimalTeam("opus-cost")
	opusTeam.Spec.Lead.Model = "opus"
	opusTeam.Spec.Teammates = []claudev1alpha1.TeammateSpec{{Name: "a", Model: "opus"}}
	t0 := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	opusTeam.Status.StartedAt = &t0

	sonnetTeam := minimalTeam("sonnet-cost")
	sonnetTeam.Spec.Lead.Model = "sonnet"
	sonnetTeam.Spec.Teammates = []claudev1alpha1.TeammateSpec{{Name: "a", Model: "sonnet"}}
	sonnetTeam.Status.StartedAt = &t0

	var opusCost, sonnetCost float64
	_, _ = fmt.Sscanf(estimateCost(opusTeam), "%f", &opusCost)
	_, _ = fmt.Sscanf(estimateCost(sonnetTeam), "%f", &sonnetCost)
	assert.Greater(t, opusCost, sonnetCost)
}

// --- isTimedOut ---

func TestIsTimedOut_NotTimedOut(t *testing.T) {
	team := withLifecycle(minimalTeam("t"), "4h", "100.00")
	t0 := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	team.Status.StartedAt = &t0
	assert.False(t, newReconciler(team).isTimedOut(team))
}

func TestIsTimedOut_TimedOut(t *testing.T) {
	team := withLifecycle(minimalTeam("t"), "30m", "100.00")
	t0 := metav1.NewTime(time.Now().Add(-60 * time.Minute))
	team.Status.StartedAt = &t0
	assert.True(t, newReconciler(team).isTimedOut(team))
}

func TestIsTimedOut_NoStartTime(t *testing.T) {
	team := withLifecycle(minimalTeam("t"), "1h", "100.00")
	assert.False(t, newReconciler(team).isTimedOut(team))
}

// --- isBudgetExceeded ---

func TestIsBudgetExceeded_UnderBudget(t *testing.T) {
	budget := "1000.00"
	team := minimalTeam("b")
	team.Spec.Lifecycle = &claudev1alpha1.LifecycleSpec{BudgetLimit: &budget}
	team.Status.EstimatedCost = "0.50"
	assert.False(t, newReconciler(team).isBudgetExceeded(team))
}

func TestIsBudgetExceeded_OverBudget(t *testing.T) {
	budget := "1.00"
	team := minimalTeam("b")
	team.Spec.Lifecycle = &claudev1alpha1.LifecycleSpec{BudgetLimit: &budget}
	team.Status.EstimatedCost = "1.50"
	assert.True(t, newReconciler(team).isBudgetExceeded(team))
}

func TestIsBudgetExceeded_NoBudgetLimit(t *testing.T) {
	team := minimalTeam("b")
	team.Status.EstimatedCost = "999.99"
	assert.False(t, newReconciler(team).isBudgetExceeded(team), "no budget limit should never exceed")
}

// --- dependenciesMet ---

func TestDependenciesMet_NoDeps(t *testing.T) {
	team := minimalTeam("d")
	r := newReconciler(team)
	assert.True(t, r.dependenciesMet(context.Background(), team, nil))
	assert.True(t, r.dependenciesMet(context.Background(), team, []string{}))
}

func TestDependenciesMet_DepSucceeded(t *testing.T) {
	team := minimalTeam("d")
	pod := succeededPod("d-first", "default", "d")
	r := newReconciler(team, pod)
	assert.True(t, r.dependenciesMet(context.Background(), team, []string{"first"}))
}

func TestDependenciesMet_DepNotSpawned(t *testing.T) {
	team := minimalTeam("d")
	r := newReconciler(team)
	assert.False(t, r.dependenciesMet(context.Background(), team, []string{"first"}))
}

func TestDependenciesMet_DepStillRunning(t *testing.T) {
	team := minimalTeam("d")
	pod := runningPod("d-first", "default", "d")
	r := newReconciler(team, pod)
	assert.False(t, r.dependenciesMet(context.Background(), team, []string{"first"}))
}

// --- checkApprovalGate ---

func TestCheckApprovalGate_NoGateDefined(t *testing.T) {
	team := minimalTeam("ag")
	r := newReconciler(team)
	approved, err := r.checkApprovalGate(context.Background(), team, "spawn-worker")
	require.NoError(t, err)
	assert.True(t, approved)
}

func TestCheckApprovalGate_GatePresentNotApproved(t *testing.T) {
	team := minimalTeam("ag")
	team.Spec.Lifecycle = &claudev1alpha1.LifecycleSpec{
		ApprovalGates: []claudev1alpha1.ApprovalGateSpec{{Event: "spawn-worker", Channel: "none"}},
	}
	r := newReconciler(team)
	approved, err := r.checkApprovalGate(context.Background(), team, "spawn-worker")
	require.NoError(t, err)
	assert.False(t, approved)
}

func TestCheckApprovalGate_ApprovedViaAnnotation(t *testing.T) {
	team := minimalTeam("ag")
	team.Annotations = map[string]string{"approved.claude.amcheste.io/spawn-worker": "true"}
	team.Spec.Lifecycle = &claudev1alpha1.LifecycleSpec{
		ApprovalGates: []claudev1alpha1.ApprovalGateSpec{{Event: "spawn-worker", Channel: "none"}},
	}
	r := newReconciler(team)
	approved, err := r.checkApprovalGate(context.Background(), team, "spawn-worker")
	require.NoError(t, err)
	assert.True(t, approved)
}

// --- Pod/Volume introspection helpers ---

func envMap(pod *corev1.Pod) map[string]string {
	m := map[string]string{}
	for _, e := range pod.Spec.Containers[0].Env {
		m[e.Name] = e.Value
	}
	return m
}

func volumeNames(pod *corev1.Pod) []string {
	names := make([]string, len(pod.Spec.Volumes))
	for i, v := range pod.Spec.Volumes {
		names[i] = v.Name
	}
	return names
}

func mountPaths(pod *corev1.Pod) []string {
	paths := make([]string, len(pod.Spec.Containers[0].VolumeMounts))
	for i, m := range pod.Spec.Containers[0].VolumeMounts {
		paths[i] = m.MountPath
	}
	return paths
}
