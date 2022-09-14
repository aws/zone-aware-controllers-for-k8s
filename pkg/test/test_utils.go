package test

import (
	"context"
	"time"

	. "github.com/onsi/gomega"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"

	opsv1 "github.com/aws/zone-aware-controllers-for-k8s/api/v1"
	"github.com/aws/zone-aware-controllers-for-k8s/pkg/utils"
)

const (
	timeout            = time.Second * 10
	interval           = time.Millisecond * 250
	defaultZdbWaitTime = time.Second * 3
)

type Utils struct {
	Ctx    context.Context
	Client client.Client
}

func (t *Utils) testLabels(value string) map[string]string {
	return map[string]string{"name": value}
}

func (t *Utils) CreateZdb(maxUnavailable intstr.IntOrString, dryRun bool, label string) *opsv1.ZoneDisruptionBudget {
	name := label + "-zdb"
	labels := t.testLabels(label)
	zdb := &opsv1.ZoneDisruptionBudget{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
			Labels:    labels,
		},
		Spec: opsv1.ZoneDisruptionBudgetSpec{
			MaxUnavailable: &maxUnavailable,
			Selector:       &metav1.LabelSelector{MatchLabels: labels},
			DryRun:         dryRun,
		},
	}
	Expect(t.Client.Create(t.Ctx, zdb)).Should(Succeed())
	return zdb
}

func (t *Utils) UpdateZdbDisruptions(zdb *opsv1.ZoneDisruptionBudget, disruptionsAllowed map[string]int32) *opsv1.ZoneDisruptionBudget {
	zdb.Status.DisruptionsAllowed = disruptionsAllowed
	zdb.Status.ObservedGeneration = zdb.Generation
	Expect(t.Client.Status().Update(t.Ctx, zdb)).Should(Succeed())
	return zdb
}

func (t *Utils) GetZdb(name string) *opsv1.ZoneDisruptionBudget {
	// Waiting some time to make sure the controller updates the ZDB status
	time.Sleep(defaultZdbWaitTime)

	zdb := &opsv1.ZoneDisruptionBudget{}
	Eventually(func() bool {
		err := t.Client.Get(t.Ctx, types.NamespacedName{Name: name, Namespace: metav1.NamespaceDefault}, zdb)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	return zdb
}

func (t *Utils) CreatePod(name string, zone string, label string, phase v1.PodPhase, condition v1.PodConditionType) *v1.Pod {
	labels := t.testLabels(label)
	node := t.GetOrCreateNode(name, zone)
	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
			Labels:    labels,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{Name: "any", Image: "any"}},
			NodeName:   node.Name,
		},
	}
	Expect(t.Client.Create(t.Ctx, pod)).Should(Succeed())

	return t.UpdatePodStatus(pod, phase, condition)
}

func (t *Utils) CreateSimplePod(name string, node *v1.Node) *v1.Pod {
	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{Name: "any", Image: "any"}},
			NodeName:   node.Name,
		},
	}
	Expect(t.Client.Create(t.Ctx, pod)).Should(Succeed())

	key := types.NamespacedName{
		Name:      pod.Name,
		Namespace: metav1.NamespaceDefault,
	}
	Eventually(func() bool {
		err := t.Client.Get(t.Ctx, key, pod)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	return pod
}

func (t *Utils) CreateStatefulSetPod(name string, zone string, phase v1.PodPhase, label string, ss *apps.StatefulSet) *v1.Pod {
	labels := t.testLabels(label)
	labels[apps.ControllerRevisionHashLabelKey] = ss.Status.CurrentRevision
	node := t.GetOrCreateNode(name, zone)
	var trueVar = true
	controllerRef := metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       utils.ControllerKindSS.Kind,
		Name:       ss.Name,
		UID:        ss.UID,
		Controller: &trueVar,
	}
	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       metav1.NamespaceDefault,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{controllerRef},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{Name: "any", Image: "any"}},
			NodeName:   node.Name,
		},
	}
	Expect(t.Client.Create(t.Ctx, pod)).Should(Succeed())

	return t.UpdatePodStatus(pod, phase, v1.PodReady)
}

