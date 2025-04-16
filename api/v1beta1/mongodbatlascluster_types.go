package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type MongoDBAtlasCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MongoDBAtlasClusterSpec   `json:"spec,omitempty"`
	Status MongoDBAtlasClusterStatus `json:"status,omitempty"`
}

type MongoDBAtlasClusterSpec struct {
	Suspend     bool                   `json:"suspend,omitempty"`
	ScaleToZero []metav1.LabelSelector `json:"scaleToZero,omitempty"`
	GracePeriod metav1.Duration        `json:"gracePeriod,omitempty"`
	Interval    metav1.Duration        `json:"interval,omitempty"`
	GroupID     string                 `json:"groupID,omitempty"`
	Secret      LocalObjectReference   `json:"secret,omitempty"`
	ClusterName string                 `json:"clusterName,omitempty"`
}

// KeycloakClusterList contains a list of KeycloakCluster.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type MongoDBAtlasClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MongoDBAtlasCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MongoDBAtlasCluster{}, &MongoDBAtlasClusterList{})
}

type MongoDBAtlasClusterStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the last generation reconciled by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

func MongoDBAtlasClusterReconciling(set MongoDBAtlasCluster, status metav1.ConditionStatus, reason, message string) MongoDBAtlasCluster {
	setResourceCondition(&set, ConditionReconciling, status, reason, message, set.Generation)
	return set
}

func MongoDBAtlasClusterReady(set MongoDBAtlasCluster, status metav1.ConditionStatus, reason, message string) MongoDBAtlasCluster {
	setResourceCondition(&set, ConditionReady, status, reason, message, set.Generation)
	return set
}

func MongoDBAtlasClusterScaledToZero(set MongoDBAtlasCluster, status metav1.ConditionStatus, reason, message string) MongoDBAtlasCluster {
	setResourceCondition(&set, ConditionScaledToZero, status, reason, message, set.Generation)
	return set
}

// GetStatusConditions returns a pointer to the Status.Conditions slice
func (in *MongoDBAtlasCluster) GetStatusConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

func (in *MongoDBAtlasCluster) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

func (in *MongoDBAtlasCluster) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}
