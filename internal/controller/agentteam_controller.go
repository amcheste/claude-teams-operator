package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	claudev1alpha1 "github.com/camlabs/claude-teams-operator/api/v1alpha1"
)

// AgentTeamReconciler reconciles an AgentTeam object.
type AgentTeamReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=claude.camlabs.dev,resources=agentteams,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=claude.camlabs.dev,resources=agentteams/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=claude.camlabs.dev,resources=agentteams/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *AgentTeamReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	var team claudev1alpha1.AgentTeam
	if err := r.Get(ctx, req.NamespacedName, &team); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	log.V(1).Info("Reconciling AgentTeam", "phase", team.Status.Phase)

	switch team.Status.Phase {
	case "", "Pending":
		return r.reconcilePending(ctx, &team)
	case "Initializing":
		return r.reconcileInitializing(ctx, &team)
	case "Running":
		return r.reconcileRunning(ctx, &team)
	case "Completed", "Failed", "TimedOut", "BudgetExceeded":
		return r.reconcileTerminal(ctx, &team)
	default:
		log.Info("Unknown phase, resetting to Pending", "phase", team.Status.Phase)
		team.Status.Phase = "Pending"
		return ctrl.Result{Requeue: true}, r.Status().Update(ctx, &team)
	}
}

// --- Phase: Pending ---

// reconcilePending creates shared PVCs and, for coding teams, the init Job.
func (r *AgentTeamReconciler) reconcilePending(ctx context.Context, team *claudev1alpha1.AgentTeam) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Phase: Pending")

	// Always create the team-state PVC (holds .claude/teams/ and .claude/tasks/ for agent coordination).
	if err := r.ensurePVC(ctx, team, teamStatePVCName(team), "nfs", corev1.ReadWriteMany, "1Gi"); err != nil {
		return ctrl.Result{}, fmt.Errorf("ensuring team-state PVC: %w", err)
	}

	// Coding mode: repo PVC + init Job to clone and create worktrees.
	if team.Spec.Repository != nil && team.Spec.Repository.URL != "" {
		if err := r.ensurePVC(ctx, team, repoPVCName(team), "nfs", corev1.ReadWriteMany, "10Gi"); err != nil {
			return ctrl.Result{}, fmt.Errorf("ensuring repo PVC: %w", err)
		}
		if err := r.ensureInitJob(ctx, team); err != nil {
			return ctrl.Result{}, fmt.Errorf("ensuring init job: %w", err)
		}
	}

	// Cowork mode: create the output PVC if configured.
	if team.Spec.Workspace != nil && team.Spec.Workspace.Output != nil {
		out := team.Spec.Workspace.Output
		pvcName := out.PVC
		if pvcName == "" {
			pvcName = outputPVCName(team)
		}
		size := out.Size
		if size == "" {
			size = "5Gi"
		}
		sc := out.StorageClass
		if sc == "" {
			sc = "nfs"
		}
		if err := r.ensurePVC(ctx, team, pvcName, sc, corev1.ReadWriteMany, size); err != nil {
			return ctrl.Result{}, fmt.Errorf("ensuring output PVC: %w", err)
		}
	}

	team.Status.Phase = "Initializing"
	now := metav1.Now()
	team.Status.StartedAt = &now
	setCondition(team, "Progressing", metav1.ConditionTrue, "Initializing", "PVCs provisioned, init job started")
	return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(ctx, team)
}

// --- Phase: Initializing ---

