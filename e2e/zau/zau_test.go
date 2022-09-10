//go:build integration

package zau

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/aws/zone-aware-controllers-for-k8s/e2e"

	"github.com/aws/zone-aware-controllers-for-k8s/pkg/utils"

	operatorv1 "github.com/aws/zone-aware-controllers-for-k8s/api/v1"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestE2EZauSimpleDeployment(t *testing.T) {
	t.Parallel()

	f := features.New("simple deployment").
		Assess("pods converge on updated label", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			g := NewWithT(t)
			err := triggerUpgrade(ctx, t, cfg)
			g.Expect(err).To(BeNil())

			err = wait.For(upgradeToFinish(ctx, t, cfg), wait.WithTimeout(10*time.Minute))
			g.Expect(err).To(BeNil())

			return ctx
		}).
		Assess("statefulset pods are never down in multiple AZs", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			g := NewWithT(t)
			err := triggerUpgrade(ctx, t, cfg)
			g.Expect(err).To(BeNil())

			t.Log("polling pod terminations until deployment is finished")
			for {
				breaching, err := disruptionsAreBreaching(ctx, t, cfg)
				g.Expect(err).To(BeNil())
				g.Expect(breaching).To(BeFalse())

				p, err := percentOfReplicasUpgraded(ctx, cfg)
				g.Expect(err).To(BeNil())
				t.Logf("%d%% of replicas have been upgraded", p)
				if p >= 100 {
					break
				}
			}

			return ctx
		}).Feature()

	testenv.Test(t, f)
}

func TestE2EZauAttemptsDeployment(t *testing.T) {
	t.Parallel()

	f := features.New("unhealthy legacy pod in first zone").
		Assess("deployment proceeds", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			g := NewWithT(t)
			zones := e2e.ZonesFromContext(ctx)
			sort.Strings(zones)
			disruptedZones := zones[:1]
			t.Logf("applying faulty image to pods in zones %v", disruptedZones)
			err := e2e.DisruptZones(ctx, t, cfg, disruptedZones)
			g.Expect(err).To(BeNil())

			err = triggerUpgrade(ctx, t, cfg)
			g.Expect(err).To(BeNil())

			err = wait.For(zauToDeleteReplicas(ctx, t, cfg))
			g.Expect(err).To(BeNil())

			err = wait.For(upgradeToFinish(ctx, t, cfg))
			g.Expect(err).To(BeNil())

			return ctx
		}).Feature()

	testenv.Test(t, f)
}

func TestE2EZauPausesDeployment(t *testing.T) {
	t.Parallel()

	f1 := features.New("bad upgrade version").
		Assess("deployment pauses", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			g := NewWithT(t)
			t.Log("deploying faulty image")
			err := deployFaultyImage(ctx, cfg)
			g.Expect(err).To(BeNil())

			err = wait.For(zauToDeleteReplicas(ctx, t, cfg))
			g.Expect(err).To(BeNil())

			// get zau state after starting deployment
			var zau operatorv1.ZoneAwareUpdate
			err = cfg.Client().Resources().Get(ctx, zauNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &zau)
			g.Expect(err).To(BeNil())

			t.Log("polling zau to verify paused deployment")
			const pollInterval = 5 * time.Second
			for i := 0; i < 12; i++ {
				time.Sleep(pollInterval)
				var _zau operatorv1.ZoneAwareUpdate
				err = cfg.Client().Resources().Get(ctx, zauNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &_zau)
				g.Expect(err).To(BeNil())
				g.Expect(_zau.Status.UpdateStep).To(Equal(zau.Status.UpdateStep))
				g.Expect(_zau.Status.CurrentRevision).To(Equal(zau.Status.CurrentRevision))
			}

			return ctx
		}).Feature()

	f2 := features.New("unhealthy pods in multiple zones").
		Assess("deployment pauses", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			g := NewWithT(t)
			zones := e2e.ZonesFromContext(ctx)
			sort.Strings(zones)
			disruptedZones := zones[1:]
			t.Logf("applying faulty image to pods in zones %v", disruptedZones)
			err := e2e.DisruptZones(ctx, t, cfg, disruptedZones)
			g.Expect(err).To(BeNil())

			err = triggerUpgrade(ctx, t, cfg)
			g.Expect(err).To(BeNil())

			// get zau state after starting deployment
			var zau operatorv1.ZoneAwareUpdate
			err = cfg.Client().Resources().Get(ctx, zauNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &zau)
			g.Expect(err).To(BeNil())

			// poll zau periodically to verify no update steps were taken
			t.Log("polling zau to verify paused deployment")
			const pollInterval = 5 * time.Second
			for i := 0; i < 12; i++ {
				time.Sleep(pollInterval)
				var _zau operatorv1.ZoneAwareUpdate
				err = cfg.Client().Resources().Get(ctx, zauNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &_zau)
				g.Expect(err).To(BeNil())
				g.Expect(int(_zau.Status.UpdateStep)).To(Equal(0))
				g.Expect(int(_zau.Status.DeletedReplicas)).To(Equal(0))
				g.Expect(_zau.Status.CurrentRevision).To(Equal(zau.Status.CurrentRevision))
			}

			return ctx
		}).Feature()

	testenv.Test(t, f1, f2)
}

