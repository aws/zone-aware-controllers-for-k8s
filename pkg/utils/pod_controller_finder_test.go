package utils

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
)

func createStatefulSet(replicas int32) *apps.StatefulSet {
	labels := map[string]string{"foo": "bar"}
	ss := &apps.StatefulSet{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Name:      "foobar",
			Namespace: metav1.NamespaceDefault,
			Labels:    labels,
		},
		Spec: apps.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), ss)).Should(Succeed())
	return ss
}

var _ = Describe("getPodStatefulSet", func() {
	Context("When there is a statefulset associated to the pod", func() {
		It("Should return the correct controller and scale", func() {
			expectedScale := int32(10)
			ss := createStatefulSet(expectedScale)
			controllerRef := &metav1.OwnerReference{
				APIVersion: "apps/v1",
				Kind:       ControllerKindSS.Kind,
				Name:       ss.Name,
				UID:        ss.UID,
			}
			result, err := getPodStatefulSet(k8sClient, controllerRef, metav1.NamespaceDefault)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.UID).To(Equal(ss.UID))
			Expect(result.Scale).To(Equal(expectedScale))
		})
	})
	Context("When there is no statefulset associated to the pod", func() {
		It("Should return nil", func() {
			controllerRef := &metav1.OwnerReference{
				APIVersion: "apps/v1",
				Kind:       ControllerKindSS.Kind,
				Name:       "Some statefulset",
				UID:        uuid.NewUUID(),
			}
			result, err := getPodStatefulSet(k8sClient, controllerRef, metav1.NamespaceDefault)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})
})
