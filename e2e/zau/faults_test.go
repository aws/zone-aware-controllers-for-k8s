//go:build integration

package zau

import (
	"context"
	"fmt"

	operatorv1 "github.com/aws/zone-aware-controllers-for-k8s/api/v1"

	"github.com/aws/zone-aware-controllers-for-k8s/e2e"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// configurePauseAlarm updates zdb.Spec.pauseRolloutAlarm
func configurePauseAlarm(ctx context.Context, cfg *envconf.Config, alarmName string) error {
	var zau operatorv1.ZoneAwareUpdate
	if err := cfg.Client().Resources(e2e.NamespaceFromContext(ctx)).Get(ctx, zauNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &zau); err != nil {
		return fmt.Errorf("getting statefulset: %v", err)
	}
	return cfg.Client().Resources().Patch(ctx, &zau, pauseAlarmPatch(alarmName))
}

// deployHealthyImage will upgrade the StatefulSet under test to a working container image
func deployHealthyImage(ctx context.Context, cfg *envconf.Config) error {
	return deployImage(ctx, cfg, "registry.k8s.io/nginx-slim:0.8")
}

// deployFaultyImage will upgrade the StatefulSet under test to a container image that will cause a pod failure
func deployFaultyImage(ctx context.Context, cfg *envconf.Config) error {
	return deployImage(ctx, cfg, "gcr.io/google-containers/pause:latest")
}

// deployImage will upgrade the StatefulSet pods to run the given image
func deployImage(ctx context.Context, cfg *envconf.Config, image string) error {
	var ss appsv1.StatefulSet
	if err := cfg.Client().Resources(e2e.NamespaceFromContext(ctx)).Get(ctx, statefulSetNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &ss); err != nil {
		return fmt.Errorf("getting statefulset: %v", err)
	}
	patch := statefulsetImagePatch(image)
	return cfg.Client().Resources(e2e.NamespaceFromContext(ctx)).Patch(ctx, &ss, patch)
}
