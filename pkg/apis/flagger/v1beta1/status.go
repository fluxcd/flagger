package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CanaryConditionType is the type of a CanaryCondition
type CanaryConditionType string

const (
	// PromotedType refers to the result of the last canary analysis
	PromotedType CanaryConditionType = "Promoted"
)

// CanaryCondition is a status condition for a Canary
type CanaryCondition struct {
	// Type of this condition
	Type CanaryConditionType `json:"type"`

	// Status of this condition
	Status corev1.ConditionStatus `json:"status"`

	// LastUpdateTime of this condition
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// LastTransitionTime of this condition
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason for the current status of this condition
	Reason string `json:"reason,omitempty"`

	// Message associated with this condition
	Message string `json:"message,omitempty"`
}

// CanaryPhase is a label for the condition of a canary at the current time
type CanaryPhase string

const (
	// CanaryPhaseInitializing means the canary initializing is underway
	CanaryPhaseInitializing CanaryPhase = "Initializing"
	// CanaryPhaseInitialized means the primary deployment, hpa and ClusterIP services
	// have been created along with the service mesh or ingress objects
	CanaryPhaseInitialized CanaryPhase = "Initialized"
	// CanaryPhaseWaiting means the canary rollout is paused (waiting for confirmation to proceed)
	CanaryPhaseWaiting CanaryPhase = "Waiting"
	// CanaryPhaseProgressing means the canary analysis is underway
	CanaryPhaseProgressing CanaryPhase = "Progressing"
	// CanaryPhasePromoting means the canary analysis is finished and the primary spec has been updated
	CanaryPhasePromoting CanaryPhase = "Promoting"
	// CanaryPhaseProgressing means the canary promotion is finished and traffic has been routed back to primary
	CanaryPhaseFinalising CanaryPhase = "Finalising"
	// CanaryPhaseSucceeded means the canary analysis has been successful
	// and the canary deployment has been promoted
	CanaryPhaseSucceeded CanaryPhase = "Succeeded"
	// CanaryPhaseFailed means the canary analysis failed
	// and the canary deployment has been scaled to zero
	CanaryPhaseFailed CanaryPhase = "Failed"
	// CanaryPhaseTerminating means the canary has been marked
	// for deletion and in the finalizing state
	CanaryPhaseTerminating CanaryPhase = "Terminating"
	// CanaryPhaseTerminated means the canary has been finalized
	// and successfully deleted
	CanaryPhaseTerminated CanaryPhase = "Terminated"
)

// CanaryStatus is used for state persistence (read-only)
type CanaryStatus struct {
	Phase        CanaryPhase `json:"phase"`
	FailedChecks int         `json:"failedChecks"`
	CanaryWeight int         `json:"canaryWeight"`
	Iterations   int         `json:"iterations"`
	// +optional
	TrackedConfigs *map[string]string `json:"trackedConfigs,omitempty"`
	// +optional
	LastAppliedSpec string `json:"lastAppliedSpec,omitempty"`
	// +optional
	LastPromotedSpec string `json:"lastPromotedSpec,omitempty"`
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// +optional
	Conditions []CanaryCondition `json:"conditions,omitempty"`
}
