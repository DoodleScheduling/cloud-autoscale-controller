/*
Copyright 2022 Doodle.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	infrav1beta1 "github.com/doodlescheduling/cloud-autoscale-controller/api/v1beta1"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	secretIndexKey = ".metadata.secret"
)

//+kubebuilder:rbac:groups=cloudautoscale.infra.doodle.com,resources=awsrdsinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudautoscale.infra.doodle.com,resources=awsrdsinstances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudautoscale.infra.doodle.com,resources=awsrdsinstances/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// AWSRDSInstanceReconciler reconciles a Namespace object
type AWSRDSInstanceReconciler struct {
	client.Client
	HTTPClient *http.Client
	Log        logr.Logger
	Recorder   record.EventRecorder
}

type AWSRDSInstanceReconcilerOptions struct {
	MaxConcurrentReconciles int
}

// SetupWithManager sets up the controller with the Manager.
func (r *AWSRDSInstanceReconciler) SetupWithManager(mgr ctrl.Manager, opts AWSRDSInstanceReconcilerOptions) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1beta1.AWSRDSInstance{}, builder.WithPredicates(
			predicate.GenerationChangedPredicate{},
		)).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForChangeBySelector),
		).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForSecretChange),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: opts.MaxConcurrentReconciles}).
		Complete(r)
}

func (r *AWSRDSInstanceReconciler) requestsForSecretChange(ctx context.Context, o client.Object) []reconcile.Request {
	sectet, ok := o.(*corev1.Secret)
	if !ok {
		panic(fmt.Sprintf("expected a Secret, got %T", o))
	}

	var list infrav1beta1.AWSRDSInstanceList
	if err := r.List(ctx, &list, client.MatchingFields{
		secretIndexKey: objectKey(sectet).String(),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, instance := range list.Items {
		r.Log.V(1).Info("referenced secret from a AWSRDSInstance changed detected", "namespace", instance.GetNamespace(), "name", instance.GetName())
		reqs = append(reqs, reconcile.Request{NamespacedName: objectKey(&instance)})
	}

	return reqs
}

func (r *AWSRDSInstanceReconciler) requestsForChangeBySelector(ctx context.Context, o client.Object) []reconcile.Request {
	var list infrav1beta1.AWSRDSInstanceList
	if err := r.List(ctx, &list, client.InNamespace(o.GetNamespace())); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, instance := range list.Items {
		for _, selector := range instance.Spec.ScaleToZero {
			labelSel, err := metav1.LabelSelectorAsSelector(&selector)
			if err != nil {
				r.Log.Error(err, "can not select scaleToZero selectors")
				continue
			}

			if labelSel.Matches(labels.Set(o.GetLabels())) {
				r.Log.V(1).Info("change of referenced resource detected", "namespace", o.GetNamespace(), "name", o.GetName(), "kind", o.GetObjectKind().GroupVersionKind().Kind, "resource", instance.GetName())
				reqs = append(reqs, reconcile.Request{NamespacedName: objectKey(&instance)})
			}
		}
	}

	return reqs
}

func (r *AWSRDSInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("namespace", req.Namespace, "name", req.Name)

	instance := infrav1beta1.AWSRDSInstance{}
	err := r.Get(ctx, req.NamespacedName, &instance)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if instance.Spec.Suspend {
		return ctrl.Result{}, nil
	}

	instance, result, err := r.reconcile(ctx, instance, logger)
	instance.Status.ObservedGeneration = instance.GetGeneration()

	if err != nil {
		logger.Error(err, "reconcile error occured")
		instance = infrav1beta1.AWSRDSInstanceReady(instance, metav1.ConditionFalse, "ReconciliationFailed", err.Error())
		r.Recorder.Event(&instance, "Normal", "error", err.Error())
	}

	// Update status after reconciliation.
	if err := r.patchStatus(ctx, &instance); err != nil {
		logger.Error(err, "unable to update status after reconciliation")
		return ctrl.Result{}, err
	}

	if err == nil && instance.Spec.Interval.Duration != 0 {
		result.RequeueAfter = instance.Spec.Interval.Duration
	}

	return result, err

}

func (r *AWSRDSInstanceReconciler) reconcile(ctx context.Context, instance infrav1beta1.AWSRDSInstance, logger logr.Logger) (infrav1beta1.AWSRDSInstance, ctrl.Result, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Name:      instance.Spec.Secret.Name,
		Namespace: instance.Namespace,
	}, &secret); err != nil {
		return instance, reconcile.Result{}, err
	}

	opts := rdsOptions{
		AWSRDSInstanceSpec: instance.Spec,
	}

	if val, ok := secret.Data["AWS_ACCESS_KEY_ID"]; !ok {
		return instance, ctrl.Result{}, errors.New("AWS_ACCESS_KEY_ID not found in secret")
	} else {
		opts.AccessKeyID = string(val)
	}

	if val, ok := secret.Data["AWS_SECRET_ACCESS_KEY"]; !ok {
		return instance, ctrl.Result{}, errors.New("AWS_SECRET_ACCESS_KEY not found in secret")
	} else {
		opts.SecretAccessKey = string(val)
	}

	if instance.Spec.InstanceName == "" {
		opts.instanceName = instance.Name
	} else {
		opts.instanceName = instance.Spec.InstanceName
	}

	logger = logger.WithValues("instance", opts.instanceName)
	podsRunning, err := r.hasRunningPods(ctx, instance, logger)

	if err != nil {
		return instance, ctrl.Result{}, err
	}

	if podsRunning {
		instance = infrav1beta1.AWSRDSInstanceScaledToZero(instance, metav1.ConditionFalse, "PodsRunning", "selector matches at least one running pod")
	} else {
		instance = infrav1beta1.AWSRDSInstanceScaledToZero(instance, metav1.ConditionTrue, "PodsNotRunning", "no running pods detected")
	}

	suspend := !podsRunning

	var (
		res ctrl.Result
	)

	scaledToZeroCondition := conditions.Get(&instance, infrav1beta1.ConditionScaledToZero)

	if suspend {
		if scaledToZeroCondition != nil && scaledToZeroCondition.Status == metav1.ConditionTrue {
			if time.Since(scaledToZeroCondition.LastTransitionTime.Time) >= instance.Spec.GracePeriod.Duration {
				logger.Info("pod scaled to zero grace period reached", "grace-period", instance.Spec.GracePeriod)
			} else {
				logger.V(1).Info("pod scaled to zero grace period in progress", "grace-period", instance.Spec.GracePeriod)
				return instance, reconcile.Result{
					RequeueAfter: instance.Spec.GracePeriod.Duration,
				}, nil
			}
		}

		logger.Info("make sure RDS instances are suspended", "instance", opts.instanceName)
		res, err = r.suspend(ctx, logger, opts)

		if err == nil {
			instance = infrav1beta1.AWSRDSInstanceReady(instance, metav1.ConditionTrue, "ReconciliationSuccessful", "rds instance suspended")
		}
	} else {
		logger.Info("make sure RDS instances are resumed", "instance", opts.instanceName)
		res, err = r.resume(ctx, logger, opts)

		if err == nil {
			instance = infrav1beta1.AWSRDSInstanceReady(instance, metav1.ConditionTrue, "ReconciliationSuccessful", "rds instance suspended")
		}
	}

	return instance, res, err
}

func (r *AWSRDSInstanceReconciler) hasRunningPods(ctx context.Context, instance infrav1beta1.AWSRDSInstance, logger logr.Logger) (bool, error) {
	if len(instance.Spec.ScaleToZero) == 0 {
		return true, nil
	}

	for _, selector := range instance.Spec.ScaleToZero {
		selector, err := metav1.LabelSelectorAsSelector(&selector)
		if err != nil {
			return false, err
		}

		var list corev1.PodList
		if err := r.List(ctx, &list, client.InNamespace(instance.Namespace), &client.MatchingLabelsSelector{Selector: selector}); err != nil {
			return false, err
		}

		//Compatibility to DoodleScheduling/k8s-pause, otherwise no pods would be sufficient
		for _, pod := range list.Items {
			if pod.Status.Phase != "Suspended" {
				return true, nil
			}
		}
	}

	return false, nil
}

type rdsOptions struct {
	infrav1beta1.AWSRDSInstanceSpec
	instanceName    string
	AccessKeyID     string
	SecretAccessKey string
}

func (r *AWSRDSInstanceReconciler) resume(ctx context.Context, logger logr.Logger, opts rdsOptions) (ctrl.Result, error) {
	client, output, err := r.initClient(ctx, opts)
	if err != nil {
		return ctrl.Result{}, err
	}

	if *output.DBInstances[0].DBInstanceStatus == "stopped" {
		logger.Info("resume rds instance")
		input := &rds.StartDBInstanceInput{
			DBInstanceIdentifier: &opts.instanceName,
		}
		_, err := client.StartDBInstance(ctx, input)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *AWSRDSInstanceReconciler) initClient(ctx context.Context, opts rdsOptions) (*rds.Client, *rds.DescribeDBInstancesOutput, error) {
	var provider aws.CredentialsProviderFunc = func(ctx context.Context) (aws.Credentials, error) {
		return aws.Credentials{
			AccessKeyID:     opts.AccessKeyID,
			SecretAccessKey: opts.SecretAccessKey,
		}, nil
	}

	rdsOpts := rds.Options{
		Region:      opts.Region,
		Credentials: provider,
		HTTPClient:  r.HTTPClient,
	}

	client := rds.New(rdsOpts)

	input := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: &opts.instanceName,
	}

	output, err := client.DescribeDBInstances(ctx, input)
	if err != nil {
		return client, output, err
	}

	if len(output.DBInstances) != 1 {
		return client, output, errors.New("no such rds instance found")
	}

	return client, output, nil
}

func (r *AWSRDSInstanceReconciler) suspend(ctx context.Context, logger logr.Logger, opts rdsOptions) (ctrl.Result, error) {
	client, output, err := r.initClient(ctx, opts)
	if err != nil {
		return ctrl.Result{}, err
	}

	if *output.DBInstances[0].DBInstanceStatus == "available" {
		logger.Info("suspend rds instance")
		input := &rds.StopDBInstanceInput{
			DBInstanceIdentifier: &opts.instanceName,
		}
		_, err := client.StopDBInstance(ctx, input)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *AWSRDSInstanceReconciler) patchStatus(ctx context.Context, instance *infrav1beta1.AWSRDSInstance) error {
	key := client.ObjectKeyFromObject(instance)
	latest := &infrav1beta1.AWSRDSInstance{}
	if err := r.Get(ctx, key, latest); err != nil {
		return err
	}

	return r.Status().Patch(ctx, instance, client.MergeFrom(latest))
}