// reconcileInitializing waits for the init Job (coding mode), then deploys agent pods.
func (r *AgentTeamReconciler) reconcileInitializing(ctx context.Context, team *claudev1alpha1.AgentTeam) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Phase: Initializing")

	// In coding mode, wait for the init Job before spawning pods.
	if team.Spec.Repository != nil && team.Spec.Repository.URL != "" {
		done, failed, err := r.checkJobStatus(ctx, team, initJobName(team))
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("checking init job: %w", err)
		}
		if failed {
			team.Status.Phase = "Failed"
			setCondition(team, "Progressing", metav1.ConditionFalse, "InitJobFailed", "Init job exceeded backoff limit")
			return ctrl.Result{}, r.Status().Update(ctx, team)
		}
		if !done {
			log.Info("Init job still running")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}

	// Deploy the lead pod.
	if err := r.ensureAgentPod(ctx, team, "lead", team.Spec.Lead.Model, team.Spec.Lead.Prompt,
		team.Spec.Lead.PermissionMode, true, team.Spec.Lead.Resources, nil,
		team.Spec.Lead.Skills, team.Spec.Lead.MCPServers); err != nil {
		return ctrl.Result{}, fmt.Errorf("ensuring lead pod: %w", err)
	}

	// Deploy teammates whose dependencies are already met (or have none).
	for _, tm := range team.Spec.Teammates {
		if !r.dependenciesMet(ctx, team, tm.DependsOn) {
			continue
		}
		approved, err := r.checkApprovalGate(ctx, team, "spawn-"+tm.Name)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !approved {
			r.setTeammatePendingApproval(team, tm.Name, "spawn-"+tm.Name)
			continue
		}
		if err := r.ensureAgentPod(ctx, team, tm.Name, tm.Model, tm.Prompt,
			"auto-accept", false, tm.Resources, tm.Scope,
			tm.Skills, tm.MCPServers); err != nil {
			return ctrl.Result{}, fmt.Errorf("ensuring teammate pod %s: %w", tm.Name, err)
		}
	}

	team.Status.Phase = "Running"
	setCondition(team, "Progressing", metav1.ConditionTrue, "Running", "Agent pods deployed")
	return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, team)
}

// --- Phase: Running ---

// reconcileRunning monitors agents, enforces timeout/budget, handles dynamic pod spawning and completion.
func (r *AgentTeamReconciler) reconcileRunning(ctx context.Context, team *claudev1alpha1.AgentTeam) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Phase: Running")

	// Check timeout.
	if r.isTimedOut(team) {
		log.Info("Team timed out")
		if err := r.terminateAllPods(ctx, team); err != nil {
			return ctrl.Result{}, err
		}
		team.Status.Phase = "TimedOut"
		setCondition(team, "Progressing", metav1.ConditionFalse, "TimedOut", "Team exceeded configured timeout")
		return ctrl.Result{}, r.Status().Update(ctx, team)
	}

	// Update cost estimate and check budget.
	team.Status.EstimatedCost = estimateCost(team)
	if r.isBudgetExceeded(team) {
		log.Info("Budget exceeded", "cost", team.Status.EstimatedCost)
		if err := r.terminateAllPods(ctx, team); err != nil {
			return ctrl.Result{}, err
		}
		team.Status.Phase = "BudgetExceeded"
		setCondition(team, "Progressing", metav1.ConditionFalse, "BudgetExceeded", "Estimated cost exceeded budget limit")
		return ctrl.Result{}, r.Status().Update(ctx, team)
	}

	// Sync pod statuses into team.Status.
	if err := r.syncPodStatuses(ctx, team); err != nil {
		return ctrl.Result{}, err
	}

	// Spawn any newly unblocked or newly approved teammates.
	for _, tm := range team.Spec.Teammates {
		podName := agentPodName(team, tm.Name)
		pod := &corev1.Pod{}
		if err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: team.Namespace}, pod); err == nil {
			continue // Already spawned.
		} else if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		if !r.dependenciesMet(ctx, team, tm.DependsOn) {
			continue
		}
		approved, err := r.checkApprovalGate(ctx, team, "spawn-"+tm.Name)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !approved {
			r.setTeammatePendingApproval(team, tm.Name, "spawn-"+tm.Name)
			continue
		}
		r.clearTeammatePendingApproval(team, tm.Name)
		if err := r.ensureAgentPod(ctx, team, tm.Name, tm.Model, tm.Prompt,
			"auto-accept", false, tm.Resources, tm.Scope,
			tm.Skills, tm.MCPServers); err != nil {
			return ctrl.Result{}, fmt.Errorf("spawning teammate %s: %w", tm.Name, err)
		}
		log.Info("Spawned teammate", "name", tm.Name)
	}

	// Check for overall completion.
	allDone, anyFailed, err := r.allPodsComplete(ctx, team)
	if err != nil {
		return ctrl.Result{}, err
	}
	if anyFailed {
		team.Status.Phase = "Failed"
		setCondition(team, "Progressing", metav1.ConditionFalse, "AgentFailed", "One or more agent pods failed")
		return ctrl.Result{}, r.Status().Update(ctx, team)
	}
	if allDone {
		log.Info("All agents complete, running onComplete")
		if err := r.executeOnComplete(ctx, team); err != nil {
			log.Error(err, "OnComplete action failed")
			// Don't fail the team for post-completion actions.
		}
		team.Status.Phase = "Completed"
		setCondition(team, "Progressing", metav1.ConditionFalse, "Completed", "All agents finished successfully")
		return ctrl.Result{}, r.Status().Update(ctx, team)
	}

	// Save any status changes made during this reconcile.
	if err := r.Status().Update(ctx, team); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// --- Phase: Terminal ---

