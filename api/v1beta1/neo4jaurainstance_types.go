package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Neo4jAuraInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   Neo4jAuraInstanceSpec   `json:"spec,omitempty"`
	Status Neo4jAuraInstanceStatus `json:"status,omitempty"`
}

type Neo4jAuraInstanceSpec struct {
	Suspend      bool                   `json:"suspend,omitempty"`
	ScaleToZero  []metav1.LabelSelector `json:"scaleToZero,omitempty"`
	GracePeriod  metav1.Duration        `json:"gracePeriod,omitempty"`
	Interval     metav1.Duration        `json:"interval,omitempty"`
	Secret       LocalObjectReference   `json:"secret,omitempty"`
	InstanceName string                 `json:"instanceName,omitempty"`
}

// KeycloakClusterList contains a list of KeycloakCluster.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Neo4jAuraInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Neo4jAuraInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Neo4jAuraInstance{}, &Neo4jAuraInstanceList{})
}

type Neo4jAuraInstanceStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the last generation reconciled by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

func Neo4jAuraInstanceReconciling(set Neo4jAuraInstance, status metav1.ConditionStatus, reason, message string) Neo4jAuraInstance {
	setResourceCondition(&set, ConditionReconciling, status, reason, message, set.Generation)
	return set
}

func Neo4jAuraInstanceReady(set Neo4jAuraInstance, status metav1.ConditionStatus, reason, message string) Neo4jAuraInstance {
	setResourceCondition(&set, ConditionReady, status, reason, message, set.Generation)
	return set
}

func Neo4jAuraInstanceScaledToZero(set Neo4jAuraInstance, status metav1.ConditionStatus, reason, message string) Neo4jAuraInstance {
	setResourceCondition(&set, ConditionScaledToZero, status, reason, message, set.Generation)
	return set
}

// GetStatusConditions returns a pointer to the Status.Conditions slice
func (in *Neo4jAuraInstance) GetStatusConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

func (in *Neo4jAuraInstance) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

func (in *Neo4jAuraInstance) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}
