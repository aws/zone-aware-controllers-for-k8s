//go:build integration

package e2e

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// GetPodZone returns the zone in which the pod is scheduled
func GetPodZone(ctx context.Context, cfg *envconf.Config, pod corev1.Pod) (string, error) {
	var node corev1.Node
	if err := cfg.Client().Resources().Get(ctx, pod.Spec.NodeName, pod.Namespace, &node); err != nil {
		return "", err
	}
	zone, found := node.ObjectMeta.Labels[corev1.LabelTopologyZone]
	if !found {
		return "", errors.New("pod is scheduled on node without zone label")
	}

	return zone, nil
}