// reconcileTerminal cleans up pods for completed/failed teams.
func (r *AgentTeamReconciler) reconcileTerminal(ctx context.Context, team *claudev1alpha1.AgentTeam) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Phase: Terminal", "phase", team.Status.Phase)

	if err := r.terminateAllPods(ctx, team); err != nil {
		return ctrl.Result{}, err
	}

	if team.Status.CompletedAt == nil {
		now := metav1.Now()
		team.Status.CompletedAt = &now
		return ctrl.Result{}, r.Status().Update(ctx, team)
	}
	return ctrl.Result{}, nil
}

// --- PVC Management ---

func (r *AgentTeamReconciler) ensurePVC(ctx context.Context, team *claudev1alpha1.AgentTeam, name, storageClass string, accessMode corev1.PersistentVolumeAccessMode, size string) error {
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: team.Namespace}, pvc); err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	qty, err := resource.ParseQuantity(size)
	if err != nil {
		return fmt.Errorf("parsing storage size %q: %w", size, err)
	}

	pvc = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: team.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{accessMode},
			StorageClassName: &storageClass,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: qty,
				},
			},
		},
	}
	ctrl.SetControllerReference(team, pvc, r.Scheme)
	return r.Create(ctx, pvc)
}

// --- Init Job (Coding Mode) ---

// ensureInitJob creates the Job that clones the repo and sets up per-teammate worktrees.
func (r *AgentTeamReconciler) ensureInitJob(ctx context.Context, team *claudev1alpha1.AgentTeam) error {
	jobName := initJobName(team)
	job := &batchv1.Job{}
	if err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: team.Namespace}, job); err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	repo := team.Spec.Repository
	branch := repo.Branch
	if branch == "" {
		branch = "main"
	}

	teammateNames := make([]string, len(team.Spec.Teammates))
	for i, tm := range team.Spec.Teammates {
		teammateNames[i] = tm.Name
	}

	// The init script clones the repo and sets up one worktree per teammate.
	initScript := fmt.Sprintf(`
set -eu
echo "[init] Cloning %s @ %s"
git clone --branch %s %s /workspace/repo
cd /workspace/repo

echo "[init] Creating per-teammate worktrees"
for tm in %s; do
  git worktree add /workspace/worktrees/$tm -b teammate-$tm
  echo "  worktree: /workspace/worktrees/$tm"
done

echo "[init] Initialising team-state directories"
mkdir -p /state/teams/%s/inboxes
mkdir -p /state/tasks/%s
printf '{"tasks":[],"version":1}\n' > /state/tasks/%s/tasks.json

echo "[init] Done"
`,
		repo.URL, branch, branch, repo.URL,
		strings.Join(teammateNames, " "),
		team.Name, team.Name, team.Name,
	)

	envVars := []corev1.EnvVar{}
	if repo.CredentialsSecret != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name: "GIT_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: repo.CredentialsSecret},
					Key:                  "token",
					Optional:             boolPtr(true),
				},
			},
		})
	}

	backoffLimit := int32(3)
	job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: team.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:    "init",
							Image:   "alpine/git:latest",
							Command: []string{"sh", "-c", initScript},
							Env:     envVars,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "repo", MountPath: "/workspace"},
								{Name: "team-state", MountPath: "/state"},
							},
						},
					},
					Volumes: []corev1.Volume{
						pvcVolume("repo", repoPVCName(team)),
						pvcVolume("team-state", teamStatePVCName(team)),
					},
				},
			},
		},
	}
	ctrl.SetControllerReference(team, job, r.Scheme)
	return r.Create(ctx, job)
}

