package zau

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetNamespaceNames(t *testing.T) {
	for _, testcase := range []struct {
		description string
		namespaces  corev1.NamespaceList
		expected    []string
	}{
		{
			description: "no namespaces",
			namespaces: corev1.NamespaceList{
				Items: []corev1.Namespace{},
			},
			expected: nil,
		},
		{
			description: "one namespace",
			namespaces: corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
				},
			},
			expected: []string{"foo"},
		},
		{
			description: "many namespaces",
			namespaces: corev1.NamespaceList{
				Items: []corev1.Namespace{
					{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "bar"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "baz"}},
				},
			},
			expected: []string{"foo", "bar", "baz"},
		},
	} {
		t.Run(testcase.description, func(t *testing.T) {
			got := getNamespaceNames(testcase.namespaces)
			if !reflect.DeepEqual(testcase.expected, got) {
				t.Fatalf("got: %v; wanted: %v", got, testcase.expected)
			}
		})
	}
}

func TestGetPodNames(t *testing.T) {
	for _, testcase := range []struct {
		description string
		namespaces  corev1.PodList
		expected    []string
	}{
		{
			description: "no pods",
			namespaces: corev1.PodList{
				Items: []corev1.Pod{},
			},
			expected: nil,
		},
		{
			description: "one pod",
			namespaces: corev1.PodList{
				Items: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
				},
			},
			expected: []string{"foo"},
		},
		{
			description: "many pods",
			namespaces: corev1.PodList{
				Items: []corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "bar"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "baz"}},
				},
			},
			expected: []string{"foo", "bar", "baz"},
		},
	} {
		t.Run(testcase.description, func(t *testing.T) {
			got := getPodNames(testcase.namespaces)
			if !reflect.DeepEqual(testcase.expected, got) {
				t.Fatalf("got: %v; wanted: %v", got, testcase.expected)
			}
		})
	}
}
