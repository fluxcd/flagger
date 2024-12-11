package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:validation:Enum=Ready

// HTTPScaledObjectCreationStatus describes the creation status
// of the scaler's additional resources such as Services, Ingresses and Deployments
type HTTPScaledObjectCreationStatus string

const (
	// Ready indicates the object is fully created
	Ready HTTPScaledObjectCreationStatus = "Ready"
)

// +kubebuilder:validation:Enum=ErrorCreatingAppScaledObject;AppScaledObjectCreated;TerminatingResources;AppScaledObjectTerminated;AppScaledObjectTerminationError;PendingCreation;HTTPScaledObjectIsReady;

// HTTPScaledObjectConditionReason describes the reason why the condition transitioned
type HTTPScaledObjectConditionReason string

const (
	ErrorCreatingAppScaledObject    HTTPScaledObjectConditionReason = "ErrorCreatingAppScaledObject"
	AppScaledObjectCreated          HTTPScaledObjectConditionReason = "AppScaledObjectCreated"
	TerminatingResources            HTTPScaledObjectConditionReason = "TerminatingResources"
	AppScaledObjectTerminated       HTTPScaledObjectConditionReason = "AppScaledObjectTerminated"
	AppScaledObjectTerminationError HTTPScaledObjectConditionReason = "AppScaledObjectTerminationError"
	PendingCreation                 HTTPScaledObjectConditionReason = "PendingCreation"
	HTTPScaledObjectIsReady         HTTPScaledObjectConditionReason = "HTTPScaledObjectIsReady"
)

// HTTPScaledObjectCondition stores the condition state
type HTTPScaledObjectCondition struct {
	// Timestamp of the condition
	// +optional
	Timestamp string `json:"timestamp" description:"Timestamp of this condition"`
	// Type of condition
	// +required
	Type HTTPScaledObjectCreationStatus `json:"type" description:"type of status condition"`
	// Status of the condition, one of True, False, Unknown.
	// +required
	Status metav1.ConditionStatus `json:"status" description:"status of the condition, one of True, False, Unknown"`
	// Reason for the condition's last transition.
	// +optional
	Reason HTTPScaledObjectConditionReason `json:"reason,omitempty" description:"one-word CamelCase reason for the condition's last transition"`
	// Message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty" description:"human-readable message indicating details about last transition"`
}

type Conditions []HTTPScaledObjectCondition

// GetReadyCondition returns Condition of type Ready
func (c *Conditions) GetReadyCondition() HTTPScaledObjectCondition {
	if *c == nil {
		c = GetInitializedConditions()
	}
	return c.getCondition(Ready)
}

// GetInitializedConditions returns Conditions initialized to the default -> Status: Unknown
func GetInitializedConditions() *Conditions {
	return &Conditions{{Type: Ready, Status: metav1.ConditionUnknown}}
}

// IsTrue is true if the condition is True
func (c *HTTPScaledObjectCondition) IsTrue() bool {
	if c == nil {
		return false
	}
	return c.Status == metav1.ConditionTrue
}

// IsFalse is true if the condition is False
func (c *HTTPScaledObjectCondition) IsFalse() bool {
	if c == nil {
		return false
	}
	return c.Status == metav1.ConditionFalse
}

func (c Conditions) getCondition(conditionType HTTPScaledObjectCreationStatus) HTTPScaledObjectCondition {
	for i := range c {
		if c[i].Type == conditionType {
			return c[i]
		}
	}
	return HTTPScaledObjectCondition{}
}
