package controllers

import (
	"math/rand"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("ZDB Controller", func() {

	replicas := 9
	maxUnavailable := 1
	zones := []string{"us-east-1a", "us-east-1b", "us-east-1c"}

	Context("When all pods are active", func() {
		It("Should allow disruptions to any zone", func() {
			label := "test1"
			ss := testUtils.CreateStatefulSet(int32(replicas), label)
			zdb := testUtils.CreateZdb(intstr.FromInt(maxUnavailable), false, label)

			for i := 0; i < replicas; {
				for _, zone := range zones {
					testUtils.CreateStatefulSetPod(podName(label, i), zone, v1.PodRunning, label, ss)
					i++
				}
			}

			zdb = testUtils.GetZdb(zdb.Name)

			zoneCount := len(zones)
			for _, zone := range zones {
				Expect(zdb.Status.ExpectedPods[zone]).Should(Equal(int32(replicas / zoneCount)))
				Expect(zdb.Status.CurrentHealthy[zone]).Should(Equal(int32(replicas / zoneCount)))
				Expect(zdb.Status.CurrentUnhealthy[zone]).Should(Equal(int32(0)))
				Expect(zdb.Status.DesiredHealthy[zone]).Should(Equal(int32(replicas/zoneCount - maxUnavailable)))
				Expect(zdb.Status.DisruptionsAllowed[zone]).Should(Equal(int32(maxUnavailable)))
				Expect(len(zdb.Status.DisruptedPods)).Should(Equal(0))
				Expect(zdb.Status.ObservedGeneration).Should(Equal(int64(1)))
			}
		})
	})

	Context("When there is a single non active pod", func() {
		It("Should block disruptions to other zones", func() {
			label := "test2"
			ss := testUtils.CreateStatefulSet(int32(replicas), label)
			zdb := testUtils.CreateZdb(intstr.FromInt(maxUnavailable), false, label)

			for i := 0; i < replicas; {
				for _, zone := range zones {
					testUtils.CreateStatefulSetPod(podName(label, i), zone, v1.PodRunning, label, ss)
					i++
				}
			}

			nonActivePod := testUtils.GetPod(podName(label, rand.Intn(replicas)))
			testUtils.UpdatePodStatus(nonActivePod, v1.PodPending, v1.PodReady)
			nonActivePodZone := testUtils.GetPodZone(nonActivePod)

			zdb = testUtils.GetZdb(zdb.Name)

			zoneCount := len(zones)
			for _, zone := range zones {
				Expect(zdb.Status.ExpectedPods[zone]).Should(Equal(int32(replicas / zoneCount)))
				Expect(zdb.Status.DesiredHealthy[zone]).Should(Equal(int32(replicas/zoneCount - maxUnavailable)))
				if zone != nonActivePodZone {
					Expect(zdb.Status.CurrentHealthy[zone]).Should(Equal(int32(replicas / zoneCount)))
					Expect(zdb.Status.CurrentUnhealthy[zone]).Should(Equal(int32(0)))
					Expect(zdb.Status.DisruptionsAllowed[zone]).Should(Equal(int32(0)))
				} else {
					Expect(zdb.Status.CurrentHealthy[zone]).Should(Equal(int32(replicas/zoneCount - 1)))
					Expect(zdb.Status.CurrentUnhealthy[zone]).Should(Equal(int32(1)))
					Expect(zdb.Status.DisruptionsAllowed[zone]).Should(Equal(int32(maxUnavailable - 1)))
				}

				Expect(len(zdb.Status.DisruptedPods)).Should(Equal(0))
				Expect(zdb.Status.ObservedGeneration).Should(Equal(int64(1)))
			}
		})
	})

	Context("When there are non active pods in two zones", func() {
		It("Should block disruptions to all zones", func() {
			label := "test3"
			ss := testUtils.CreateStatefulSet(int32(replicas), label)
			zdb := testUtils.CreateZdb(intstr.FromInt(maxUnavailable), false, label)

			numberOfNonActivePods := 2
			for i := 0; i < replicas; {
				for _, zone := range zones {
					var podPhase v1.PodPhase
					if numberOfNonActivePods > 0 {
						podPhase = v1.PodPending
						numberOfNonActivePods--
					} else {
						podPhase = v1.PodRunning
					}
					testUtils.CreateStatefulSetPod(podName(label, i), zone, podPhase, label, ss)
					i++
				}
			}

			zdb = testUtils.GetZdb(zdb.Name)

			zoneCount := len(zones)
			lastZone := zones[zoneCount-1]
			for _, zone := range zones {
				Expect(zdb.Status.ExpectedPods[zone]).Should(Equal(int32(replicas / zoneCount)))
				Expect(zdb.Status.DesiredHealthy[zone]).Should(Equal(int32(replicas/zoneCount - maxUnavailable)))
				if zone == lastZone {
					Expect(zdb.Status.CurrentHealthy[zone]).Should(Equal(int32(replicas / zoneCount)))
					Expect(zdb.Status.CurrentUnhealthy[zone]).Should(Equal(int32(0)))
				} else {
					Expect(zdb.Status.CurrentHealthy[zone]).Should(Equal(int32(replicas/zoneCount - 1)))
					Expect(zdb.Status.CurrentUnhealthy[zone]).Should(Equal(int32(1)))
				}
				Expect(zdb.Status.DisruptionsAllowed[zone]).Should(Equal(int32(0)))
				Expect(len(zdb.Status.DisruptedPods)).Should(Equal(0))
				Expect(zdb.Status.ObservedGeneration).Should(Equal(int64(1)))
			}
		})
	})

	Context("When there is a pod getting disrupted/evicted", func() {
		It("Should block disruptions to other zones", func() {
			label := "test4"
			ss := testUtils.CreateStatefulSet(int32(replicas), label)
			zdb := testUtils.CreateZdb(intstr.FromInt(maxUnavailable), false, label)

			for i := 0; i < replicas; {
				for _, zone := range zones {
					testUtils.CreateStatefulSetPod(podName(label, i), zone, v1.PodRunning, label, ss)
					i++
				}
			}

			disruptedPod := testUtils.GetPod(podName(label, rand.Intn(replicas)))
			disruptedPodZone := testUtils.GetPodZone(disruptedPod)

			zdb = testUtils.GetZdb(zdb.Name)
			zdb.Status.DisruptedPods = map[string]metav1.Time{}
			zdb.Status.DisruptedPods[disruptedPod.Name] = metav1.Time{Time: time.Now()}
			Expect(k8sClient.Status().Update(ctx, zdb)).Should(Succeed())

			zdb = testUtils.GetZdb(zdb.Name)
			zoneCount := len(zones)
			for _, zone := range zones {
				Expect(zdb.Status.ExpectedPods[zone]).Should(Equal(int32(replicas / zoneCount)))
				Expect(zdb.Status.DesiredHealthy[zone]).Should(Equal(int32(replicas/zoneCount - maxUnavailable)))
				if zone != disruptedPodZone {
					Expect(zdb.Status.CurrentHealthy[zone]).Should(Equal(int32(replicas / zoneCount)))
					Expect(zdb.Status.CurrentUnhealthy[zone]).Should(Equal(int32(0)))
					Expect(zdb.Status.DisruptionsAllowed[zone]).Should(Equal(int32(0)))
				} else {
					Expect(zdb.Status.CurrentHealthy[zone]).Should(Equal(int32(replicas/zoneCount - 1)))
					Expect(zdb.Status.CurrentUnhealthy[zone]).Should(Equal(int32(1)))
					Expect(zdb.Status.DisruptionsAllowed[zone]).Should(Equal(int32(maxUnavailable - 1)))
				}

				Expect(len(zdb.Status.DisruptedPods)).Should(Equal(1))
				Expect(zdb.Status.ObservedGeneration).Should(Equal(int64(1)))
			}
		})
	})
})

func podName(label string, num int) string {
	return label + "-" + strconv.Itoa(num)
}
