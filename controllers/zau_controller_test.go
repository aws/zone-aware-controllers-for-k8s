package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	opsv1 "github.com/aws/zone-aware-controllers-for-k8s/api/v1"
	"github.com/aws/zone-aware-controllers-for-k8s/pkg/podzone"
	"github.com/aws/zone-aware-controllers-for-k8s/pkg/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

type mockAlarmStateProvider struct {
	state types.StateValue
	err   error
}

func (m mockAlarmStateProvider) AlarmState(ctx context.Context, alarmName string) (types.StateValue, error) {
	return m.state, m.err
}

var _ = Describe("ZAU Controller", func() {
	zones := []string{"us-east-1a", "us-east-1b", "us-east-1c"}
	replicas := 9
	maxUnavailable := 2

	var controller *ZoneAwareUpdateReconciler

	BeforeEach(func() {
		podZoneHelper := podzone.Helper{
			Client: k8sClient,
			Logger: ctrl.Log.WithName("pod-zone-helper-test"),
			Cache:  podzone.NewCache(),
		}

		controller = &ZoneAwareUpdateReconciler{
			Client:        k8sClient,
			Logger:        ctrl.Log.WithName("zau-controller-test"),
			PodZoneHelper: &podZoneHelper,
		}
	})

	Describe("updateStatefulSet", func() {
		Context("When statefulset strategy is not OnDelete", func() {
			It("It should not delete pods", func() {
				ss, zau, pods := createResources("zau-test1", replicas, maxUnavailable, zones)

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				// no pods in terminating state
				expectNoDeletions(zau, pods)
			})
		})

		Context("When all pods are ready", func() {
			It("It should delete initially a single pod in the first zone", func() {
				ss, zau, pods := createResources("zau-test20", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				expectLastPodInFirstZoneToBeDeleted(zau, pods)
			})

			It("It should double the number of pods updated after the fist step", func() {
				ss, zau, pods := createResources("zau-test21", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType

				// zone-1: [pod-0, pod-3, pod-6]
				pods[6].Labels[apps.ControllerRevisionHashLabelKey] = ss.Status.UpdateRevision
				testUtils.UpdatePod(pods[6])
				testUtils.UpdateZauStep(zau, 1, ss.Status.UpdateRevision)

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				assertContainDeletions(pods, []int{0, 3}) // first two pods in the first zone

				Expect(zau.Status.UpdateStep).Should(Equal(int32(2)))
				Expect(zau.Status.DeletedReplicas).Should(Equal(int32(2)))
				for i, zone := range zones {
					if i == 0 {
						Expect(zau.Status.OldReplicas[zone]).Should(Equal(int32(2)))
					} else {
						Expect(zau.Status.OldReplicas[zone]).Should(Equal(int32(3)))
					}
				}
			})
		})

		Context("When there is only one pod in the first zone to be updated", func() {
			It("It should only delete the first pod in the first zone", func() {
				ss, zau, pods := createResources("zau-test22", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType

				// zone-1: [pod-0, pod-3, pod-6]
				pods[6].Labels[apps.ControllerRevisionHashLabelKey] = ss.Status.UpdateRevision
				testUtils.UpdatePod(pods[6])
				pods[3].Labels[apps.ControllerRevisionHashLabelKey] = ss.Status.UpdateRevision
				testUtils.UpdatePod(pods[3])
				testUtils.UpdateZauStep(zau, 1, ss.Status.UpdateRevision)

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				assertContainDeletions(pods, []int{0}) // first pod in the first zone

				Expect(zau.Status.UpdateStep).Should(Equal(int32(2)))
				Expect(zau.Status.DeletedReplicas).Should(Equal(int32(1)))
				for i, zone := range zones {
					if i == 0 {
						Expect(zau.Status.OldReplicas[zone]).Should(Equal(int32(1)))
					} else {
						Expect(zau.Status.OldReplicas[zone]).Should(Equal(int32(3)))
					}
				}
			})
		})

		Context("When all pods in the first zone were already updated", func() {
			It("It should delete maxUnavailable pods in the second zone", func() {
				ss, zau, pods := createResources("zau-test23", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType

				// zone-1: [pod-0, pod-3, pod-6]
				pods[6].Labels[apps.ControllerRevisionHashLabelKey] = ss.Status.UpdateRevision
				testUtils.UpdatePod(pods[6])
				pods[3].Labels[apps.ControllerRevisionHashLabelKey] = ss.Status.UpdateRevision
				testUtils.UpdatePod(pods[3])
				pods[0].Labels[apps.ControllerRevisionHashLabelKey] = ss.Status.UpdateRevision
				testUtils.UpdatePod(pods[0])
				testUtils.UpdateZauStep(zau, 2, ss.Status.UpdateRevision)

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				assertContainDeletions(pods, []int{4, 7}) // last 2 pods in the second zone

				Expect(zau.Status.UpdateStep).Should(Equal(int32(3)))
				Expect(zau.Status.DeletedReplicas).Should(Equal(int32(2)))
				for i, zone := range zones {
					if i == 0 {
						Expect(zau.Status.OldReplicas[zone]).Should(Equal(int32(0)))
					} else {
						Expect(zau.Status.OldReplicas[zone]).Should(Equal(int32(3)))
					}
				}
			})
		})

		Context("When there is a pod getting terminated", func() {
			It("It should not proceed to delete other pods", func() {
				ss, zau, pods := createResources("zau-test3", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType

				// delete last pod
				deletedPod := testUtils.DeletePod(pods[len(pods)-1])

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				for i := range pods {
					pod := testUtils.GetPod(pods[i].Name)
					if pod.Name == deletedPod.Name {
						Expect(utils.IsTerminating(pod)).Should(BeTrue())
						Expect(pod.DeletionTimestamp).Should(Equal(deletedPod.DeletionTimestamp))
					} else {
						Expect(utils.IsTerminating(pod)).Should(BeFalse())
					}
				}
				Expect(zau.Status.UpdateStep).Should(Equal(int32(0)))
				Expect(zau.Status.DeletedReplicas).Should(Equal(int32(0)))
			})
		})

		Context("When all pods are in the new revision already", func() {
			It("It should not proceed to delete pods", func() {
				ss, zau, pods := createResources("zau-test4", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType

				for i := range pods {
					pod := pods[i]
					pod.Labels[apps.ControllerRevisionHashLabelKey] = ss.Status.UpdateRevision
					testUtils.UpdatePod(pod)
				}

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				expectNoDeletions(zau, pods)
				Expect(len(zau.Status.OldReplicas)).Should(Equal(0))
			})
		})

		Context("When there is non ready pods in the new revision", func() {
			It("It should not proceed to delete other pods", func() {
				ss, zau, pods := createResources("zau-test5", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType

				nonReadyPod := testUtils.UpdatePodStatus(pods[4], v1.PodPending, v1.ContainersReady)
				nonReadyPod.Labels[apps.ControllerRevisionHashLabelKey] = ss.Status.UpdateRevision
				nonReadyPod = testUtils.UpdatePod(nonReadyPod)

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				for i := range pods {
					pod := testUtils.GetPod(pods[i].Name)
					if pod.Name == nonReadyPod.Name {
						Expect(pod.Status.Phase).Should(Equal(v1.PodPending))
					}
					Expect(utils.IsTerminating(pod)).Should(BeFalse())
				}
				Expect(zau.Status.UpdateStep).Should(Equal(int32(0)))
				Expect(zau.Status.DeletedReplicas).Should(Equal(int32(0)))
			})
		})

		Context("When there is non ready pods in the old revision in multiple zones", func() {
			It("It should not proceed to delete other pods", func() {
				ss, zau, pods := createResources("zau-test6", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType

				firstNonReadyPod := testUtils.UpdatePodStatus(pods[4], v1.PodPending, v1.ContainersReady)
				secondNonReadyPod := testUtils.UpdatePodStatus(pods[2], v1.PodPending, v1.ContainersReady)

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				for i := range pods {
					pod := testUtils.GetPod(pods[i].Name)
					if pod.Name == firstNonReadyPod.Name || pod.Name == secondNonReadyPod.Name {
						Expect(pod.Status.Phase).Should(Equal(v1.PodPending))
					}
					Expect(utils.IsTerminating(pod)).Should(BeFalse())
				}
				Expect(zau.Status.UpdateStep).Should(Equal(int32(0)))
				Expect(zau.Status.DeletedReplicas).Should(Equal(int32(0)))
			})
		})

		Context("When there is non ready pods in the second zone", func() {
			It("It should not proceed to delete other pods", func() {
				ss, zau, pods := createResources("zau-test7", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType

				firstNonReadyPod := testUtils.UpdatePodStatus(pods[4], v1.PodPending, v1.ContainersReady)
				secondNonReadyPod := testUtils.UpdatePodStatus(pods[7], v1.PodPending, v1.ContainersReady)

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				for i := range pods {
					pod := testUtils.GetPod(pods[i].Name)
					if pod.Name == firstNonReadyPod.Name || pod.Name == secondNonReadyPod.Name {
						Expect(pod.Status.Phase).Should(Equal(v1.PodPending))
					}
					Expect(utils.IsTerminating(pod)).Should(BeFalse())
				}
				Expect(zau.Status.UpdateStep).Should(Equal(int32(0)))
				Expect(zau.Status.DeletedReplicas).Should(Equal(int32(0)))
			})
		})

		Context("When there is non ready pods in the first zone", func() {
			It("It should delete the non ready pod", func() {
				ss, zau, pods := createResources("zau-test8", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType

				nonReadyPod := 0
				testUtils.UpdatePodStatus(pods[nonReadyPod], v1.PodPending, v1.ContainersReady)

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				assertContainDeletions(pods, []int{nonReadyPod})

				Expect(zau.Status.UpdateStep).Should(Equal(int32(1)))
				Expect(zau.Status.DeletedReplicas).Should(Equal(int32(1)))
			})
		})

		Context("When dryRun is enabled", func() {
			It("It should update zau status but not delete pods", func() {
				ss, zau, pods := createResources("zau-test9", replicas, maxUnavailable, zones)
				zau.Spec.DryRun = true

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				assertHaveNoDeletions(pods)

				Expect(zau.Status.UpdateStep).Should(Equal(int32(1)))
				Expect(zau.Status.DeletedReplicas).Should(Equal(int32(1)))
			})
		})

		Context("When a new deployment starts before the last one finishes", func() {
			It("It should reset the UpdateStep counter and only terminate a single pod", func() {
				ss, zau, pods := createResources("zau-test10", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType
				testUtils.UpdateZauStep(zau, 3, "oldRev")

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				expectLastPodInFirstZoneToBeDeleted(zau, pods)
			})
		})

		Context("When PauseRolloutAlarm is not in alarm", func() {
			It("It should not pause the rollout", func() {
				ss, zau, pods := createResources("zau-test30", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType
				zau.Spec.PauseRolloutAlarm = "anyAlarm"

				controller.AlarmStateProvider = &mockAlarmStateProvider{state: types.StateValueOk, err: nil}

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				expectLastPodInFirstZoneToBeDeleted(zau, pods)
			})
		})

		Context("When PauseRolloutAlarm is in alarm", func() {
			It("It should pause the rollout", func() {
				ss, zau, pods := createResources("zau-test31", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType
				zau.Spec.PauseRolloutAlarm = "anyAlarm"

				controller.AlarmStateProvider = &mockAlarmStateProvider{state: types.StateValueAlarm, err: nil}

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeTrue())

				expectNoDeletions(zau, pods)
				for zone := range zau.Status.OldReplicas {
					Expect(zau.Status.OldReplicas[zone]).Should(Equal(int32(3)))
				}
			})

			It("It should not pause the rollout when IgnoreAlarm is true", func() {
				ss, zau, pods := createResources("zau-test32", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType
				zau.Spec.PauseRolloutAlarm = "anyAlarm"
				zau.Spec.IgnoreAlarm = true

				controller.AlarmStateProvider = &mockAlarmStateProvider{state: types.StateValueAlarm, err: nil}

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).Should(BeNil())
				Expect(recheck).Should(BeFalse())

				expectLastPodInFirstZoneToBeDeleted(zau, pods)
			})
		})

		Context("When fail to get PauseRolloutAlarm state", func() {
			It("It should pause the rollout", func() {
				ss, zau, pods := createResources("zau-test33", replicas, maxUnavailable, zones)
				ss.Spec.UpdateStrategy.Type = apps.OnDeleteStatefulSetStrategyType
				zau.Spec.PauseRolloutAlarm = "anyAlarm"

				controller.AlarmStateProvider = &mockAlarmStateProvider{state: "", err: fmt.Errorf("anyError")}

				recheck, err := controller.updateStatefulSet(context.TODO(), zau, ss)
				Expect(err).ShouldNot(BeNil())
				Expect(recheck).Should(BeFalse())

				expectNoDeletions(zau, pods)
			})
		})
	})

	Describe("maxPodsToDelete", func() {
		tests := []struct {
			name              string
			maxUnavailable    int
			updateStep        int32
			exponentialFactor string
			result            int
		}{
			{
				name:              "step 0",
				maxUnavailable:    10,
				updateStep:        0,
				exponentialFactor: "2.0",
				result:            1,
			},
			{
				name:              "step 1",
				maxUnavailable:    10,
				updateStep:        1,
				exponentialFactor: "2.0",
				result:            2,
			},
			{
				name:              "step 2",
				maxUnavailable:    10,
				updateStep:        2,
				exponentialFactor: "2.0",
				result:            4,
			},
			{
				name:              "step 4",
				maxUnavailable:    10,
				updateStep:        4,
				exponentialFactor: "2.0",
				result:            10,
			},
			{
				name:              "step 62",
				maxUnavailable:    10,
				updateStep:        62,
				exponentialFactor: "2.0",
				result:            10,
			},
			{
				name:              "step 63 - overflow",
				maxUnavailable:    10,
				updateStep:        63,
				exponentialFactor: "2.0",
				result:            10,
			},
			{
				name:              "exponential 1 - update pods one by one",
				maxUnavailable:    10,
				updateStep:        2,
				exponentialFactor: "1.0",
				result:            1,
			},
			{
				name:              "exponential 0 - disables exponential updates",
				maxUnavailable:    10,
				updateStep:        8,
				exponentialFactor: "0",
				result:            10,
			},
		}
		for _, tt := range tests {
			Context("When "+tt.name, func() {
				It("It should compute the correct number of pods to be deleted.", func() {
					result, err := controller.maxPodsToDelete(tt.maxUnavailable, tt.updateStep, tt.exponentialFactor)
					Expect(err).Should(BeNil())
					Expect(result).Should(Equal(tt.result))
				})
			})
		}
	})

	Describe("updateZauStatus", func() {
		Context("When pods in a single zone are in the old revision", func() {
			It("It should reset other zones in OldReplicas", func() {
				label := "zau-update1"
				ss := testUtils.CreateStatefulSet(int32(replicas), label)
				zau := testUtils.CreateZau(ss.Name, intstr.FromInt(maxUnavailable), label)
				zau.Status.OldReplicas = make(map[string]int32)
				zau.Status.OldReplicas["zone-1"] = 2
				zau.Status.OldReplicas["zone-2"] = 2

				oldPodsCountMap := make(map[string]int32)
				oldPodsCountMap["zone-2"] = 1

				err := controller.updateZauStatus(ctx, zau, ss, 1, 1, oldPodsCountMap, false)
				Expect(err).Should(BeNil())

				Expect(zau.Status.OldReplicas["zone-1"]).Should(Equal(int32(0)))
				Expect(zau.Status.OldReplicas["zone-2"]).Should(Equal(int32(1)))
			})
		})

		Context("When previously computed OldReplicas status don't have all zones", func() {
			It("It should include the new zone", func() {
				label := "zau-update2"
				ss := testUtils.CreateStatefulSet(int32(replicas), label)
				zau := testUtils.CreateZau(ss.Name, intstr.FromInt(maxUnavailable), label)
				zau.Status.OldReplicas = make(map[string]int32)
				zau.Status.OldReplicas["zone-1"] = 2

				oldPodsCountMap := make(map[string]int32)
				oldPodsCountMap["zone-1"] = 3
				oldPodsCountMap["zone-2"] = 1

				err := controller.updateZauStatus(ctx, zau, ss, 1, 1, oldPodsCountMap, false)
				Expect(err).Should(BeNil())

				Expect(zau.Status.OldReplicas["zone-1"]).Should(Equal(int32(3)))
				Expect(zau.Status.OldReplicas["zone-2"]).Should(Equal(int32(1)))
			})
		})
	})

	Describe("findZauForPod", func() {
		Context("When there is a ZAU associated to the Pod's StatefulSet", func() {
			It("It should return a reconcile request with the ZAU information", func() {
				_, zau, pods := createResources("zau-findtest1", 1, maxUnavailable, zones)
				results := controller.findZauForPod(pods[0])
				Expect(len(results)).Should(Equal(1))
				Expect(results[0].Name).Should(Equal(zau.Name))
			})
		})

		Context("When there is no StatefulSet associated to the Pod", func() {
			It("It should not return a reconcile request", func() {
				pod := testUtils.CreatePod("zau-findtest11", zones[0], "anylabel", v1.PodRunning, v1.PodReady)
				results := controller.findZauForPod(pod)
				Expect(len(results)).Should(Equal(0))
			})
		})
	})

	Describe("findZauForStatefulSet", func() {
		Context("When there is a ZAU associated to the StatefulSet", func() {
			It("It should return a reconcile request with the ZAU information", func() {
				ss, zau, _ := createResources("zau-findtest2", 0, maxUnavailable, zones)
				results := controller.findZauForStatefulSet(ss)
				Expect(len(results)).Should(Equal(1))
				Expect(results[0].Name).Should(Equal(zau.Name))
			})
		})

		Context("When there is no ZAU associated to the StatefulSet", func() {
			It("It should not return a reconcile request", func() {
				ss := testUtils.CreateStatefulSet(int32(replicas), "zau-findtest21")
				results := controller.findZauForStatefulSet(ss)
				Expect(len(results)).Should(Equal(0))
			})
		})
	})
})

func expectNoDeletions(zau *opsv1.ZoneAwareUpdate, pods []*v1.Pod) {
	assertHaveNoDeletions(pods)

	Expect(zau.Status.UpdateStep).Should(Equal(int32(0)))
	Expect(zau.Status.DeletedReplicas).Should(Equal(int32(0)))
}

func expectLastPodInFirstZoneToBeDeleted(zau *opsv1.ZoneAwareUpdate, pods []*v1.Pod) {
	assertContainDeletions(pods, []int{6})

	Expect(zau.Status.UpdateStep).Should(Equal(int32(1)))
	Expect(zau.Status.DeletedReplicas).Should(Equal(int32(1)))
	Expect(len(zau.Status.OldReplicas)).Should(Equal(3))
	for zone := range zau.Status.OldReplicas {
		Expect(zau.Status.OldReplicas[zone]).Should(Equal(int32(3)))
	}
}

func createResources(label string, replicas int, maxUnavailable int, zones []string) (*apps.StatefulSet, *opsv1.ZoneAwareUpdate, []*v1.Pod) {
	ss := testUtils.CreateStatefulSet(int32(replicas), label)
	zau := testUtils.CreateZau(ss.Name, intstr.FromInt(maxUnavailable), label)
	pods := createPods(label, replicas, zones, ss)

	// Changing the UpdateRevision to force a rollout
	ss.Status.UpdateRevision = "update"
	ss = testUtils.UpdateStatefulSetStatus(ss)

	return ss, zau, pods
}

// Create n number of pod replicas evenly distributed accross zones.
// Example: 3 replicas, 3 zones
//
//	zone-1: [pod-0, pod-3, pod-6]
//	zone-2: [pod-1, pod-4, pod-7]
//	zone-3: [pod-2, pod-5, pod-8]
func createPods(label string, replicas int, zones []string, ss *apps.StatefulSet) []*v1.Pod {
	pods := []*v1.Pod{}
	for i := 0; i < replicas; {
		for _, zone := range zones {
			pod := testUtils.CreateStatefulSetPod(podName(label, i), zone, v1.PodRunning, label, ss)
			pods = append(pods, pod)
			i++
		}
	}
	return pods
}

func getOrdinal(pod *v1.Pod) int {
	parts := strings.Split(pod.Name, "-")
	id, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return -1
	}
	return id
}

func contains(s []int, e int) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func assertContainDeletions(pods []*v1.Pod, expectedTerminated []int) {
	matchDeletions(pods, expectedTerminated)
}

func assertHaveNoDeletions(pods []*v1.Pod) {
	matchDeletions(pods, []int{})
}

func matchDeletions(pods []*v1.Pod, expectedTerminated []int) (success bool, err error) {
	for i := range pods {
		pod := testUtils.GetPod(pods[i].Name)
		id := getOrdinal(pod)
		if contains(expectedTerminated, id) {
			Expect(utils.IsTerminating(pod)).Should(BeTrue(), "Expected pod # %d to be terminated", id)
		} else {
			Expect(utils.IsTerminating(pod)).Should(BeFalse(), "Expected pod # %d to not be terminated", id)
		}
	}
	return true, nil
}
