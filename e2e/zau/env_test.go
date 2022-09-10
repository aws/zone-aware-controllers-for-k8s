//go:build integration

package zau

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	operatorv1 "github.com/aws/zone-aware-controllers-for-k8s/api/v1"
	"github.com/aws/zone-aware-controllers-for-k8s/e2e"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// createStatefulSet instantiates a StatefulSet for controllers to target in tests
func createStatefulSet(ctx context.Context, cfg *envconf.Config, t *testing.T, _ features.Feature) (context.Context, error) {
	decoder := scheme.Codecs.UniversalDecoder()

	ss := appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: e2e.NamespaceFromContext(ctx)}}
	if _, _, err := decoder.Decode(statefulsetManifest, nil, &ss); err != nil {
		return ctx, fmt.Errorf("decoding StatefulSet manifest: %v", err)
	}

	t.Logf("creating test StatefulSet in namespace %v", ss.Namespace)

	return contextWithStatefulSetName(ctx, ss.Name), cfg.Client().Resources().Create(ctx, &ss)
}

// createZAU initializes a ZAU instance in the test namespace
func createZAU(ctx context.Context, cfg *envconf.Config, t *testing.T, _ features.Feature) (context.Context, error) {
	decoder := scheme.Codecs.UniversalDecoder()

	zau := operatorv1.ZoneAwareUpdate{ObjectMeta: metav1.ObjectMeta{Namespace: e2e.NamespaceFromContext(ctx)}}
	if _, _, err := decoder.Decode(zauManifest, nil, &zau); err != nil {
		return ctx, fmt.Errorf("decoding zau manifest: %v", err)
	}

	t.Logf("creating zau in test namespace %v", zau.Namespace)

	return contextWithZauName(ctx, zau.Name), cfg.Client().Resources().Create(ctx, &zau)
}

// waitForHealthyEnvironment confirms that tests are targeting a healthy environment
func waitForHealthyEnvironment(ctx context.Context, cfg *envconf.Config, t *testing.T, _ features.Feature) (context.Context, error) {
	var deployment appsv1.Deployment
	deploymentName := getEnv("E2E_DEPLOYMENT", "zone-aware-controllers-controller-manager")
	namespace := getEnv("E2E_NAMESPACE", "zone-aware-controllers-system")
	if err := cfg.Client().Resources().Get(ctx, deploymentName, namespace, &deployment); err != nil {
		return ctx, err
	}
	if deployment.Status.UnavailableReplicas != 0 {
		return ctx, errors.New("zone-aware-controllers has unavailable replicas")
	}

	err := wait.For(func() (done bool, err error) {
		var ss appsv1.StatefulSet
		if err := cfg.Client().Resources().Get(ctx, statefulSetNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &ss); err != nil {
			return false, err
		}
		t.Logf("statefulset has %d/%d ready replicas", ss.Status.ReadyReplicas, *ss.Spec.Replicas)
		return ss.Status.ReadyReplicas == *ss.Spec.Replicas, nil
	})
	if err != nil {
		return ctx, fmt.Errorf("waiting for ready StatefulSet replicas: %v", err)
	}

	return ctx, nil
}

func getEnv(key, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = defaultValue
	}
	return value
}

func statefulSetNameFromContext(ctx context.Context) string {
	return ctx.Value("statefulset-under-test").(string)
}

func contextWithStatefulSetName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, "statefulset-under-test", name)
}

func contextWithZauName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, "test-zau", name)
}

func zauNameFromContext(ctx context.Context) string {
	return ctx.Value("test-zau").(string)
}
