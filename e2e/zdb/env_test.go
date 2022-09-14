//go:build integration

package zdb

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/zone-aware-controllers-for-k8s/e2e"

	operatorv1 "github.com/aws/zone-aware-controllers-for-k8s/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// createZDB initializes a ZDB instance in the test namespace
func createZDB(ctx context.Context, cfg *envconf.Config, t *testing.T, _ features.Feature) (context.Context, error) {
	decoder := scheme.Codecs.UniversalDecoder()

	zdb := operatorv1.ZoneDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Namespace: e2e.NamespaceFromContext(ctx)}}
	if _, _, err := decoder.Decode(zdbManifest, nil, &zdb); err != nil {
		return ctx, fmt.Errorf("decoding zdb manifest: %v", err)
	}

	t.Logf("creating zdb in test namespace %v", zdb.Namespace)

	return contextWithZdbName(ctx, zdb.Name), cfg.Client().Resources().Create(ctx, &zdb)
}

// createStatefulSet initializes a StatefulSet to use in controller tests
func createStatefulSet(ctx context.Context, cfg *envconf.Config, t *testing.T, _ features.Feature) (context.Context, error) {
	decoder := scheme.Codecs.UniversalDecoder()

	manifest := appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: e2e.NamespaceFromContext(ctx)}}
	if _, _, err := decoder.Decode(statefulsetManifest, nil, &manifest); err != nil {
		return ctx, fmt.Errorf("decoding StatefulSet manifest: %v", err)
	}

	t.Logf("creating test StatefulSet in namespace %v", manifest.Namespace)

	err := cfg.Client().Resources().Create(ctx, &manifest)
	if err != nil {
		return ctx, err
	}

	err = wait.For(func() (done bool, err error) {
		var ss appsv1.StatefulSet
		_err := cfg.Client().Resources().Get(ctx, manifest.Name, e2e.NamespaceFromContext(ctx), &ss)
		if _err != nil {
			return false, _err
		}
		return ss.Status.ReadyReplicas == *ss.Spec.Replicas, nil
	})

	return contextWithStatefulSetName(ctx, manifest.Name), nil
}

func contextWithZdbName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, "test-zdb", name)
}

func zdbNameFromContext(ctx context.Context) string {
	return ctx.Value("test-zdb").(string)
}

func contextWithStatefulSetName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, "statefulset-under-test", name)
}

func statefulSetNameFromContext(ctx context.Context) string {
	return ctx.Value("statefulset-under-test").(string)
}
