package statefulset

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pusher/wave/pkg/core"
	"github.com/pusher/wave/test/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("StatefulSet controller Suite", func() {
	var c client.Client
	var m utils.Matcher

	var sts *appsv1.StatefulSet
	var requests <-chan reconcile.Request
	var mgrStopped *sync.WaitGroup
	var stopMgr chan struct{}

	const timeout = time.Second * 5
	const consistentlyTimeout = time.Second

	var ownerRef metav1.OwnerReference
	var cm1 *corev1.ConfigMap
	var cm2 *corev1.ConfigMap
	var cm3 *corev1.ConfigMap
	var s1 *corev1.Secret
	var s2 *corev1.Secret
	var s3 *corev1.Secret

	var waitForStatefulSetReconciled = func(obj core.Object) {
		request := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      obj.GetName(),
				Namespace: obj.GetNamespace(),
			},
		}
		// wait for reconcile for creating the StatefulSet
		Eventually(requests, timeout).Should(Receive(Equal(request)))
	}

	BeforeEach(func() {
		mgr, err := manager.New(cfg, manager.Options{})
		Expect(err).NotTo(HaveOccurred())
		c = mgr.GetClient()
		m = utils.Matcher{Client: c}

		var recFn reconcile.Reconciler
		recFn, requests = SetupTestReconcile(newReconciler(mgr))
		Expect(add(mgr, recFn)).NotTo(HaveOccurred())

		stopMgr, mgrStopped = StartTestManager(mgr)

		// Create some configmaps and secrets
		cm1 = utils.ExampleConfigMap1.DeepCopy()
		cm2 = utils.ExampleConfigMap2.DeepCopy()
		cm3 = utils.ExampleConfigMap3.DeepCopy()
		s1 = utils.ExampleSecret1.DeepCopy()
		s2 = utils.ExampleSecret2.DeepCopy()
		s3 = utils.ExampleSecret3.DeepCopy()

		m.Create(cm1).Should(Succeed())
		m.Create(cm2).Should(Succeed())
		m.Create(cm3).Should(Succeed())
		m.Create(s1).Should(Succeed())
		m.Create(s2).Should(Succeed())
		m.Create(s3).Should(Succeed())
		m.Get(cm1, timeout).Should(Succeed())
		m.Get(cm2, timeout).Should(Succeed())
		m.Get(cm3, timeout).Should(Succeed())
		m.Get(s1, timeout).Should(Succeed())
		m.Get(s2, timeout).Should(Succeed())
		m.Get(s3, timeout).Should(Succeed())

		sts = utils.ExampleStatefulSet.DeepCopy()

		// Create a statefulset and wait for it to be reconciled
		m.Create(sts).Should(Succeed())
		waitForStatefulSetReconciled(sts)

		ownerRef = utils.GetOwnerRefStatefulSet(sts)
	})

	AfterEach(func() {
		// Make sure to delete any finalizers (if the deployment exists)
		Eventually(func() error {
			key := types.NamespacedName{Namespace: sts.GetNamespace(), Name: sts.GetName()}
			err := c.Get(context.TODO(), key, sts)
			if err != nil && errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			sts.SetFinalizers([]string{})
			return c.Update(context.TODO(), sts)
		}, timeout).Should(Succeed())

		Eventually(func() error {
			key := types.NamespacedName{Namespace: sts.GetNamespace(), Name: sts.GetName()}
			err := c.Get(context.TODO(), key, sts)
			if err != nil && errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if len(sts.GetFinalizers()) > 0 {
				return fmt.Errorf("Finalizers not upated")
			}
			return nil
		}, timeout).Should(Succeed())

		close(stopMgr)
		mgrStopped.Wait()

		utils.DeleteAll(cfg, timeout,
			&appsv1.DeploymentList{},
			&corev1.ConfigMapList{},
			&corev1.SecretList{},
			&corev1.EventList{},
		)
	})

	Context("When a Deployment is reconciled", func() {
		Context("And it has the required annotation", func() {
			BeforeEach(func() {
				annotations := sts.GetAnnotations()
				if annotations == nil {
					annotations = make(map[string]string)
				}
				annotations[core.RequiredAnnotation] = "true"
				sts.SetAnnotations(annotations)

				m.Update(sts).Should(Succeed())
				waitForStatefulSetReconciled(sts)

				// Get the updated Deployment
				m.Get(sts, timeout).Should(Succeed())
			})

			It("Adds OwnerReferences to all children", func() {
				for _, obj := range []core.Object{cm1, cm2, cm3, s1, s2, s3} {
					m.Eventually(obj, timeout).Should(utils.WithOwnerReferences(ContainElement(ownerRef)))
				}
			})

			It("Adds a finalizer to the Deployment", func() {
				m.Eventually(sts, timeout).Should(utils.WithFinalizers(ContainElement(core.FinalizerString)))
			})

			It("Adds a config hash to the Pod Template", func() {
				m.Eventually(sts, timeout).Should(utils.WithPodTemplateAnnotations(HaveKey(core.ConfigHashAnnotation)))
			})

			It("Sends an event when updating the hash", func() {
				m.Eventually(sts, timeout).Should(utils.WithPodTemplateAnnotations(HaveKey(core.ConfigHashAnnotation)))

				events := &corev1.EventList{}
				eventMessage := func(event *corev1.Event) string {
					return event.Message
				}

				hashMessage := "Configuration hash updated to ebabf80ef45218b27078a41ca16b35a4f91cb5672f389e520ae9da6ee3df3b1c"
				m.Eventually(events, timeout).Should(utils.WithItems(ContainElement(WithTransform(eventMessage, Equal(hashMessage)))))
			})

			Context("And a child is removed", func() {
				var originalHash string
				BeforeEach(func() {
					m.Eventually(sts, timeout).Should(utils.WithPodTemplateAnnotations(HaveKey(core.ConfigHashAnnotation)))
					originalHash = sts.Spec.Template.GetAnnotations()[core.ConfigHashAnnotation]

					// Remove "container2" which references Secret example2 and ConfigMap
					// example2
					containers := sts.Spec.Template.Spec.Containers
					Expect(containers[0].Name).To(Equal("container1"))
					sts.Spec.Template.Spec.Containers = []corev1.Container{containers[0]}
					m.Update(sts).Should(Succeed())
					waitForStatefulSetReconciled(sts)

					// Get the updated Deployment
					m.Get(sts, timeout).Should(Succeed())
				})

				It("Removes the OwnerReference from the orphaned ConfigMap", func() {
					m.Eventually(cm2, timeout).ShouldNot(utils.WithOwnerReferences(ContainElement(ownerRef)))
				})

				It("Removes the OwnerReference from the orphaned Secret", func() {
					m.Eventually(s2, timeout).ShouldNot(utils.WithOwnerReferences(ContainElement(ownerRef)))
				})

				It("Updates the config hash in the Pod Template", func() {
					m.Eventually(sts, timeout).ShouldNot(utils.WithPodTemplateAnnotations(HaveKeyWithValue(core.ConfigHashAnnotation, originalHash)))
				})
			})

			Context("And a child is updated", func() {
				var originalHash string

				BeforeEach(func() {
					m.Eventually(sts, timeout).Should(utils.WithPodTemplateAnnotations(HaveKey(core.ConfigHashAnnotation)))
					originalHash = sts.Spec.Template.GetAnnotations()[core.ConfigHashAnnotation]
				})

				Context("A ConfigMap volume is updated", func() {
					BeforeEach(func() {
						m.Get(cm1, timeout).Should(Succeed())
						cm1.Data["key1"] = "modified"
						m.Update(cm1).Should(Succeed())

						waitForStatefulSetReconciled(sts)

						// Get the updated Deployment
						m.Get(sts, timeout).Should(Succeed())
					})

					It("Updates the config hash in the Pod Template", func() {
						m.Eventually(sts, timeout).ShouldNot(utils.WithAnnotations(HaveKeyWithValue(core.ConfigHashAnnotation, originalHash)))
					})
				})

				Context("A ConfigMap EnvSource is updated", func() {
					BeforeEach(func() {
						m.Get(cm2, timeout).Should(Succeed())
						cm2.Data["key1"] = "modified"
						m.Update(cm2).Should(Succeed())

						waitForStatefulSetReconciled(sts)

						// Get the updated Deployment
						m.Get(sts, timeout).Should(Succeed())
					})

					It("Updates the config hash in the Pod Template", func() {
						m.Eventually(sts, timeout).ShouldNot(utils.WithAnnotations(HaveKeyWithValue(core.ConfigHashAnnotation, originalHash)))
					})
				})

				Context("A Secret volume is updated", func() {
					BeforeEach(func() {
						m.Get(s1, timeout).Should(Succeed())
						if s1.StringData == nil {
							s1.StringData = make(map[string]string)
						}
						s1.StringData["key1"] = "modified"
						m.Update(s1).Should(Succeed())

						waitForStatefulSetReconciled(sts)

						// Get the updated Deployment
						m.Get(sts, timeout).Should(Succeed())
					})

					It("Updates the config hash in the Pod Template", func() {
						m.Eventually(sts, timeout).ShouldNot(utils.WithAnnotations(HaveKeyWithValue(core.ConfigHashAnnotation, originalHash)))
					})
				})

				Context("A Secret EnvSource is updated", func() {
					BeforeEach(func() {
						m.Get(s2, timeout).Should(Succeed())
						if s2.StringData == nil {
							s2.StringData = make(map[string]string)
						}
						s2.StringData["key1"] = "modified"
						m.Update(s2).Should(Succeed())

						waitForStatefulSetReconciled(sts)

						// Get the updated Deployment
						m.Get(sts, timeout).Should(Succeed())
					})

					It("Updates the config hash in the Pod Template", func() {
						m.Eventually(sts, timeout).ShouldNot(utils.WithAnnotations(HaveKeyWithValue(core.ConfigHashAnnotation, originalHash)))
					})
				})
			})

			Context("And the annotation is removed", func() {
				BeforeEach(func() {
					m.Get(sts, timeout).Should(Succeed())
					sts.SetAnnotations(make(map[string]string))
					m.Update(sts).Should(Succeed())
					waitForStatefulSetReconciled(sts)

					m.Eventually(sts, timeout).ShouldNot(utils.WithAnnotations(HaveKey(core.RequiredAnnotation)))
				})

				It("Removes the OwnerReference from the all children", func() {
					for _, obj := range []core.Object{cm1, cm2, s1, s2} {
						m.Eventually(obj, timeout).ShouldNot(utils.WithOwnerReferences(ContainElement(ownerRef)))
					}
				})

				It("Removes the Deployment's finalizer", func() {
					m.Eventually(sts, timeout).ShouldNot(utils.WithFinalizers(ContainElement(core.FinalizerString)))
				})
			})

			Context("And is deleted", func() {
				BeforeEach(func() {
					// Make sure the cache has synced before we run the test
					m.Eventually(sts, timeout).Should(utils.WithPodTemplateAnnotations(HaveKey(core.ConfigHashAnnotation)))
					m.Delete(sts).Should(Succeed())
					m.Eventually(sts, timeout).ShouldNot(utils.WithDeletionTimestamp(BeNil()))
					waitForStatefulSetReconciled(sts)

					// Get the updated Deployment
					m.Get(sts, timeout).Should(Succeed())
				})
				It("Removes the OwnerReference from the all children", func() {
					for _, obj := range []core.Object{cm1, cm2, s1, s2} {
						m.Eventually(obj, timeout).ShouldNot(utils.WithOwnerReferences(ContainElement(ownerRef)))
					}
				})

				It("Removes the Deployment's finalizer", func() {
					// Removing the finalizer causes the deployment to be deleted
					m.Get(sts, timeout).ShouldNot(Succeed())
				})
			})
		})

		Context("And it does not have the required annotation", func() {
			BeforeEach(func() {
				// Get the updated Deployment
				m.Get(sts, timeout).Should(Succeed())
			})

			It("Doesn't add any OwnerReferences to any children", func() {
				for _, obj := range []core.Object{cm1, cm2, s1, s2} {
					m.Consistently(obj, consistentlyTimeout).ShouldNot(utils.WithOwnerReferences(ContainElement(ownerRef)))
				}
			})

			It("Doesn't add a finalizer to the Deployment", func() {
				m.Consistently(sts, consistentlyTimeout).ShouldNot(utils.WithFinalizers(ContainElement(core.FinalizerString)))
			})

			It("Doesn't add a config hash to the Pod Template", func() {
				m.Consistently(sts, consistentlyTimeout).ShouldNot(utils.WithAnnotations(ContainElement(core.ConfigHashAnnotation)))
			})
		})
	})

})
