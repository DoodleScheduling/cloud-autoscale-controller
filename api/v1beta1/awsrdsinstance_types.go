package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type AWSRDSInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AWSRDSInstanceSpec   `json:"spec,omitempty"`
	Status AWSRDSInstanceStatus `json:"status,omitempty"`
}

type AWSRDSInstanceSpec struct {
	Suspend      bool                   `json:"suspend,omitempty"`
	ScaleToZero  []metav1.LabelSelector `json:"scaleToZero,omitempty"`
	GracePeriod  metav1.Duration        `json:"gracePeriod,omitempty"`
	Interval     metav1.Duration        `json:"interval,omitempty"`
	Secret       LocalObjectReference   `json:"secret,omitempty"`
	Region       string                 `json:"region,omitempty"`
	InstanceName string                 `json:"instanceName,omitempty"`
}

// KeycloakClusterList contains a list of KeycloakCluster.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AWSRDSInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AWSRDSInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AWSRDSInstance{}, &AWSRDSInstanceList{})
}

type AWSRDSInstanceStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the last generation reconciled by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

func AWSRDSInstanceReconciling(set AWSRDSInstance, status metav1.ConditionStatus, reason, message string) AWSRDSInstance {
	setResourceCondition(&set, ConditionReconciling, status, reason, message, set.Generation)
	return set
}

func AWSRDSInstanceReady(set AWSRDSInstance, status metav1.ConditionStatus, reason, message string) AWSRDSInstance {
	setResourceCondition(&set, ConditionReady, status, reason, message, set.Generation)
	return set
}

func AWSRDSInstanceScaledToZero(set AWSRDSInstance, status metav1.ConditionStatus, reason, message string) AWSRDSInstance {
	setResourceCondition(&set, ConditionScaledToZero, status, reason, message, set.Generation)
	return set
}

// GetStatusConditions returns a pointer to the Status.Conditions slice
func (in *AWSRDSInstance) GetStatusConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

func (in *AWSRDSInstance) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

func (in *AWSRDSInstance) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}