func (r *AgentTeamReconciler) checkJobStatus(ctx context.Context, team *claudev1alpha1.AgentTeam, jobName string) (completed, failed bool, err error) {
	job := &batchv1.Job{}
	if err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: team.Namespace}, job); err != nil {
		if errors.IsNotFound(err) {
			return false, false, nil
		}
		return false, false, err
	}
	if job.Status.Succeeded > 0 {
		return true, false, nil
	}
	limit := int32(3)
	if job.Spec.BackoffLimit != nil {
		limit = *job.Spec.BackoffLimit
	}
	if job.Status.Failed >= limit {
		return false, true, nil
	}
	return false, false, nil
}

// --- Agent Pod Management ---

// ensureAgentPod creates an agent pod if it doesn't already exist.
func (r *AgentTeamReconciler) ensureAgentPod(
	ctx context.Context,
	team *claudev1alpha1.AgentTeam,
	agentName, model, prompt, permissionMode string,
	isLead bool,
	resources corev1.ResourceRequirements,
	scope *claudev1alpha1.ScopeSpec,
	skills []claudev1alpha1.SkillSpec,
	mcpServers []claudev1alpha1.MCPServerSpec,
) error {
	podName := agentPodName(team, agentName)
	pod := &corev1.Pod{}
	if err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: team.Namespace}, pod); err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	// Create a ConfigMap with the MCP config if this agent has MCP servers.
	if len(mcpServers) > 0 {
		if err := r.ensureMCPConfigMap(ctx, team, agentName, mcpServers); err != nil {
			return fmt.Errorf("ensuring MCP configmap for %s: %w", agentName, err)
		}
	}

	pod = r.buildAgentPod(team, agentName, model, prompt, permissionMode, isLead, resources, scope, skills, mcpServers)
	return r.Create(ctx, pod)
}

// ensureMCPConfigMap creates a ConfigMap with the agent's .mcp.json content.
func (r *AgentTeamReconciler) ensureMCPConfigMap(ctx context.Context, team *claudev1alpha1.AgentTeam, agentName string, servers []claudev1alpha1.MCPServerSpec) error {
	cmName := agentMCPConfigMapName(team, agentName)
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: team.Namespace}, cm); err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	type mcpEntry struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	mcpMap := map[string]mcpEntry{}
	for _, s := range servers {
		mcpMap[s.Name] = mcpEntry{Type: "sse", URL: s.URL}
	}
	wrapped := map[string]interface{}{"mcpServers": mcpMap}
	data, err := json.Marshal(wrapped)
	if err != nil {
		return fmt.Errorf("marshalling MCP config: %w", err)
	}

	cm = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: team.Namespace,
		},
		Data: map[string]string{
			"mcp.json": string(data),
		},
	}
	ctrl.SetControllerReference(team, cm, r.Scheme)
	return r.Create(ctx, cm)
}

