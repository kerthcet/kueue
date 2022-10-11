package e2e

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kueue "sigs.k8s.io/kueue/apis/kueue/v1alpha2"
	"sigs.k8s.io/kueue/test/e2e/framework"

	"sigs.k8s.io/kueue/pkg/util/testing"
)

// +kubebuilder:docs-gen:collapse=Imports

var _ = ginkgo.Describe("End To End Suite", func() {
	var ns *corev1.Namespace

	ginkgo.BeforeEach(func() {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "e2e-",
			},
		}
		gomega.Expect(k8sClient.Create(ctx, ns)).To(gomega.Succeed())
	})

	ginkgo.AfterEach(func() {
		gomega.Expect(k8sClient.Delete(ctx, ns)).To(gomega.Succeed())
	})

	ginkgo.When("Creating a Job Without Queueing Logic", func() {

		ginkgo.It("Should stay in suspended", func() {
			sampleJob := testing.MakeJob("test-job", ns.Name)
			annotations := map[string]string{
				"kueue.x-k8s.io/queue-name": "main",
			}
			sampleJob.ObjectMeta.Annotations = annotations
			gomega.Expect(k8sClient.Create(context.Background(), &sampleJob.Job)).Should(gomega.Succeed())
			lookupKey := types.NamespacedName{Name: "test-job", Namespace: ns.Name}
			createdJob := &batchv1.Job{}
			gomega.Eventually(func() bool {
				if err := k8sClient.Get(context.Background(), lookupKey, createdJob); err != nil {
					return false
				}
				return *createdJob.Spec.Suspend
			}, framework.Timeout, framework.Interval).Should(gomega.BeTrue())
			gomega.Expect(k8sClient.Delete(context.Background(), &sampleJob.Job)).Should(gomega.Succeed())

		})
	})
	ginkgo.When("Creating Single-ClusterQueue-Setup From Samples", func() {
		var (
			ns            *corev1.Namespace
			resourceKueue *kueue.ResourceFlavor
			localQueue    *kueue.LocalQueue
			clusterQueue  *kueue.ClusterQueue
		)
		ginkgo.BeforeEach(func() {
			ns = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "e2e-",
				},
			}
			gomega.Expect(k8sClient.Create(ctx, ns)).To(gomega.Succeed())
		})

		ginkgo.BeforeEach(func() {
			resourceKueue = testing.MakeResourceFlavor("default").Obj()
			gomega.Expect(k8sClient.Create(context.Background(), resourceKueue)).Should(gomega.Succeed())
			localQueue = testing.MakeLocalQueue("main", ns.Name).Obj()
			clusterQueue = &kueue.ClusterQueue{ObjectMeta: metav1.ObjectMeta{
				Name: "cluster-queue",
			},
				Spec: kueue.ClusterQueueSpec{
					NamespaceSelector: &metav1.LabelSelector{},
					Resources: []kueue.Resource{
						{
							Name: corev1.ResourceCPU,
							Flavors: []kueue.Flavor{{
								Name: "default",
								Quota: kueue.Quota{
									Min: resource.MustParse("1"),
								},
							}},
						},
						{
							Name: corev1.ResourceMemory,
							Flavors: []kueue.Flavor{{
								Name: "default",
								Quota: kueue.Quota{
									Min: resource.MustParse("36Gi"),
								},
							}},
						},
					}},
			}

			localQueue.Spec.ClusterQueue = "cluster-queue"
			gomega.Expect(k8sClient.Create(context.Background(), clusterQueue)).Should(gomega.Succeed())

			gomega.Expect(k8sClient.Create(context.Background(), localQueue)).Should(gomega.Succeed())
		})
		ginkgo.AfterEach(func() {
			gomega.Expect(k8sClient.Delete(context.Background(), localQueue)).Should(gomega.Succeed())
			gomega.Expect(k8sClient.Delete(context.Background(), clusterQueue)).Should(gomega.Succeed())
			gomega.Expect(k8sClient.Delete(context.Background(), resourceKueue)).Should(gomega.Succeed())
		})

		ginkgo.AfterEach(func() {
			gomega.Expect(k8sClient.Delete(context.Background(), ns)).Should(gomega.Succeed())
		})
		ginkgo.It("Should create all objects in that file", func() {

			ginkgo.By("Creating a Job")
			sampleJob := testing.MakeJob("test-job", ns.Name).Request("cpu", "1").Request("memory", "20Mi")

			annotations := map[string]string{
				"kueue.x-k8s.io/queue-name": "main",
			}
			sampleJob.ObjectMeta.Annotations = annotations
			gomega.Expect(k8sClient.Create(context.Background(), &sampleJob.Job)).Should(gomega.Succeed())
			lookupKey := types.NamespacedName{Name: "test-job", Namespace: ns.Name}
			createdJob := &batchv1.Job{}
			gomega.Eventually(func() bool {
				if err := k8sClient.Get(context.Background(), lookupKey, createdJob); err != nil {
					return false
				}
				return !*createdJob.Spec.Suspend
			}, framework.Timeout, framework.Interval).Should(gomega.BeTrue())

		})
	})
})