func TestE2EZauRollback(t *testing.T) {
	t.Parallel()

	f1 := features.New("rollback bad image").
		Assess("deployment eventually succeeds", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			g := NewWithT(t)
			t.Log("deploying faulty image")
			err := deployFaultyImage(ctx, cfg)
			g.Expect(err).To(BeNil())

			err = wait.For(zauToDeleteReplicas(ctx, t, cfg))
			g.Expect(err).To(BeNil())

			// get zau state after starting deployment
			var zau operatorv1.ZoneAwareUpdate
			err = cfg.Client().Resources().Get(ctx, zauNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &zau)
			g.Expect(err).To(BeNil())

			t.Log("polling zau to verify paused deployment")
			const pollInterval = 5 * time.Second
			for i := 0; i < 12; i++ {
				time.Sleep(pollInterval)
				var _zau operatorv1.ZoneAwareUpdate
				err = cfg.Client().Resources().Get(ctx, zauNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &_zau)
				g.Expect(err).To(BeNil())
				g.Expect(_zau.Status.UpdateStep).To(Equal(zau.Status.UpdateStep))
			}

			t.Log("deploying working image")
			err = deployHealthyImage(ctx, cfg)
			g.Expect(err).To(BeNil())

			t.Log("waiting for deployment to finish")
			err = wait.For(upgradeToFinish(ctx, t, cfg))
			g.Expect(err).To(BeNil())

			return ctx
		}).Feature()

	testenv.Test(t, f1)
}

func TestE2EZauOverlappingDeployments(t *testing.T) {
	t.Parallel()

	f1 := features.New("overlapping deployment").
		Assess("no disruption breach", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			g := NewWithT(t)
			upgrades := []string{"k8s.gcr.io/nginx-slim:0.6", "k8s.gcr.io/nginx-slim:0.7", "k8s.gcr.io/nginx-slim:0.8", "k8s.gcr.io/nginx-slim:0.9"}

			for _, upgrade := range upgrades {
				t.Logf("updated statefulset image to %s", upgrade)
				err := deployImage(ctx, cfg, upgrade)
				g.Expect(err).To(BeNil())

				t.Log("waiting for zau to start deployment")
				err = wait.For(zauToDeleteReplicas(ctx, t, cfg))
				g.Expect(err).To(BeNil())

				t.Log("polling to make assertions during deployment")
				for {
					breaching, err := disruptionsAreBreaching(ctx, t, cfg)
					g.Expect(err).To(BeNil())
					g.Expect(breaching).To(BeFalse(), "pod disruptions are breaching")

					p, err := percentOfReplicasUpgraded(ctx, cfg)
					g.Expect(err).To(BeNil())
					t.Logf("%d%% of replicas have been upgraded", p)
					if p >= 50 {
						break
					}
				}
			}

			t.Log("waiting for deployment to finish")
			err := wait.For(upgradeToFinish(ctx, t, cfg))
			g.Expect(err).To(BeNil())

			t.Log("verifying all pods are upgraded to latest version")
			images, err := getDeployedImages(ctx, cfg)
			g.Expect(images).To(HaveLen(1))
			g.Expect(images).To(Equal(upgrades[len(upgrades)-1:]))

			return ctx
		}).Feature()

	testenv.Test(t, f1)
}

