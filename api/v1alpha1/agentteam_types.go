// Package v1alpha1 contains API Schema definitions for the claude v1alpha1 API group.
// +kubebuilder:object:generate=true
// +groupName=claude.amcheste.io
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentTeamSpec defines the desired state of an AgentTeam.
type AgentTeamSpec struct {
	// Repository configuration for the codebase agents will work on.
	// Use this for coding tasks. Optional when Workspace is set.
	// +optional
	Repository *RepositorySpec `json:"repository,omitempty"`

	// Workspace configures non-git inputs and outputs for Cowork teams.
	// Use this for knowledge-work tasks (documents, reports, email, etc.).
	// +optional
	Workspace *WorkspaceSpec `json:"workspace,omitempty"`

	// Auth configures how agents authenticate with the Anthropic API.
	Auth AuthSpec `json:"auth"`

	// Lead configures the team lead agent.
	Lead LeadSpec `json:"lead"`

	// Teammates defines the worker agents in the team.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	Teammates []TeammateSpec `json:"teammates"`

	// Coordination configures how agents communicate.
	// +optional
	Coordination *CoordinationSpec `json:"coordination,omitempty"`

	// Lifecycle configures team runtime behavior and budget.
	// +optional
	Lifecycle *LifecycleSpec `json:"lifecycle,omitempty"`

	// QualityGates configures validation before marking team complete.
	// +optional
	QualityGates *QualityGateSpec `json:"qualityGates,omitempty"`

	// Observability configures metrics and notifications.
	// +optional
	Observability *ObservabilitySpec `json:"observability,omitempty"`
}

// RepositorySpec defines the git repository configuration.
type RepositorySpec struct {
	// URL is the git clone URL.
	URL string `json:"url"`

	// Branch to clone and work from.
	// +kubebuilder:default="main"
	Branch string `json:"branch,omitempty"`

	// WorktreeStrategy determines how git worktrees are managed.
	// +kubebuilder:validation:Enum=per-teammate;shared
	// +kubebuilder:default="per-teammate"
	WorktreeStrategy string `json:"worktreeStrategy,omitempty"`

	// CredentialsSecret references a Secret containing git credentials.
	// The secret should contain either 'ssh-privatekey' or 'token'.
	// +optional
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
}

// AuthSpec defines Anthropic API authentication.
type AuthSpec struct {
	// APIKeySecret references a Secret containing ANTHROPIC_API_KEY.
	// +optional
	APIKeySecret string `json:"apiKeySecret,omitempty"`

	// OAuthSecret references a Secret containing OAuth tokens for subscription auth.
	// +optional
	OAuthSecret string `json:"oauthSecret,omitempty"`
}

