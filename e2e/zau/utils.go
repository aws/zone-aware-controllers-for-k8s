package zau

import (
	corev1 "k8s.io/api/core/v1"
)

func getNamespaceNames(list corev1.NamespaceList) []string {
	var out []string
	for _, ns := range list.Items {
		out = append(out, ns.Name)
	}
	return out
}

func getPodNames(list corev1.PodList) []string {
	var out []string
	for _, pod := range list.Items {
		out = append(out, pod.Name)
	}
	return out
}