// Note: this test (and the operator) make some assumptions about infrastructure and the context of the test runner
// * Test runner environment must have AWS credentials configured
// * The AWS account must have a composite alarm named "PauseDeployment.Test" with a child alarm named "PauseDeployment.Child"
// * The child alarm should be configured with statistic "Maximum" so that publishing a value of alarm.threshold + 1 triggers the alarm
//
// The test will publish metrics to trigger the child alarm
func TestE2EZauRolloutAlarm(t *testing.T) {
	f1 := features.New("pause deployment").
		Assess("triggered alarm pauses deployment", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			g := NewWithT(t)

			const parentAlarmName, childAlarmName = "PauseDeployment.Test", "PauseDeployment.Child"

			t.Log("retrieving child pause alarm from cloudwatch")
			cw := cloudwatch.NewFromConfig(e2e.AWSConfigFromContext(ctx))
			alarmDescription, err := cw.DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{
				AlarmNames: []string{childAlarmName},
				AlarmTypes: []cwtypes.AlarmType{cwtypes.AlarmTypeMetricAlarm},
			})
			g.Expect(err).To(BeNil())
			g.Expect(alarmDescription.MetricAlarms).To(HaveLen(1))
			childAlarm := alarmDescription.MetricAlarms[0]

			// because the test alarm is effectively globally shared between tests
			// we wait for the alarm to enter an OK state in case it is still in ALARM
			// state from a prior test run
			t.Logf("waiting for alarm %v to enter OK state", parentAlarmName)
			err = wait.For(compositeAlarmState(ctx, t, cfg, parentAlarmName, cwtypes.StateValueOk))

			t.Logf("configuring zau pause alarm: %v", parentAlarmName)
			err = configurePauseAlarm(ctx, cfg, parentAlarmName)
			g.Expect(err).To(BeNil())

			t.Log("triggering statefulset upgrade")
			err = triggerUpgrade(ctx, t, cfg)
			g.Expect(err).To(BeNil())

			t.Log("triggering statefulset upgrade")
			err = wait.For(zauToDeleteReplicas(ctx, t, cfg))
			g.Expect(err).To(BeNil())

			t.Log("triggering pause alarm")
			_, err = cw.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
				Namespace: childAlarm.Namespace,
				MetricData: []cwtypes.MetricDatum{
					{
						MetricName: childAlarm.MetricName,
						Value:      aws.Float64(aws.ToFloat64(childAlarm.Threshold) + 1),
					},
				},
			})
			g.Expect(err).To(BeNil())

			err = wait.For(compositeAlarmState(ctx, t, cfg, parentAlarmName, cwtypes.StateValueAlarm))
			g.Expect(err).To(BeNil())

			// get baseline zau state
			var zau operatorv1.ZoneAwareUpdate
			err = cfg.Client().Resources().Get(ctx, zauNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &zau)
			g.Expect(err).To(BeNil())

			// polling time is brief here to avoid flakiness, e.g. situations where
			// the alarm enters OK state in later loop iterations and the ZAU proceeds
			// with updates before the test is done polling
			t.Log("polling zau to verify paused deployment")
			pollInterval := 5 * time.Second
			for i := 0; i < 6; i++ {
				time.Sleep(pollInterval)
				var _zau operatorv1.ZoneAwareUpdate
				err = cfg.Client().Resources().Get(ctx, zauNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &_zau)
				g.Expect(err).To(BeNil())
				g.Expect(_zau.Status.UpdateStep).To(Equal(zau.Status.UpdateStep))
				g.Expect(_zau.Status.CurrentRevision).To(Equal(zau.Status.CurrentRevision))
			}

			// alarm period should pass and the alarm should enter OK state
			t.Log("waiting for alarm to enter OK state")
			err = wait.For(compositeAlarmState(ctx, t, cfg, parentAlarmName, cwtypes.StateValueOk))
			g.Expect(err).To(BeNil())

			t.Log("waiting for successful upgrade after coming out of alarm")
			err = wait.For(upgradeToFinish(ctx, t, cfg))
			g.Expect(err).To(BeNil())

			return ctx
		}).Feature()

	testenv.Test(t, f1)
}

