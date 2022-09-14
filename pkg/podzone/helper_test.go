package podzone

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("PodZoneHelper", func() {

	Describe("PodZoneCache", func() {
		var helper *Helper

		BeforeEach(func() {
			helper = &Helper{
				Client: k8sClient,
				Logger: ctrl.Log.WithName("pod-zone-helper"),
				Cache:  NewCache(),
			}
		})

		Context("When there is a pod with node information", func() {
			It("Should update cache", func() {
				podName := "test-cache1"
				zone := "ca-central-1a"
				node := testUtils.GetOrCreateNode(podName, zone)
				pod := testUtils.CreateSimplePod(podName, node)

				zone, found := helper.getPodZone(context.TODO(), pod)
				Expect(found).Should(BeTrue())
				Expect(zone).Should(Equal(zone))

				value, ok, err := helper.Cache.GetByKey(podName)
				Expect(err).Should(BeNil())
				Expect(ok).Should(BeTrue())
				Expect(value.(PodZone).Zone).Should(Equal(zone))
			})
		})

		Context("When there is a pod without node information", func() {
			It("Should get zone from cache", func() {
				podName := "test-cache2"
				zone := "ca-central-1a"
				pod := testUtils.CreateSimplePod(podName, &v1.Node{})

				By("Pre-populate cache")
				podZone := PodZone{
					PodName: pod.Name,
					Zone:    zone,
				}
				err := helper.Cache.Add(podZone)
				Expect(err).Should(BeNil())

				zone, found := helper.getPodZone(context.TODO(), pod)
				Expect(found).Should(BeTrue())
				Expect(zone).Should(Equal(zone))
			})

			It("Should not return zone if not in the cache", func() {
				podName := "test-cache3"
				zone := "ca-central-1a"
				pod := testUtils.CreateSimplePod(podName, &v1.Node{})

				zone, found := helper.getPodZone(context.TODO(), pod)
				Expect(found).Should(BeFalse())
				Expect(zone).Should(Equal(""))
			})
		})
	})
})