// buildAgentPod constructs a Pod spec for a Claude Code agent.
func (r *AgentTeamReconciler) buildAgentPod(
	team *claudev1alpha1.AgentTeam,
	agentName, model, prompt, permissionMode string,
	isLead bool,
	resources corev1.ResourceRequirements,
	scope *claudev1alpha1.ScopeSpec,
	skills []claudev1alpha1.SkillSpec,
	mcpServers []claudev1alpha1.MCPServerSpec,
) *corev1.Pod {
	role := "teammate"
	if isLead {
		role = "lead"
	}

	labels := map[string]string{
		"app.kubernetes.io/name":      "claude-teams-operator",
		"app.kubernetes.io/instance":  team.Name,
		"app.kubernetes.io/component": agentName,
		"claude.camlabs.dev/team":     team.Name,
		"claude.camlabs.dev/role":     role,
	}

	envVars := []corev1.EnvVar{
		{Name: "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", Value: "1"},
		{Name: "CLAUDE_CODE_TEAM_NAME", Value: team.Name},
		{Name: "CLAUDE_CODE_AGENT_NAME", Value: agentName},
		{Name: "CLAUDE_CODE_ROLE", Value: role},
		{Name: "CLAUDE_MODEL", Value: model},
		{Name: "CLAUDE_PERMISSION_MODE", Value: permissionMode},
		{Name: "AGENT_PROMPT", Value: prompt},
	}

	// Auth: prefer API key, fall back to OAuth.
	if team.Spec.Auth.APIKeySecret != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name: "ANTHROPIC_API_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: team.Spec.Auth.APIKeySecret},
					Key:                  "ANTHROPIC_API_KEY",
				},
			},
		})
	} else if team.Spec.Auth.OAuthSecret != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name: "CLAUDE_OAUTH_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: team.Spec.Auth.OAuthSecret},
					Key:                  "CLAUDE_OAUTH_TOKEN",
				},
			},
		})
	}

	// Scope: pass include/exclude paths to the entrypoint.
	if scope != nil {
		if len(scope.IncludePaths) > 0 {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "SCOPE_INCLUDE_PATHS",
				Value: strings.Join(scope.IncludePaths, ":"),
			})
		}
		if len(scope.ExcludePaths) > 0 {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "SCOPE_EXCLUDE_PATHS",
				Value: strings.Join(scope.ExcludePaths, ":"),
			})
		}
	}

	// In coding mode, route each teammate to their own worktree.
	if team.Spec.Repository != nil && !isLead {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "WORKTREE_PATH",
			Value: fmt.Sprintf("worktrees/%s", agentName),
		})
	}

	volumes := []corev1.Volume{
		// team-state is mounted at /var/claude-state; entrypoint symlinks teams/ and tasks/ into ~/.claude/.
		pvcVolume("team-state", teamStatePVCName(team)),
	}
	volumeMounts := []corev1.VolumeMount{
		{Name: "team-state", MountPath: "/var/claude-state"},
	}

	// Coding mode: mount the repo PVC.
	if team.Spec.Repository != nil {
		volumes = append(volumes, pvcVolume("repo", repoPVCName(team)))
		volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: "repo", MountPath: "/workspace"})
	}

	// Cowork mode: mount output PVC and all input volumes.
	if team.Spec.Workspace != nil {
		if team.Spec.Workspace.Output != nil {
			outPVCName := team.Spec.Workspace.Output.PVC
			if outPVCName == "" {
				outPVCName = outputPVCName(team)
			}
			mountPath := team.Spec.Workspace.Output.MountPath
			if mountPath == "" {
				mountPath = "/workspace/output"
			}
			volumes = append(volumes, pvcVolume("workspace-output", outPVCName))
			volumeMounts = append(volumeMounts, corev1.VolumeMount{Name: "workspace-output", MountPath: mountPath})
		}
		for i, input := range team.Spec.Workspace.Inputs {
			volName := fmt.Sprintf("workspace-input-%d", i)
			if input.ConfigMap != "" {
				volumes = append(volumes, corev1.Volume{
					Name: volName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: input.ConfigMap},
						},
					},
				})
			} else if input.PVC != "" {
				volumes = append(volumes, pvcVolumeReadOnly(volName, input.PVC))
			} else {
				continue
			}
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      volName,
				MountPath: input.MountPath,
				ReadOnly:  true,
			})
		}
	}

	// Skills: each ConfigMap-backed skill gets mounted at /var/claude-skills/{name}/.
	// The entrypoint copies them into ~/.claude/skills/{name}/.
	for _, skill := range skills {
		if skill.Source.ConfigMap == "" {
			continue // OCI not yet implemented.
		}
		volName := "skill-" + skill.Name
		volumes = append(volumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: skill.Source.ConfigMap},
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: fmt.Sprintf("/var/claude-skills/%s", skill.Name),
			ReadOnly:  true,
		})
	}

	// MCP config: mount the per-agent ConfigMap at /var/claude-mcp/mcp.json.
	// The entrypoint copies it to ~/.mcp.json.
	if len(mcpServers) > 0 {
		cmName := agentMCPConfigMapName(team, agentName)
		volumes = append(volumes, corev1.Volume{
			Name: "mcp-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "mcp-config",
			MountPath: "/var/claude-mcp",
			ReadOnly:  true,
		})
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentPodName(team, agentName),
			Namespace: team.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:         "claude-code",
					Image:        "ghcr.io/camlabs/claude-code-runner:latest",
					Env:          envVars,
					VolumeMounts: volumeMounts,
					Resources:    resources,
				},
			},
			Volumes: volumes,
		},
	}

	ctrl.SetControllerReference(team, pod, r.Scheme)
	return pod
}

