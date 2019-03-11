/*
Copyright 2018 Pusher Ltd.

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

package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pusher/wave/test/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var _ = Describe("Wave owner references Suite", func() {
	var c client.Client
	var h *Handler
	var m utils.Matcher
	var deployment *appsv1.Deployment
	var podControllerDeployment podController
	var mgrStopped *sync.WaitGroup
	var stopMgr chan struct{}

	const timeout = time.Second * 5

	var ownerRef metav1.OwnerReference

	BeforeEach(func() {
		mgr, err := manager.New(cfg, manager.Options{})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()
		h = NewHandler(c, mgr.GetRecorder("wave"))
		m = utils.Matcher{Client: c}

		// Create some configmaps and secrets
		m.Create(utils.ExampleConfigMap1.DeepCopy()).Should(Succeed())
		m.Create(utils.ExampleConfigMap2.DeepCopy()).Should(Succeed())
		m.Create(utils.ExampleSecret1.DeepCopy()).Should(Succeed())
		m.Create(utils.ExampleSecret2.DeepCopy()).Should(Succeed())

		deployment = utils.ExampleDeployment.DeepCopy()
		podControllerDeployment = &Deployment{deployment}

		m.Create(deployment).Should(Succeed())

		ownerRef = utils.GetOwnerRefDeployment(deployment)

		stopMgr, mgrStopped = StartTestManager(mgr)
		m.Get(deployment, timeout).Should(Succeed())
	})

	AfterEach(func() {
		close(stopMgr)
		mgrStopped.Wait()

		utils.DeleteAll(cfg, timeout,
			&appsv1.DeploymentList{},
			&corev1.ConfigMapList{},
			&corev1.SecretList{},
		)
	})

	Context("handleDelete", func() {
		var cm1 *corev1.ConfigMap
		var cm2 *corev1.ConfigMap
		var s1 *corev1.Secret
		var s2 *corev1.Secret

		BeforeEach(func() {
			cm1 = utils.ExampleConfigMap1.DeepCopy()
			cm2 = utils.ExampleConfigMap2.DeepCopy()
			s1 = utils.ExampleSecret1.DeepCopy()
			s2 = utils.ExampleSecret2.DeepCopy()

			for _, obj := range []Object{cm1, cm2, s1, s2} {
				obj.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
				m.Update(obj).Should(Succeed())
			}

			f := deployment.GetFinalizers()
			f = append(f, FinalizerString)
			f = append(f, "keep.me.around/finalizer")
			deployment.SetFinalizers(f)
			m.Update(deployment).Should(Succeed())

			_, err := h.handleDelete(podControllerDeployment)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			// Make sure to delete any finalizers (if the deployment exists)
			Eventually(func() error {
				key := types.NamespacedName{Namespace: deployment.GetNamespace(), Name: deployment.GetName()}
				err := c.Get(context.TODO(), key, deployment)
				if err != nil && errors.IsNotFound(err) {
					return nil
				}
				if err != nil {
					return err
				}
				deployment.SetFinalizers([]string{})
				return c.Update(context.TODO(), deployment)
			}, timeout).Should(Succeed())

			Eventually(func() error {
				key := types.NamespacedName{Namespace: deployment.GetNamespace(), Name: deployment.GetName()}
				err := c.Get(context.TODO(), key, deployment)
				if err != nil && errors.IsNotFound(err) {
					return nil
				}
				if err != nil {
					return err
				}
				if len(deployment.GetFinalizers()) > 0 {
					return fmt.Errorf("Finalizers not upated")
				}
				return nil
			}, timeout).Should(Succeed())
		})

		It("removes owner references from all children", func() {
			for _, obj := range []Object{cm1, cm2, s1, s2} {
				m.Eventually(obj, timeout).ShouldNot(utils.WithOwnerReferences(ContainElement(ownerRef)))
			}
		})

		It("removes the finalizer from the deployment", func() {
			m.Eventually(deployment, timeout).ShouldNot(utils.WithFinalizers(ContainElement(FinalizerString)))
		})
	})

	// Waiting for toBeDeleted to be implemented
	Context("toBeDeleted", func() {
		It("returns true if deletion timestamp is non-nil", func() {
			t := metav1.NewTime(time.Now())
			deployment.SetDeletionTimestamp(&t)
			Expect(toBeDeleted(deployment)).To(BeTrue())
		})

		It("returns false if the deleteion timestamp is nil", func() {
			Expect(toBeDeleted(deployment)).To(BeFalse())
		})

	})

})