// LeadSpec defines the team lead configuration.
type LeadSpec struct {
	// Model to use for the team lead.
	// +kubebuilder:validation:Enum=opus;sonnet;haiku
	// +kubebuilder:default="opus"
	Model string `json:"model,omitempty"`

	// Prompt is the initial instruction for the team lead.
	Prompt string `json:"prompt"`

	// PermissionMode controls how the lead handles permission requests.
	// +kubebuilder:validation:Enum=auto-accept;plan;default
	// +kubebuilder:default="auto-accept"
	PermissionMode string `json:"permissionMode,omitempty"`

	// Skills to mount into .claude/skills/ for the lead agent.
	// +optional
	Skills []SkillSpec `json:"skills,omitempty"`

	// MCPServers configures Model Context Protocol connections for the lead agent.
	// +optional
	MCPServers []MCPServerSpec `json:"mcpServers,omitempty"`

	// Resources defines compute resources for the lead pod.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// SkillSource identifies where to load a skill from. Exactly one field should be set.
type SkillSource struct {
	// ConfigMap references a ConfigMap in the same namespace.
	// Each key in the ConfigMap becomes a file in the skill directory.
	// +optional
	ConfigMap string `json:"configMap,omitempty"`

	// OCI is an OCI artifact reference containing the skill files (e.g. "ghcr.io/org/skills/web-research:v1").
	// TODO: OCI skill pulling is not yet implemented; use ConfigMap instead.
	// +optional
	OCI string `json:"oci,omitempty"`
}

// SkillSpec defines a Claude Code skill to mount into an agent pod.
type SkillSpec struct {
	// Name is the skill directory name under .claude/skills/.
	Name string `json:"name"`

	// Source identifies where to load the skill from.
	Source SkillSource `json:"source"`
}

// MCPServerSpec configures a Model Context Protocol server for an agent.
type MCPServerSpec struct {
	// Name identifies this MCP server in the agent's config.
	Name string `json:"name"`

	// URL is the MCP server endpoint.
	URL string `json:"url"`

	// CredentialsSecret references a Secret containing an 'apiKey' key for bearer auth.
	// +optional
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
}

// WorkspaceInputSpec defines a read-only input mounted into the agent pod.
type WorkspaceInputSpec struct {
	// ConfigMap references a ConfigMap to mount as a directory.
	// +optional
	ConfigMap string `json:"configMap,omitempty"`

	// PVC references an existing PersistentVolumeClaim to mount read-only.
	// +optional
	PVC string `json:"pvc,omitempty"`

	// MountPath is where to mount this input inside the container.
	MountPath string `json:"mountPath"`
}

// WorkspaceOutputSpec defines the writable output volume for a Cowork team.
type WorkspaceOutputSpec struct {
	// PVC is the name of an existing PVC to use. If empty, the operator creates one named "{team}-output".
	// +optional
	PVC string `json:"pvc,omitempty"`

	// StorageClass for the auto-created PVC. Defaults to "nfs".
	// +optional
	StorageClass string `json:"storageClass,omitempty"`

	// Size of the auto-created PVC.
	// +kubebuilder:default="5Gi"
	Size string `json:"size,omitempty"`

	// MountPath inside the container where the output volume is mounted.
	// +kubebuilder:default="/workspace/output"
	MountPath string `json:"mountPath,omitempty"`
}

// WorkspaceSpec configures non-git inputs and outputs for Cowork teams.
// Use this instead of (or alongside) Repository for knowledge-work tasks.
type WorkspaceSpec struct {
	// Inputs are read-only volumes mounted into all agent pods.
	// +optional
	Inputs []WorkspaceInputSpec `json:"inputs,omitempty"`

	// Output configures the shared writable output volume.
	// +optional
	Output *WorkspaceOutputSpec `json:"output,omitempty"`
}

// ApprovalGateSpec pauses execution before a named event until human approval is recorded.
// Approval is granted by adding the annotation approved.claude.amcheste.io/{event}=true to the AgentTeam.
type ApprovalGateSpec struct {
	// Event is the gate identifier. Use "spawn-{teammate-name}" to gate spawning a specific teammate.
	Event string `json:"event"`

	// Channel is how the approval request notification is sent.
	// +kubebuilder:validation:Enum=webhook;none
	// +kubebuilder:default="none"
	Channel string `json:"channel,omitempty"`

	// WebhookURL to POST when this gate is triggered (used when channel is "webhook").
	// +optional
	WebhookURL string `json:"webhookUrl,omitempty"`
}

// TeammateSpec defines a single teammate agent.
type TeammateSpec struct {
	// Name is the unique identifier for this teammate.
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	Name string `json:"name"`

	// Model to use for this teammate.
	// +kubebuilder:validation:Enum=opus;sonnet;haiku
	// +kubebuilder:default="sonnet"
	Model string `json:"model,omitempty"`

	// Prompt is the spawn instruction for this teammate.
	Prompt string `json:"prompt"`

	// Scope restricts which files this teammate can access.
	// +optional
	Scope *ScopeSpec `json:"scope,omitempty"`

	// DependsOn lists teammate names that must complete before this one starts.
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`

	// Skills to mount into .claude/skills/ for this teammate.
	// +optional
	Skills []SkillSpec `json:"skills,omitempty"`

	// MCPServers configures Model Context Protocol connections for this teammate.
	// +optional
	MCPServers []MCPServerSpec `json:"mcpServers,omitempty"`

	// Resources defines compute resources for this teammate's pod.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ScopeSpec restricts file access for a teammate.
type ScopeSpec struct {
	// IncludePaths lists paths the teammate should focus on.
	// +optional
	IncludePaths []string `json:"includePaths,omitempty"`

	// ExcludePaths lists paths the teammate should not modify.
	// +optional
	ExcludePaths []string `json:"excludePaths,omitempty"`
}

// CoordinationSpec configures inter-agent communication.
type CoordinationSpec struct {
	// MailboxBackend determines how mailbox messages are transported.
	// +kubebuilder:validation:Enum=shared-volume;redis;nats
	// +kubebuilder:default="shared-volume"
	MailboxBackend string `json:"mailboxBackend,omitempty"`

	// TaskBackend determines how the shared task list is stored.
	// +kubebuilder:validation:Enum=shared-volume;beads
	// +kubebuilder:default="shared-volume"
	TaskBackend string `json:"taskBackend,omitempty"`

	// Beads configures optional Beads integration for persistent tracking.
	// +optional
	Beads *BeadsSpec `json:"beads,omitempty"`
}

// BeadsSpec configures Beads integration.
type BeadsSpec struct {
	// Enabled turns on Beads tracking.
	Enabled bool `json:"enabled"`

	// DoltServerService is the K8s service name for the Dolt SQL server.
	// +optional
	DoltServerService string `json:"doltServerService,omitempty"`

	// DoltServerPort is the port for the Dolt SQL server.
	// +kubebuilder:default=3306
	DoltServerPort int32 `json:"doltServerPort,omitempty"`
}

// LifecycleSpec controls team runtime behavior.
type LifecycleSpec struct {
	// Timeout is the maximum duration the team can run (e.g. "4h", "30m").
	// +kubebuilder:default="4h"
	Timeout string `json:"timeout,omitempty"`

	// BudgetLimit is the maximum API spend in USD before the team is terminated (e.g. "10.00").
	// +optional
	BudgetLimit *string `json:"budgetLimit,omitempty"`

	// OnComplete determines what happens when the team finishes.
	// +kubebuilder:validation:Enum=create-pr;push-branch;notify;none
	// +kubebuilder:default="notify"
	OnComplete string `json:"onComplete,omitempty"`

	// PullRequest configures PR creation when onComplete is "create-pr".
	// +optional
	PullRequest *PullRequestSpec `json:"pullRequest,omitempty"`

	// ApprovalGates pause execution before specified events until human approval is recorded.
	// Grant approval by annotating the AgentTeam: kubectl annotate agentteam <name> approved.claude.amcheste.io/<event>=true
	// +optional
	ApprovalGates []ApprovalGateSpec `json:"approvalGates,omitempty"`

	// MaxRestarts bounds how many times each teammate pod may be re-spawned
	// after a Failed phase before the team itself is marked Failed. The lead
	// pod is not subject to this limit — a lead crash always fails the team.
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxRestarts *int32 `json:"maxRestarts,omitempty"`
}

// PullRequestSpec configures automatic PR creation.
type PullRequestSpec struct {
	// TargetBranch is the branch to open the PR against.
	// +kubebuilder:default="main"
	TargetBranch string `json:"targetBranch,omitempty"`

	// TitleTemplate is a Go template for the PR title.
	// Available variables: .TeamName, .Namespace
	TitleTemplate string `json:"titleTemplate,omitempty"`

	// Reviewers to request on the PR.
	// +optional
	Reviewers []string `json:"reviewers,omitempty"`

	// Labels to apply to the PR.
	// +optional
	Labels []string `json:"labels,omitempty"`
}

// QualityGateSpec configures validation steps.
type QualityGateSpec struct {
	// RequireTests ensures tests pass before completion.
	RequireTests bool `json:"requireTests,omitempty"`

	// RequireLint ensures linting passes before completion.
	RequireLint bool `json:"requireLint,omitempty"`

	// ValidationScript is a custom script to run before marking complete.
	// +optional
	ValidationScript string `json:"validationScript,omitempty"`
}

// ObservabilitySpec configures monitoring and notifications.
type ObservabilitySpec struct {
	// Metrics configures Prometheus metrics exposition.
	// +optional
	Metrics *MetricsSpec `json:"metrics,omitempty"`

	// LogLevel controls operator log verbosity for this team.
	// +kubebuilder:validation:Enum=debug;info;warn;error
	// +kubebuilder:default="info"
	LogLevel string `json:"logLevel,omitempty"`

	// Webhook configures event notifications.
	// +optional
	Webhook *WebhookSpec `json:"webhook,omitempty"`
}

// MetricsSpec configures Prometheus metrics.
type MetricsSpec struct {
	// Enabled turns on metrics exposition.
	Enabled bool `json:"enabled"`

	// Port for the metrics endpoint.
	// +kubebuilder:default=9090
	Port int32 `json:"port,omitempty"`
}

// WebhookSpec configures event notifications.
type WebhookSpec struct {
	// URL to POST events to.
	URL string `json:"url"`

	// Events to send notifications for.
	// +kubebuilder:validation:MinItems=1
	Events []string `json:"events"`
}

// --- Status Types ---

// AgentTeamStatus defines the observed state of an AgentTeam.
type AgentTeamStatus struct {
	// Phase is the current lifecycle phase of the team.
	// +kubebuilder:validation:Enum=Pending;Initializing;Running;Completed;Failed;TimedOut;BudgetExceeded
	Phase string `json:"phase,omitempty"`

	// StartedAt is when the team began execution.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// CompletedAt is when the team finished execution.
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// TotalTokensUsed is the estimated total tokens consumed.
	TotalTokensUsed int64 `json:"totalTokensUsed,omitempty"`

	// EstimatedCost is the estimated API cost in USD (e.g. "4.50").
	EstimatedCost string `json:"estimatedCost,omitempty"`

	// Ready reports how many teammate pods are ready vs. declared, in the form
	// "running+completed/total" (e.g. "3/5"). Shown in `kubectl get` output.
	// +optional
	Ready string `json:"ready,omitempty"`

	// Lead reports the team lead's status.
	// +optional
	Lead *AgentStatus `json:"lead,omitempty"`

	// Teammates reports each teammate's status.
	// +optional
	Teammates []TeammateStatus `json:"teammates,omitempty"`

	// Tasks reports aggregate task progress.
	// +optional
	Tasks *TaskSummary `json:"tasks,omitempty"`

	// PullRequest reports PR creation status.
	// +optional
	PullRequest *PullRequestStatus `json:"pullRequest,omitempty"`

	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// AgentStatus reports a single agent's state.
type AgentStatus struct {
	// PodName is the name of the agent's pod.
	PodName string `json:"podName,omitempty"`

	// Phase of this agent.
	// +kubebuilder:validation:Enum=Pending;Running;Idle;Completed;Failed;Waiting
	Phase string `json:"phase,omitempty"`
}

// TeammateStatus reports a teammate's state.
type TeammateStatus struct {
	AgentStatus `json:",inline"`

	// Name matches the teammate's spec name.
	Name string `json:"name"`

	// TasksCompleted is the number of tasks this teammate has finished.
	TasksCompleted int `json:"tasksCompleted,omitempty"`

	// TasksClaimed is the number of tasks currently owned by this teammate.
	TasksClaimed int `json:"tasksClaimed,omitempty"`

	// PendingApproval is the approval gate event this teammate is waiting on, if any.
	// +optional
	PendingApproval string `json:"pendingApproval,omitempty"`

	// RestartCount is the number of times this teammate's pod has been
	// re-spawned after a Failed phase. The team is marked Failed when any
	// teammate's RestartCount reaches Spec.Lifecycle.MaxRestarts.
	// +optional
	RestartCount int32 `json:"restartCount,omitempty"`
}

// TaskSummary reports aggregate task progress.
type TaskSummary struct {
	Total      int `json:"total"`
	Completed  int `json:"completed"`
	InProgress int `json:"inProgress"`
	Pending    int `json:"pending"`
}

// PullRequestStatus reports PR creation state.
type PullRequestStatus struct {
	URL   string `json:"url,omitempty"`
	State string `json:"state,omitempty"`
}

// --- Top-Level Resource Definitions ---

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Tasks Done",type=integer,JSONPath=`.status.tasks.completed`
// +kubebuilder:printcolumn:name="Cost",type=string,JSONPath=`.status.estimatedCost`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentTeam is the Schema for the agentteams API.
type AgentTeam struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentTeamSpec   `json:"spec,omitempty"`
	Status AgentTeamStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentTeamList contains a list of AgentTeam.
type AgentTeamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentTeam `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentTeam{}, &AgentTeamList{})
}