// --- Status Sync ---

// syncPodStatuses reads pod phases and updates team.Status.Lead and team.Status.Teammates.
func (r *AgentTeamReconciler) syncPodStatuses(ctx context.Context, team *claudev1alpha1.AgentTeam) error {
	// Lead pod.
	leadPod := &corev1.Pod{}
	if err := r.Get(ctx, types.NamespacedName{Name: agentPodName(team, "lead"), Namespace: team.Namespace}, leadPod); err == nil {
		if team.Status.Lead == nil {
			team.Status.Lead = &claudev1alpha1.AgentStatus{}
		}
		team.Status.Lead.PodName = leadPod.Name
		team.Status.Lead.Phase = podPhaseToAgentPhase(leadPod)
	}

	// Teammate pods.
	statusMap := map[string]*claudev1alpha1.TeammateStatus{}
	for i := range team.Status.Teammates {
		statusMap[team.Status.Teammates[i].Name] = &team.Status.Teammates[i]
	}
	for _, tm := range team.Spec.Teammates {
		pod := &corev1.Pod{}
		if err := r.Get(ctx, types.NamespacedName{Name: agentPodName(team, tm.Name), Namespace: team.Namespace}, pod); err != nil {
			continue
		}
		st, ok := statusMap[tm.Name]
		if !ok {
			team.Status.Teammates = append(team.Status.Teammates, claudev1alpha1.TeammateStatus{Name: tm.Name})
			st = &team.Status.Teammates[len(team.Status.Teammates)-1]
			statusMap[tm.Name] = st
		}
		st.PodName = pod.Name
		st.Phase = podPhaseToAgentPhase(pod)
	}
	return nil
}

func podPhaseToAgentPhase(pod *corev1.Pod) string {
	switch pod.Status.Phase {
	case corev1.PodPending:
		return "Pending"
	case corev1.PodRunning:
		return "Running"
	case corev1.PodSucceeded:
		return "Completed"
	case corev1.PodFailed:
		return "Failed"
	default:
		return "Pending"
	}
}

// --- Completion ---

func (r *AgentTeamReconciler) allPodsComplete(ctx context.Context, team *claudev1alpha1.AgentTeam) (allDone, anyFailed bool, err error) {
	checkPod := func(name string) (done, failed bool, err error) {
		pod := &corev1.Pod{}
		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: team.Namespace}, pod); err != nil {
			if errors.IsNotFound(err) {
				return false, false, nil // Not yet spawned.
			}
			return false, false, err
		}
		switch pod.Status.Phase {
		case corev1.PodSucceeded:
			return true, false, nil
		case corev1.PodFailed:
			return false, true, nil
		default:
			return false, false, nil
		}
	}

	// Lead must succeed.
	done, failed, err := checkPod(agentPodName(team, "lead"))
	if err != nil || failed {
		return false, failed, err
	}
	if !done {
		return false, false, nil
	}

	// All spawned teammates must succeed.
	for _, tm := range team.Spec.Teammates {
		done, failed, err := checkPod(agentPodName(team, tm.Name))
		if err != nil || failed {
			return false, failed, err
		}
		if !done {
			return false, false, nil
		}
	}
	return true, false, nil
}