// triggerUpgrade adds a unique label to a statefulset under test so that pods being upgrading
func triggerUpgrade(ctx context.Context, t *testing.T, cfg *envconf.Config) error {
	t.Log("updating statefulset pod template")
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var ss appsv1.StatefulSet
		if err := cfg.Client().Resources().Get(ctx, statefulSetNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &ss); err != nil {
			return err
		}
		ss.Spec.Template.Labels["test-version"] = envconf.RandomName("v", 32)
		return cfg.Client().Resources().Update(ctx, &ss)
	})
}

func disruptionsAreBreaching(ctx context.Context, t *testing.T, cfg *envconf.Config) (bool, error) {
	var pods corev1.PodList
	if err := cfg.Client().Resources(e2e.NamespaceFromContext(ctx)).List(ctx, &pods); err != nil {
		return false, fmt.Errorf("listing pods: %v", err)
	}
	var ss appsv1.StatefulSet
	if err := cfg.Client().Resources().Get(ctx, statefulSetNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &ss); err != nil {
		return false, fmt.Errorf("getting statefulset: %v", err)
	}
	var zau operatorv1.ZoneAwareUpdate
	if err := cfg.Client().Resources().Get(ctx, zauNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &zau); err != nil {
		return false, fmt.Errorf("getting zau: %v", err)
	}

	// check multiple zones are not disrupted
	var zones []string
	seen := make(map[string]bool)
	for _, pod := range pods.Items {
		zone, found := e2e.PodZoneFromContext(ctx, pod.Name)
		if !found {
			return false, fmt.Errorf("pod %s zone not found", pod.Name)
		}

		if !utils.IsPodReady(&pod) && !seen[zone] {
			seen[zone] = true
			zones = append(zones, zone)
		}
	}
	t.Logf("zones disrupted: %v", zones)
	if len(zones) > 1 {
		return true, nil
	}

	// check disruptions have not breached zau maximum
	var numDisrupted int
	for _, pod := range pods.Items {
		if !utils.IsPodReady(&pod) {
			numDisrupted++
		}
	}
	t.Logf("%d pods currently disrupted", numDisrupted)
	maxUnavailable, err := intstr.GetScaledValueFromIntOrPercent(zau.Spec.MaxUnavailable, int(*ss.Spec.Replicas), true)
	if err != nil {
		return false, fmt.Errorf("interpreting zau max unavailable: %v", err)
	}
	return numDisrupted > maxUnavailable, nil
}

// percentOfReplicasUpgraded returns the percentage of replicas in a statefulset that are deployed using the updated revision
func percentOfReplicasUpgraded(ctx context.Context, cfg *envconf.Config) (int, error) {
	var ss appsv1.StatefulSet
	if err := cfg.Client().Resources().Get(ctx, statefulSetNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &ss); err != nil {
		return 0, fmt.Errorf("getting statefulset: %v", err)
	}

	return int(ss.Status.UpdatedReplicas * 100 / ss.Status.Replicas), nil
}

// getDeployedImages returns a list of the unique container images being run by statefulset pods
func getDeployedImages(ctx context.Context, cfg *envconf.Config) ([]string, error) {
	var pods corev1.PodList
	if err := cfg.Client().Resources(e2e.NamespaceFromContext(ctx)).List(ctx, &pods); err != nil {
		return nil, err
	}

	var images []string
	seen := make(map[string]bool)
	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			if !seen[container.Image] {
				seen[container.Image] = true
				images = append(images, container.Image)
			}
		}
	}

	return images, nil
}
