package webhook

import (
	opsv1 "github.com/aws/zone-aware-controllers-for-k8s/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Eviction Webhook", func() {
	Context("When the zdb status allows disruptions to the zone", func() {
		It("Should allow evictions", func() {
			label := "test1"
			disruptionsAllowed := map[string]int32{"az-1": 1}
			zdb := createZdb(label, disruptionsAllowed, false)

			pod := testUtils.CreatePod("test-allow", "az-1", label, v1.PodRunning, v1.PodReady)
			Expect(evict(pod, false)).Should(Succeed())

			zdb = testUtils.GetZdb(zdb.Name)
			Expect(len(zdb.Status.DisruptedPods)).Should(Equal(1))
		})
	})

	Context("When the zdb status does not allow disruptions to the zone", func() {
		It("Should deny evictions", func() {
			label := "test2"
			disruptionsAllowed := map[string]int32{"az-1": 0}
			zdb := createZdb(label, disruptionsAllowed, false)

			pod := testUtils.CreatePod("test-deny", "az-1", label, v1.PodRunning, v1.PodReady)
			Expect(evict(pod, false)).Should(MatchError(ContainSubstring("denying pod eviction")))

			zdb = testUtils.GetZdb(zdb.Name)
			Expect(len(zdb.Status.DisruptedPods)).Should(Equal(0))
		})

		It("Should allow evictions if the pod is not running", func() {
			label := "test3"
			disruptionsAllowed := map[string]int32{"az-1": 0}
			zdb := createZdb(label, disruptionsAllowed, false)

			pod := testUtils.CreatePod("test-not-running", "az-1", label, v1.PodPending, v1.PodScheduled)
			Expect(evict(pod, false)).Should(Succeed())

			zdb = testUtils.GetZdb(zdb.Name)
			Expect(len(zdb.Status.DisruptedPods)).Should(Equal(0))
		})

		It("Should allow evictions if the pod is not ready", func() {
			label := "test4"
			disruptionsAllowed := map[string]int32{"az-1": 0}
			zdb := createZdb(label, disruptionsAllowed, false)

			pod := testUtils.CreatePod("test-not-ready", "az-1", label, v1.PodRunning, v1.PodInitialized)
			Expect(evict(pod, false)).Should(Succeed())

			zdb = testUtils.GetZdb(zdb.Name)
			Expect(len(zdb.Status.DisruptedPods)).Should(Equal(0))
		})

		It("Should allow evictions if dryrun option is passed", func() {
			label := "test5"
			disruptionsAllowed := map[string]int32{"az-1": 0}
			zdb := createZdb(label, disruptionsAllowed, false)

			pod := testUtils.CreatePod("test-dryrun-option", "az-1", label, v1.PodRunning, v1.PodReady)
			Expect(evict(pod, true)).Should(Succeed())

			zdb = testUtils.GetZdb(zdb.Name)
			Expect(len(zdb.Status.DisruptedPods)).Should(Equal(0))
		})

		It("Should allow evictions if zdb is in dryrun mode", func() {
			label := "test6"
			disruptionsAllowed := map[string]int32{"az-1": 0}
			zdb := createZdb(label, disruptionsAllowed, true)

			pod := testUtils.CreatePod("test-dryrun-zdb", "az-1", label, v1.PodRunning, v1.PodReady)
			Expect(evict(pod, false)).Should(Succeed())

			zdb = testUtils.GetZdb(zdb.Name)
			Expect(len(zdb.Status.DisruptedPods)).Should(Equal(0))
		})
	})

	Context("When there is no zdb associated to the pod", func() {
		It("Should allow evictions", func() {
			pod := testUtils.CreatePod("test-no-zdb", "az-1", "any", v1.PodRunning, v1.PodReady)
			Expect(evict(pod, false)).Should(Succeed())
		})
	})
})

func createZdb(label string, disruptionsAllowed map[string]int32, dryRun bool) *opsv1.ZoneDisruptionBudget {
	maxUnavailable := intstr.FromInt(1)
	zdb := testUtils.CreateZdb(maxUnavailable, dryRun, label)
	return testUtils.UpdateZdbDisruptions(zdb, disruptionsAllowed)
}

func evict(pod *v1.Pod, dryRun bool) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: &metav1.DeleteOptions{},
	}
	if dryRun {
		eviction.DeleteOptions = &metav1.DeleteOptions{
			DryRun: []string{"All"},
		}
	}
	return kubeClient.PolicyV1beta1().Evictions(pod.Namespace).Evict(ctx, eviction)
}
