//go:build integration

package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/e2e-framework/klient/k8s"

	. "github.com/aws/zone-aware-controllers-for-k8s/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// DisruptZones will put a single pod into an unhealthy state in each specified zone and wait until
// the pods are not ready
func DisruptZones(ctx context.Context, t *testing.T, cfg *envconf.Config, zones []string) error {
	t.Logf("forcing pod failures in zones %v", zones)
	var pods corev1.PodList
	if err := cfg.Client().Resources(NamespaceFromContext(ctx)).List(ctx, &pods); err != nil {
		return err
	}

	podsByZone := make(map[string][]corev1.Pod)
	for _, pod := range pods.Items {
		zone, err := GetPodZone(ctx, cfg, pod)
		if err != nil {
			return fmt.Errorf("getting pod zone: %v", err)
		}
		podsByZone[zone] = append(podsByZone[zone], pod)
	}

	chooseString := func(pods []corev1.Pod) corev1.Pod {
		return pods[rand.Intn(len(pods))]
	}
	var failureTargets []string // names of the pods with faults injected
	for _, zone := range zones {
		choice := chooseString(podsByZone[zone])
		failureTargets = append(failureTargets, choice.Name)
		t.Logf("forcing pod failure for %v in zone %v", choice.Name, zone)

		patch := podImagePatch("gcr.io/google-containers/pause:latest")
		if err := cfg.Client().Resources(NamespaceFromContext(ctx)).Patch(ctx, &choice, patch); err != nil {
			return err
		}
	}

	t.Logf("waiting for pod failure")
	err := wait.For(func() (bool, error) {
		var pods corev1.PodList
		if err := cfg.Client().Resources(NamespaceFromContext(ctx)).List(ctx, &pods); err != nil {
			return false, fmt.Errorf("listing pods: %v", err)
		}
		var failed []string
		for _, pod := range pods.Items {
			if !IsPodReady(&pod) {
				failed = append(failed, pod.Name)
			}
		}
		sort.Strings(failureTargets)
		sort.Strings(failed)
		t.Logf("found failed pods %v while waiting for %v", failed, failureTargets)
		return reflect.DeepEqual(failureTargets, failed), nil
	})
	if err != nil {
		return fmt.Errorf("waiting for pods to fail: %v", err)
	}

	return nil
}

func podImagePatch(image string) k8s.Patch {
	return k8s.Patch{
		PatchType: types.StrategicMergePatchType,
		Data: []byte(fmt.Sprintf(`
        {
          "spec": {
            "containers": [{"name": "nginx", "image": "%s"}]
          }
        }`, image)),
	}
}