func (t *Utils) UpdatePodStatus(pod *v1.Pod, phase v1.PodPhase, condition v1.PodConditionType) *v1.Pod {
	pod.Status = v1.PodStatus{
		Phase: phase,
		Conditions: []v1.PodCondition{
			{Type: condition, Status: v1.ConditionTrue},
		},
	}
	Expect(t.Client.Status().Update(t.Ctx, pod)).Should(Succeed())
	key := types.NamespacedName{
		Name:      pod.Name,
		Namespace: metav1.NamespaceDefault,
	}
	Eventually(func() bool {
		err := t.Client.Get(t.Ctx, key, pod)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	Expect(pod.Status.Phase).Should(Equal(phase))
	return pod
}

func (t *Utils) UpdatePod(pod *v1.Pod) *v1.Pod {
	Expect(t.Client.Update(t.Ctx, pod)).Should(Succeed())
	key := types.NamespacedName{
		Name:      pod.Name,
		Namespace: metav1.NamespaceDefault,
	}
	Eventually(func() bool {
		err := t.Client.Get(t.Ctx, key, pod)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	return pod
}

func (t *Utils) GetPod(podName string) *v1.Pod {
	pod := &v1.Pod{}
	key := types.NamespacedName{
		Name:      podName,
		Namespace: metav1.NamespaceDefault,
	}
	Eventually(func() bool {
		err := t.Client.Get(t.Ctx, key, pod)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	return pod
}

func (t *Utils) GetPodZone(pod *v1.Pod) string {
	node := &v1.Node{}
	key := types.NamespacedName{
		Name:      pod.Spec.NodeName,
		Namespace: metav1.NamespaceDefault,
	}
	Eventually(func() bool {
		err := t.Client.Get(t.Ctx, key, node)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	return node.ObjectMeta.Labels[v1.LabelTopologyZone]
}

func (t *Utils) GetOrCreateNode(podName string, zone string) *v1.Node {
	nodeName := "node-" + zone + "-" + podName
	key := types.NamespacedName{
		Name:      nodeName,
		Namespace: metav1.NamespaceDefault,
	}
	node := &v1.Node{}
	err := t.Client.Get(t.Ctx, key, node)
	if errors.IsNotFound(err) {
		node = &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   nodeName,
				Labels: map[string]string{v1.LabelTopologyZone: zone},
			},
		}
		Expect(t.Client.Create(t.Ctx, node)).Should(Succeed())
	}
	return node
}

func (t *Utils) CreateStatefulSet(replicas int32, label string) *apps.StatefulSet {
	ssName := label + "-ss"
	labels := t.testLabels(label)
	ss := &apps.StatefulSet{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1"},
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Name:      ssName,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: apps.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
			},
		},
	}
	Expect(t.Client.Create(t.Ctx, ss)).Should(Succeed())
	return t.GetStatefulSet(ss.Name)
}

func (t *Utils) GetStatefulSet(ssName string) *apps.StatefulSet {
	ssNew := &apps.StatefulSet{}
	key := types.NamespacedName{
		Name:      ssName,
		Namespace: metav1.NamespaceDefault,
	}
	Eventually(func() bool {
		err := t.Client.Get(t.Ctx, key, ssNew)
		return err == nil
	}, timeout, interval).Should(BeTrue())
	return ssNew
}

func (t *Utils) CreateZau(statefulset string, maxUnavailable intstr.IntOrString, label string) *opsv1.ZoneAwareUpdate {
	name := label + "-zau"
	labels := t.testLabels(label)
	zau := &opsv1.ZoneAwareUpdate{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
			Labels:    labels,
		},
		Spec: opsv1.ZoneAwareUpdateSpec{
			MaxUnavailable: &maxUnavailable,
			StatefulSet:    statefulset,
		},
	}
	Expect(t.Client.Create(t.Ctx, zau)).Should(Succeed())
	return zau
}

func (t *Utils) UpdateZauStep(zau *opsv1.ZoneAwareUpdate, step int32, updateRevision string) *opsv1.ZoneAwareUpdate {
	zau.Status.UpdateStep = step
	zau.Status.UpdateRevision = updateRevision
	Expect(t.Client.Status().Update(t.Ctx, zau)).Should(Succeed())
	return zau
}

func (t *Utils) UpdateStatefulSetStatus(ss *apps.StatefulSet) *apps.StatefulSet {
	Expect(t.Client.Status().Update(t.Ctx, ss)).Should(Succeed())
	return t.GetStatefulSet(ss.Name)
}

func (t *Utils) DeletePod(pod *v1.Pod) *v1.Pod {
	Expect(t.Client.Delete(t.Ctx, pod, client.GracePeriodSeconds(1000))).Should(Succeed())
	return t.GetPod(pod.Name)
}