func (r *AgentTeamReconciler) executeOnComplete(ctx context.Context, team *claudev1alpha1.AgentTeam) error {
	if team.Spec.Lifecycle == nil {
		return nil
	}
	log := log.FromContext(ctx)
	switch team.Spec.Lifecycle.OnComplete {
	case "notify":
		if team.Spec.Observability != nil && team.Spec.Observability.Webhook != nil {
			return r.sendWebhookEvent(team.Spec.Observability.Webhook.URL, "completed", team)
		}
	case "create-pr":
		log.Info("TODO: create PR via gh CLI or GitHub API")
	case "push-branch":
		log.Info("TODO: push consolidated branch")
	}
	return nil
}

// --- Approval Gates ---

func (r *AgentTeamReconciler) checkApprovalGate(ctx context.Context, team *claudev1alpha1.AgentTeam, event string) (approved bool, err error) {
	if team.Spec.Lifecycle == nil || len(team.Spec.Lifecycle.ApprovalGates) == 0 {
		return true, nil
	}
	var gate *claudev1alpha1.ApprovalGateSpec
	for i := range team.Spec.Lifecycle.ApprovalGates {
		if team.Spec.Lifecycle.ApprovalGates[i].Event == event {
			gate = &team.Spec.Lifecycle.ApprovalGates[i]
			break
		}
	}
	if gate == nil {
		return true, nil // No gate for this event.
	}

	// Check for the approval annotation.
	annotationKey := "claude.camlabs.dev/approved/" + event
	if team.Annotations[annotationKey] == "true" {
		return true, nil
	}

	// Send webhook notification so an external system can approve.
	if gate.Channel == "webhook" && gate.WebhookURL != "" {
		if err := r.sendWebhookEvent(gate.WebhookURL, event, team); err != nil {
			log.FromContext(ctx).Error(err, "Failed to send approval webhook", "event", event)
		}
	}
	return false, nil
}

func (r *AgentTeamReconciler) sendWebhookEvent(url, event string, team *claudev1alpha1.AgentTeam) error {
	payload := map[string]string{
		"event":     event,
		"team":      team.Name,
		"namespace": team.Namespace,
		"phase":     team.Status.Phase,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body)) //nolint:gosec // URL from trusted CR
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook %s returned HTTP %d", url, resp.StatusCode)
	}
	return nil
}

func (r *AgentTeamReconciler) setTeammatePendingApproval(team *claudev1alpha1.AgentTeam, tmName, event string) {
	for i := range team.Status.Teammates {
		if team.Status.Teammates[i].Name == tmName {
			team.Status.Teammates[i].PendingApproval = event
			return
		}
	}
	team.Status.Teammates = append(team.Status.Teammates, claudev1alpha1.TeammateStatus{
		Name:            tmName,
		PendingApproval: event,
	})
}

func (r *AgentTeamReconciler) clearTeammatePendingApproval(team *claudev1alpha1.AgentTeam, tmName string) {
	for i := range team.Status.Teammates {
		if team.Status.Teammates[i].Name == tmName {
			team.Status.Teammates[i].PendingApproval = ""
			return
		}
	}
}

// --- Dependency Ordering ---

// dependenciesMet returns true if all pods named in deps have Succeeded.
func (r *AgentTeamReconciler) dependenciesMet(ctx context.Context, team *claudev1alpha1.AgentTeam, deps []string) bool {
	for _, dep := range deps {
		pod := &corev1.Pod{}
		if err := r.Get(ctx, types.NamespacedName{Name: agentPodName(team, dep), Namespace: team.Namespace}, pod); err != nil {
			return false
		}
		if pod.Status.Phase != corev1.PodSucceeded {
			return false
		}
	}
	return true
}

