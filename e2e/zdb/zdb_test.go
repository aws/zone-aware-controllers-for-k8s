//go:build integration

package zdb

import (
	"context"
	"math/rand"
	"testing"

	operatorv1 "github.com/aws/zone-aware-controllers-for-k8s/api/v1"
	"github.com/aws/zone-aware-controllers-for-k8s/e2e"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestE2EZdbEviction(t *testing.T) {
	f1 := features.New("eviction allowance").
		Assess("pods successfully evicted until reaching threshold", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			g := NewWithT(t)

			var pods corev1.PodList
			err := cfg.Client().Resources(e2e.NamespaceFromContext(ctx)).List(ctx, &pods)
			g.Expect(err).To(BeNil())
			g.Expect(pods.Items).ToNot(BeEmpty())

			var zdb operatorv1.ZoneDisruptionBudget
			err = cfg.Client().Resources().Get(ctx, zdbNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &zdb)
			g.Expect(err).To(BeNil())

			var ss appsv1.StatefulSet
			err = cfg.Client().Resources().Get(ctx, statefulSetNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &ss)
			g.Expect(err).To(BeNil())

			// select the zone to evict from
			zones := e2e.ZonesFromContext(ctx)
			g.Expect(zones).ToNot(BeEmpty())
			evictionZone := chooseString(zones)
			t.Logf("targeting zone %v for evictions in namespace %v", evictionZone, e2e.NamespaceFromContext(ctx))

			maxUnavailable, err := intstr.GetScaledValueFromIntOrPercent(zdb.Spec.MaxUnavailable, int(*ss.Spec.Replicas), true)
			g.Expect(err).To(BeNil())

			var numEvicted int
			for _, pod := range pods.Items {
				zone, found := e2e.PodZoneFromContext(ctx, pod.Name)
				g.Expect(found).To(BeTrue())
				if zone != evictionZone {
					continue
				}

				t.Logf("attempting eviction for pod %v in namespace %v", pod.Name, e2e.NamespaceFromContext(ctx))
				err = evictPod(ctx, cfg, pod.Name)

				// if error comes from webhook rejection, stop evicting pods
				// otherwise this error is unexpected
				if errors.IsForbidden(err) {
					t.Logf("eviction blocked, stopping evictions")
					break
				}
				g.Expect(err).To(BeNil())

				numEvicted++
			}

			g.Expect(numEvicted).To(BeNumerically("<=", maxUnavailable))

			return ctx
		}).Feature()

	f2 := features.New("eviction blocking").
		Assess("pods not evicted when any other zones are disrupted", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			g := NewWithT(t)

			zones := e2e.ZonesFromContext(ctx)
			g.Expect(len(zones)).To(BeNumerically(">", 1))

			// compute the list of zones to disrupt and the zone to evict from
			rand.Shuffle(len(zones), func(i, j int) {
				zones[i], zones[j] = zones[j], zones[i]
			})
			evictionZone := zones[0]
			numDisruptedZones := rand.Intn(len(zones)-1) + 1 // disrupt [1, n-1) zones
			disruptedZones := chooseStrings(numDisruptedZones, zones[1:])

			err := e2e.DisruptZones(ctx, t, cfg, disruptedZones)
			g.Expect(err).To(BeNil())

			pods := e2e.ZonePodsFromContext(ctx, evictionZone)
			pod := chooseString(pods)
			t.Logf("attempting to evict pod %v in zone %v", pod, evictionZone)
			err = evictPod(ctx, cfg, pod)
			g.Expect(err).ToNot(BeNil())
			g.Expect(errors.IsForbidden(err)).To(BeTrue())

			return ctx
		}).Feature()

	testenv.Test(t, f1, f2)
}

// evictPod creates an eviction object for the given pod
//
// it is important that this function preserves errors from the client
// so that they can be inspected from tests
func evictPod(ctx context.Context, cfg *envconf.Config, pod string) error {
	// native e2e-framework client does not support evictions, so we use the k8s go client
	client, err := kubernetes.NewForConfig(cfg.Client().RESTConfig())
	if err != nil {
		return err
	}
	ns := e2e.NamespaceFromContext(ctx)
	eviction := policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod,
			Namespace: ns,
		},
	}
	return client.PolicyV1beta1().Evictions(ns).Evict(ctx, &eviction)
}

// chooseStrings returns a random selection of n strings from the given list
func chooseStrings(n int, strs []string) []string {
	cp := make([]string, len(strs))
	copy(cp, strs)
	rand.Shuffle(len(cp), func(i, j int) {
		cp[i], cp[j] = cp[j], cp[i]
	})
	return cp[:n]
}

// chooseString returns a random string from the given list
func chooseString(strs []string) string {
	return chooseStrings(1, strs)[0]
}
