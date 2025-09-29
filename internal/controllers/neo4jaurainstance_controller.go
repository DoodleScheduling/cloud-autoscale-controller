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

	infrav1beta1 "github.com/doodlescheduling/cloud-autoscale-controller/api/v1beta1"
	auraclient "github.com/doodlescheduling/neo4j-aura-controller/pkg/aura/client"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/go-logr/logr"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
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

//+kubebuilder:rbac:groups=cloudautoscale.infra.doodle.com,resources=neo4jaurainstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloudautoscale.infra.doodle.com,resources=neo4jaurainstances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloudautoscale.infra.doodle.com,resources=neo4jaurainstances/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Neo4jAuraInstanceReconciler reconciles a Namespace object
type Neo4jAuraInstanceReconciler struct {
	client.Client
	HTTPClient *http.Client
	Log        logr.Logger
	Recorder   record.EventRecorder
}

type Neo4jAuraInstanceReconcilerOptions struct {
	MaxConcurrentReconciles int
}

// SetupWithManager sets up the controller with the Manager.
func (r *Neo4jAuraInstanceReconciler) SetupWithManager(mgr ctrl.Manager, opts Neo4jAuraInstanceReconcilerOptions) error {
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &infrav1beta1.Neo4jAuraInstance{}, secretIndexKey,
		func(o client.Object) []string {
			instance := o.(*infrav1beta1.Neo4jAuraInstance)
			keys := []string{}

			if instance.Spec.Secret.Name != "" {
				keys = []string{
					fmt.Sprintf("%s/%s", instance.GetNamespace(), instance.Spec.Secret.Name),
				}
			}

			return keys
		},
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1beta1.Neo4jAuraInstance{}, builder.WithPredicates(
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

func (r *Neo4jAuraInstanceReconciler) requestsForSecretChange(ctx context.Context, o client.Object) []reconcile.Request {
	secret, ok := o.(*corev1.Secret)
	if !ok {
		panic(fmt.Sprintf("expected a Secret, got %T", o))
	}

	var list infrav1beta1.Neo4jAuraInstanceList
	if err := r.List(ctx, &list, client.MatchingFields{
		secretIndexKey: objectKey(secret).String(),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, instance := range list.Items {
		r.Log.V(1).Info("referenced secret from a Neo4jAuraInstance changed detected", "namespace", instance.GetNamespace(), "name", instance.GetName())
		reqs = append(reqs, reconcile.Request{NamespacedName: objectKey(&instance)})
	}

	return reqs
}

func (r *Neo4jAuraInstanceReconciler) requestsForChangeBySelector(ctx context.Context, o client.Object) []reconcile.Request {
	var list infrav1beta1.Neo4jAuraInstanceList
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

func (r *Neo4jAuraInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("namespace", req.Namespace, "name", req.Name)

	instance := infrav1beta1.Neo4jAuraInstance{}
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
		logger.Error(err, "reconcile error occurred")
		instance = infrav1beta1.Neo4jAuraInstanceReady(instance, metav1.ConditionFalse, "ReconciliationFailed", err.Error())
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

func (r *Neo4jAuraInstanceReconciler) reconcile(ctx context.Context, instance infrav1beta1.Neo4jAuraInstance, logger logr.Logger) (infrav1beta1.Neo4jAuraInstance, ctrl.Result, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Name:      instance.Spec.Secret.Name,
		Namespace: instance.Namespace,
	}, &secret); err != nil {
		return instance, reconcile.Result{}, err
	}

	opts := auraOptions{
		Neo4jAuraInstanceSpec: instance.Spec,
	}

	if val, ok := secret.Data["clientID"]; !ok {
		return instance, ctrl.Result{}, errors.New("clientID not found in secret")
	} else {
		opts.ClientID = string(val)
	}

	if val, ok := secret.Data["clientSecret"]; !ok {
		return instance, ctrl.Result{}, errors.New("clientSecret not found in secret")
	} else {
		opts.ClientSecret = string(val)
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
		instance = infrav1beta1.Neo4jAuraInstanceScaledToZero(instance, metav1.ConditionFalse, "PodsRunning", "selector matches at least one running pod")
	} else {
		instance = infrav1beta1.Neo4jAuraInstanceScaledToZero(instance, metav1.ConditionTrue, "PodsNotRunning", "no running pods detected")
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

		logger.Info("make sure aura instance is suspended", "instance", opts.instanceName)
		res, err = r.suspend(ctx, logger, opts)

		if err == nil {
			instance = infrav1beta1.Neo4jAuraInstanceReady(instance, metav1.ConditionTrue, "ReconciliationSuccessful", "aura instance suspended")
		}
	} else {
		logger.Info("make sure aura instance is resumed", "instance", opts.instanceName)
		res, err = r.resume(ctx, logger, opts)

		if err == nil {
			instance = infrav1beta1.Neo4jAuraInstanceReady(instance, metav1.ConditionTrue, "ReconciliationSuccessful", "aura instance suspended")
		}
	}

	return instance, res, err
}

func (r *Neo4jAuraInstanceReconciler) hasRunningPods(ctx context.Context, instance infrav1beta1.Neo4jAuraInstance, logger logr.Logger) (bool, error) {
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

type auraOptions struct {
	infrav1beta1.Neo4jAuraInstanceSpec
	instanceName string
	ClientID     string
	ClientSecret string
}

func (r *Neo4jAuraInstanceReconciler) httpClient(ctx context.Context, opts auraOptions) (*http.Client, error) {
	conf := &clientcredentials.Config{
		ClientID:     opts.ClientID,
		ClientSecret: opts.ClientSecret,
		TokenURL:     "https://api.neo4j.io/oauth/token",
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, r.HTTPClient)
	tokenSource := conf.TokenSource(ctx)
	transport := &oauth2.Transport{
		Source: tokenSource,
		Base:   r.HTTPClient.Transport,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   r.HTTPClient.Timeout,
	}, nil
}

func (r *Neo4jAuraInstanceReconciler) resume(ctx context.Context, logger logr.Logger, opts auraOptions) (ctrl.Result, error) {
	client, instance, err := r.initClient(ctx, opts)
	if err != nil {
		return ctrl.Result{}, err
	}

	if instance.Data.Status == auraclient.InstanceDataStatusPaused || instance.Data.Status == auraclient.InstanceDataStatusPausing {
		logger.Info("resume aura instance")
		_, err := client.PostResumeInstanceWithResponse(ctx, instance.Data.Id, auraclient.PostResumeInstanceJSONRequestBody{})
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *Neo4jAuraInstanceReconciler) initClient(ctx context.Context, opts auraOptions) (*auraclient.ClientWithResponses, *auraclient.Instance, error) {
	httpClient, err := r.httpClient(ctx, opts)
	if err != nil {
		return nil, nil, err
	}

	auraClient, err := auraclient.NewClientWithResponses("https://api.neo4j.io/v1", auraclient.WithHTTPClient(httpClient))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create aura client: %w", err)
	}

	params := auraclient.GetInstancesParams{}

	auraInstances, err := auraClient.GetInstancesWithResponse(ctx, &params)
	if err != nil {
		return auraClient, nil, err
	}

	if auraInstances.StatusCode() != http.StatusOK {
		return auraClient, nil, fmt.Errorf("failed to get instance list, request failed with code %d - %s", auraInstances.StatusCode(), auraInstances.Body)
	}

	for _, remoteInstance := range auraInstances.JSON200.Data {
		if opts.instanceName != remoteInstance.Name {
			continue
		}

		auraInstance, err := auraClient.GetInstanceIdWithResponse(ctx, remoteInstance.Id)
		if err != nil {
			return auraClient, nil, fmt.Errorf("failed to get instance: %w", err)
		}

		if auraInstances.StatusCode() != http.StatusOK {
			return auraClient, nil, fmt.Errorf("failed to get instance list, request failed with code %d - %s", auraInstances.StatusCode(), auraInstances.Body)
		}

		return auraClient, auraInstance.JSON200, nil
	}

	return nil, nil, errors.New("instance not found")
}

func (r *Neo4jAuraInstanceReconciler) suspend(ctx context.Context, logger logr.Logger, opts auraOptions) (ctrl.Result, error) {
	client, instance, err := r.initClient(ctx, opts)
	if err != nil {
		return ctrl.Result{}, err
	}

	if instance.Data.Status == "running" {
		logger.Info("suspend aura instance")
		_, err := client.PostPauseInstanceWithResponse(ctx, instance.Data.Id, auraclient.PostPauseInstanceJSONRequestBody{})
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *Neo4jAuraInstanceReconciler) patchStatus(ctx context.Context, instance *infrav1beta1.Neo4jAuraInstance) error {
	key := client.ObjectKeyFromObject(instance)
	latest := &infrav1beta1.Neo4jAuraInstance{}
	if err := r.Get(ctx, key, latest); err != nil {
		return err
	}

	return r.Status().Patch(ctx, instance, client.MergeFrom(latest))
}
