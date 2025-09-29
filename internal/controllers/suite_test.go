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
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/doodlescheduling/cloud-autoscale-controller/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

var (
	cfg        *rest.Config
	k8sClient  client.Client // You'll be using this client in your tests.
	testEnv    *envtest.Environment
	ctx        context.Context
	cancel     context.CancelFunc
	httpClient = &http.Client{
		Transport: &mockTransport{},
	}
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "base", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = v1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&MongoDBAtlasClusterReconciler{
		HTTPClient: httpClient,
		Client:     k8sManager.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("MongoDBAtlasCluster"),
		Recorder:   k8sManager.GetEventRecorderFor("MongoDBAtlasCluster"),
	}).SetupWithManager(k8sManager, MongoDBAtlasClusterReconcilerOptions{})
	Expect(err).ToNot(HaveOccurred())

	err = (&AWSRDSInstanceReconciler{
		HTTPClient: httpClient,
		Log:        ctrl.Log.WithName("controllers").WithName("MongoDBAtlasCluster"),
		Client:     k8sManager.GetClient(),
		Recorder:   k8sManager.GetEventRecorderFor("AWSRDSInstance"),
	}).SetupWithManager(k8sManager, AWSRDSInstanceReconcilerOptions{})
	Expect(err).ToNot(HaveOccurred())

	err = (&Neo4jAuraInstanceReconciler{
		HTTPClient: neo4jMockHTTPClient,
		Log:        ctrl.Log.WithName("controllers").WithName("Neo4jAuraInstance"),
		Client:     k8sManager.GetClient(),
		Recorder:   k8sManager.GetEventRecorderFor("Neo4jAuraInstance"),
	}).SetupWithManager(k8sManager, Neo4jAuraInstanceReconcilerOptions{})
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

type mockTransport struct {
}

func (m *mockTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Host == "token-endpoint" {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"access_token": "token", "expires_in": 3600}`)),
		}, nil
	}

	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(``)),
	}, nil
}

func needConditions(expected []metav1.Condition, current []metav1.Condition) bool {
	for _, expectedCondition := range expected {
		var hasCondition bool
		for _, condition := range current {
			if expectedCondition.Type == condition.Type {
				hasCondition = true

				if expectedCondition.Status != condition.Status {
					return false
				}
				if expectedCondition.Reason != condition.Reason {
					return false
				}
				if expectedCondition.Message != condition.Message {
					return false
				}
			}
		}

		if !hasCondition {
			return false
		}
	}

	return true
}