// --- Timeout / Budget ---

func (r *AgentTeamReconciler) isTimedOut(team *claudev1alpha1.AgentTeam) bool {
	if team.Status.StartedAt == nil {
		return false
	}
	timeout := "4h"
	if team.Spec.Lifecycle != nil && team.Spec.Lifecycle.Timeout != "" {
		timeout = team.Spec.Lifecycle.Timeout
	}
	d, err := time.ParseDuration(timeout)
	if err != nil {
		return false
	}
	return time.Since(team.Status.StartedAt.Time) > d
}

func (r *AgentTeamReconciler) isBudgetExceeded(team *claudev1alpha1.AgentTeam) bool {
	if team.Spec.Lifecycle == nil || team.Spec.Lifecycle.BudgetLimit == nil {
		return false
	}
	var limit, current float64
	if _, err := fmt.Sscanf(*team.Spec.Lifecycle.BudgetLimit, "%f", &limit); err != nil {
		return false
	}
	fmt.Sscanf(team.Status.EstimatedCost, "%f", &current) //nolint:errcheck
	return current >= limit
}

// estimateCost returns a rough USD estimate based on elapsed time and model type.
// Assumes ~1500 tokens/min for opus ($15/M), ~3000 tokens/min for sonnet ($3/M).
func estimateCost(team *claudev1alpha1.AgentTeam) string {
	if team.Status.StartedAt == nil {
		return "0.00"
	}
	elapsed := time.Since(team.Status.StartedAt.Time).Minutes()
	total := estimateModelCost(team.Spec.Lead.Model, elapsed)
	for _, tm := range team.Spec.Teammates {
		total += estimateModelCost(tm.Model, elapsed)
	}
	return fmt.Sprintf("%.2f", total)
}

func estimateModelCost(model string, elapsedMinutes float64) float64 {
	switch model {
	case "opus":
		return elapsedMinutes * 1500 / 1_000_000 * 15
	default:
		return elapsedMinutes * 3000 / 1_000_000 * 3
	}
}

// --- Pod Cleanup ---

func (r *AgentTeamReconciler) terminateAllPods(ctx context.Context, team *claudev1alpha1.AgentTeam) error {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(team.Namespace),
		client.MatchingLabels{"claude.camlabs.dev/team": team.Name},
	); err != nil {
		return err
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.DeletionTimestamp == nil {
			if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

// --- Condition Helpers ---

func setCondition(team *claudev1alpha1.AgentTeam, condType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	for i, c := range team.Status.Conditions {
		if c.Type == condType {
			if c.Status != status {
				team.Status.Conditions[i].LastTransitionTime = now
			}
			team.Status.Conditions[i].Status = status
			team.Status.Conditions[i].Reason = reason
			team.Status.Conditions[i].Message = message
			return
		}
	}
	team.Status.Conditions = append(team.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
}

// --- Volume Helpers ---

func pvcVolume(name, claimName string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claimName},
		},
	}
}

func pvcVolumeReadOnly(name, claimName string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claimName, ReadOnly: true},
		},
	}
}

// --- Name Helpers ---

func teamStatePVCName(team *claudev1alpha1.AgentTeam) string {
	return team.Name + "-team-state"
}

func repoPVCName(team *claudev1alpha1.AgentTeam) string {
	return team.Name + "-repo"
}

func outputPVCName(team *claudev1alpha1.AgentTeam) string {
	return team.Name + "-output"
}

func initJobName(team *claudev1alpha1.AgentTeam) string {
	return team.Name + "-init"
}

func agentPodName(team *claudev1alpha1.AgentTeam, agentName string) string {
	return team.Name + "-" + agentName
}

func agentMCPConfigMapName(team *claudev1alpha1.AgentTeam, agentName string) string {
	return team.Name + "-" + agentName + "-mcp"
}

func boolPtr(b bool) *bool { return &b }

// --- Controller Setup ---

// SetupWithManager sets up the controller with the Manager.
func (r *AgentTeamReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&claudev1alpha1.AgentTeam{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
