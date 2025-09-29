package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/doodlescheduling/cloud-autoscale-controller/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = Describe("MongoDBAtlasCluster controller", func() {
	const (
		timeout  = time.Second * 4
		interval = time.Millisecond * 600
	)

	When("reconciling a suspended MongoDBAtlasCluster", func() {
		instanceName := fmt.Sprintf("cluster-%s", rand.String(5))

		It("should not update the status", func() {
			By("creating a new MongoDBAtlasCluster")
			ctx := context.Background()

			gi := &v1beta1.MongoDBAtlasCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: "default",
				},
				Spec: v1beta1.MongoDBAtlasClusterSpec{
					Suspend: true,
				},
			}
			Expect(k8sClient.Create(ctx, gi)).Should(Succeed())

			By("waiting for the reconciliation")
			instanceLookupKey := types.NamespacedName{Name: instanceName, Namespace: "default"}
			reconciledInstance := &v1beta1.MongoDBAtlasCluster{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, instanceLookupKey, reconciledInstance)
				if err != nil {
					return false
				}

				return len(reconciledInstance.Status.Conditions) == 0
			}, timeout, interval).Should(BeTrue())
		})
	})

	instanceName := fmt.Sprintf("instance-%s", rand.String(5))
	secretName := fmt.Sprintf("secret-%s", rand.String(5))

	When("it can't find the referenced secret with credentials", func() {
		It("should update the status", func() {
			By("creating a new MongoDBAtlasCluster")
			ctx := context.Background()

			gi := &v1beta1.MongoDBAtlasCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: "default",
				},
				Spec: v1beta1.MongoDBAtlasClusterSpec{
					GroupID: "x",
					Secret: v1beta1.LocalObjectReference{
						Name: secretName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, gi)).Should(Succeed())

			By("waiting for the reconciliation")
			instanceLookupKey := types.NamespacedName{Name: instanceName, Namespace: "default"}
			reconciledInstance := &v1beta1.MongoDBAtlasCluster{}

			expectedStatus := &v1beta1.MongoDBAtlasClusterStatus{
				ObservedGeneration: 1,
				Conditions: []metav1.Condition{
					{
						Type:    v1beta1.ConditionReady,
						Status:  metav1.ConditionFalse,
						Reason:  "ReconciliationFailed",
						Message: fmt.Sprintf(`Secret "%s" not found`, secretName),
					},
				},
			}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, instanceLookupKey, reconciledInstance)
				if err != nil {
					return false
				}

				return needConditions(expectedStatus.Conditions, reconciledInstance.Status.Conditions)
			}, timeout, interval).Should(BeTrue())
		})
	})

	When("it can't find the public key from the secret", func() {
		It("should update the status", func() {
			By("creating a secret")
			ctx := context.Background()

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: "default",
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			By("waiting for the reconciliation")
			instanceLookupKey := types.NamespacedName{Name: instanceName, Namespace: "default"}
			reconciledInstance := &v1beta1.MongoDBAtlasCluster{}

			expectedStatus := &v1beta1.MongoDBAtlasClusterStatus{
				ObservedGeneration: 1,
				Conditions: []metav1.Condition{
					{
						Type:    v1beta1.ConditionReady,
						Status:  metav1.ConditionFalse,
						Reason:  "ReconciliationFailed",
						Message: "publicKey not found in secret",
					},
				},
			}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, instanceLookupKey, reconciledInstance)
				if err != nil {
					return false
				}

				return needConditions(expectedStatus.Conditions, reconciledInstance.Status.Conditions)
			}, timeout, interval).Should(BeTrue())
		})
	})

	When("it can't find the private key from the secret", func() {
		It("should update the status", func() {
			By("creating a secret")
			ctx := context.Background()

			var secret corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "default"}, &secret)).Should(Succeed())

			secret.StringData = map[string]string{"publicKey": "id"}
			Expect(k8sClient.Update(ctx, &secret)).Should(Succeed())

			By("waiting for the reconciliation")
			instanceLookupKey := types.NamespacedName{Name: instanceName, Namespace: "default"}
			reconciledInstance := &v1beta1.MongoDBAtlasCluster{}

			expectedStatus := &v1beta1.MongoDBAtlasClusterStatus{
				ObservedGeneration: 1,
				Conditions: []metav1.Condition{
					{
						Type:    v1beta1.ConditionReady,
						Status:  metav1.ConditionFalse,
						Reason:  "ReconciliationFailed",
						Message: "privateKey not found in secret",
					},
				},
			}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, instanceLookupKey, reconciledInstance)
				if err != nil {
					return false
				}

				return needConditions(expectedStatus.Conditions, reconciledInstance.Status.Conditions)
			}, timeout, interval).Should(BeTrue())
		})
	})

	When("no such atlas cluster found and no pod selector set", func() {
		It("should report Ready=false and ScaledToZero=false", func() {
			By("creating a secret")
			ctx := context.Background()

			var secret corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "default"}, &secret)).Should(Succeed())

			secret.StringData = map[string]string{"privateKey": "id"}
			Expect(k8sClient.Update(ctx, &secret)).Should(Succeed())

			By("waiting for the reconciliation")
			instanceLookupKey := types.NamespacedName{Name: instanceName, Namespace: "default"}
			reconciledInstance := &v1beta1.MongoDBAtlasCluster{}

			expectedStatus := &v1beta1.MongoDBAtlasClusterStatus{
				ObservedGeneration: 1,
				Conditions: []metav1.Condition{
					{
						Type:    v1beta1.ConditionReady,
						Status:  metav1.ConditionFalse,
						Reason:  "ReconciliationFailed",
						Message: "no such atlas cluster found",
					},
					{
						Type:    v1beta1.ConditionScaledToZero,
						Status:  metav1.ConditionFalse,
						Reason:  "PodsRunning",
						Message: "selector matches at least one running pod",
					},
				},
			}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, instanceLookupKey, reconciledInstance)
				if err != nil {
					return false
				}

				return needConditions(expectedStatus.Conditions, reconciledInstance.Status.Conditions)
			}, timeout, interval).Should(BeTrue())
		})
	})

	matchLabel := fmt.Sprintf("app-%s", rand.String(5))

	When("no such atlas cluster found and a selector which matches no pods", func() {
		It("should report Ready=false and ScaledToZero=true", func() {
			By("creating a secret")
			ctx := context.Background()

			var instance v1beta1.MongoDBAtlasCluster
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: "default"}, &instance)).Should(Succeed())

			instance.Spec.ScaleToZero = []metav1.LabelSelector{
				{
					MatchLabels: map[string]string{
						"app": matchLabel,
					},
				},
			}
			Expect(k8sClient.Update(ctx, &instance)).Should(Succeed())

			By("waiting for the reconciliation")
			instanceLookupKey := types.NamespacedName{Name: instanceName, Namespace: "default"}
			reconciledInstance := &v1beta1.MongoDBAtlasCluster{}

			expectedStatus := &v1beta1.MongoDBAtlasClusterStatus{
				ObservedGeneration: 1,
				Conditions: []metav1.Condition{
					{
						Type:    v1beta1.ConditionReady,
						Status:  metav1.ConditionFalse,
						Reason:  "ReconciliationFailed",
						Message: "no such atlas cluster found",
					},
					{
						Type:    v1beta1.ConditionScaledToZero,
						Status:  metav1.ConditionTrue,
						Reason:  "PodsNotRunning",
						Message: "no running pods detected",
					},
				},
			}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, instanceLookupKey, reconciledInstance)
				if err != nil {
					return false
				}

				return needConditions(expectedStatus.Conditions, reconciledInstance.Status.Conditions)
			}, timeout, interval).Should(BeTrue())
		})
	})

	When("no such atlas cluster found and a pod is running which matches a scaleToZero selector", func() {
		It("should report Ready=false and ScaledToZero=true", func() {
			By("creating a secret")
			ctx := context.Background()

			pod := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("pod-%s", rand.String(5)),
					Labels:    map[string]string{"app": matchLabel},
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "podname",
							Image: "image",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, &pod)).Should(Succeed())

			By("waiting for the reconciliation")
			instanceLookupKey := types.NamespacedName{Name: instanceName, Namespace: "default"}
			reconciledInstance := &v1beta1.MongoDBAtlasCluster{}

			expectedStatus := &v1beta1.MongoDBAtlasClusterStatus{
				ObservedGeneration: 1,
				Conditions: []metav1.Condition{
					{
						Type:    v1beta1.ConditionReady,
						Status:  metav1.ConditionFalse,
						Reason:  "ReconciliationFailed",
						Message: "no such atlas cluster found",
					},
					{
						Type:    v1beta1.ConditionScaledToZero,
						Status:  metav1.ConditionFalse,
						Reason:  "PodsRunning",
						Message: "selector matches at least one running pod",
					},
				},
			}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, instanceLookupKey, reconciledInstance)
				if err != nil {
					return false
				}

				return needConditions(expectedStatus.Conditions, reconciledInstance.Status.Conditions)
			}, timeout, interval).Should(BeTrue())
		})
	})
})
