package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentTeamTemplateSpec defines a reusable team pattern.
type AgentTeamTemplateSpec struct {
	// Description explains the template's purpose.
	Description string `json:"description,omitempty"`

	// Teammates defines the worker agents in the template.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	Teammates []TeammateSpec `json:"teammates"`

	// Coordination configures how agents communicate.
	// +optional
	Coordination *CoordinationSpec `json:"coordination,omitempty"`

	// Lifecycle configures default runtime behavior.
	// +optional
	Lifecycle *LifecycleSpec `json:"lifecycle,omitempty"`

	// QualityGates configures default validation steps.
	// +optional
	QualityGates *QualityGateSpec `json:"qualityGates,omitempty"`
}

// AgentTeamTemplateStatus reports validation state for an AgentTeamTemplate.
// The reconciler validates teammate references and writes a Ready condition;
// AgentTeamRun controllers should refuse to instantiate templates where
// Ready is false.
type AgentTeamTemplateStatus struct {
	// Ready is true when the template has passed validation and is safe to
	// instantiate via an AgentTeamRun.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// Conditions track the latest validation state with structured reasons.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Description",type=string,JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="Teammates",type=integer,JSONPath=`.spec.teammates`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`

// AgentTeamTemplate is a reusable team definition.
type AgentTeamTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentTeamTemplateSpec   `json:"spec,omitempty"`
	Status AgentTeamTemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentTeamTemplateList contains a list of AgentTeamTemplate.
type AgentTeamTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentTeamTemplate `json:"items"`
}

// --- AgentTeamRun ---

// AgentTeamRunSpec defines an instance of a template applied to a specific repo.
type AgentTeamRunSpec struct {
	// TemplateRef references the AgentTeamTemplate to instantiate.
	TemplateRef TemplateReference `json:"templateRef"`

	// Repository configuration for this run (coding mode).
	// +optional
	Repository *RepositorySpec `json:"repository,omitempty"`

	// Workspace configures inputs/outputs for this run (Cowork mode).
	// +optional
	Workspace *WorkspaceSpec `json:"workspace,omitempty"`

	// Auth configures API authentication for this run.
	Auth AuthSpec `json:"auth"`

	// Lead configures the team lead for this run.
	Lead LeadSpec `json:"lead"`

	// Lifecycle overrides for this run.
	// +optional
	Lifecycle *LifecycleSpec `json:"lifecycle,omitempty"`
}

// TemplateReference points to an AgentTeamTemplate.
type TemplateReference struct {
	// Name of the AgentTeamTemplate in the same namespace.
	Name string `json:"name"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=`.spec.templateRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentTeamRun is an instance of an AgentTeamTemplate applied to a specific repository.
type AgentTeamRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentTeamRunSpec `json:"spec,omitempty"`
	Status AgentTeamStatus  `json:"status,omitempty"` // Reuses AgentTeamStatus
}

// +kubebuilder:object:root=true

// AgentTeamRunList contains a list of AgentTeamRun.
type AgentTeamRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentTeamRun `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&AgentTeamTemplate{}, &AgentTeamTemplateList{},
		&AgentTeamRun{}, &AgentTeamRunList{},
	)
}
