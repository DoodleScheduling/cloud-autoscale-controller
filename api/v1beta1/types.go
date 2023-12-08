package v1beta1

import (
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Options struct {
	Suspend     bool                   `json:"suspend,omitempty"`
	ScaleToZero []metav1.LabelSelector `json:"scaleToZero,omitempty"`
	GracePeriod metav1.Duration        `json:"gracePeriod,omitempty"`
	Interval    metav1.Duration        `json:"interval,omitempty"`
}

type LocalObjectReference struct {
	Name string `json:"name,omitempty"`
}

const (
	Finalizer            = "finalizers.doodle.com"
	ConditionReady       = "Ready"
	ConditionReconciling = "Reconciling"
	ConditionPodsReady   = "PodsReady"
)

// ConditionalResource is a resource with conditions
type conditionalResource interface {
	GetStatusConditions() *[]metav1.Condition
}

// setResourceCondition sets the given condition with the given status,
// reason and message on a resource.
func setResourceCondition(resource conditionalResource, condition string, status metav1.ConditionStatus, reason, message string, generation int64) {
	conditions := resource.GetStatusConditions()

	newCondition := metav1.Condition{
		Type:               condition,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
	}

	apimeta.SetStatusCondition(conditions, newCondition)
}
