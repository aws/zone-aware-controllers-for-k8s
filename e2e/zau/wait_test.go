//go:build integration

package zau

import (
	"context"
	"fmt"
	"testing"

	operatorv1 "github.com/aws/zone-aware-controllers-for-k8s/api/v1"
	"github.com/aws/zone-aware-controllers-for-k8s/e2e"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// upgradeToFinish can be used with wait.For to poll until all pods have been updated to
func upgradeToFinish(ctx context.Context, t *testing.T, cfg *envconf.Config) func() (bool, error) {
	return func() (bool, error) {
		var pods corev1.PodList
		if err := cfg.Client().Resources(e2e.NamespaceFromContext(ctx)).List(ctx, &pods); err != nil {
			return false, fmt.Errorf("listing pods: %v", err)
		}
		p, err := percentOfReplicasUpgraded(ctx, cfg)
		if err != nil {
			return false, fmt.Errorf("computing percent of replicas upgraded: %v", err)
		}
		return p == 100, nil
	}
}

// zauToDeleteReplicas can be used with wait.For to poll until the zau has started deleting StatefulSet replicas
func zauToDeleteReplicas(ctx context.Context, t *testing.T, cfg *envconf.Config) func() (bool, error) {
	return func() (bool, error) {
		t.Log("waiting for zau to start deleting replicas")
		var zau operatorv1.ZoneAwareUpdate
		if err := cfg.Client().Resources().Get(ctx, zauNameFromContext(ctx), e2e.NamespaceFromContext(ctx), &zau); err != nil {
			return false, err
		}
		return zau.Status.DeletedReplicas > 0, nil
	}
}

// compositeAlarmState can be used with wait.For to poll until a composite alarm with the given name enters the given state
func compositeAlarmState(ctx context.Context, _ *testing.T, _ *envconf.Config, alarmName string, state types.StateValue) func() (bool, error) {
	cw := cloudwatch.NewFromConfig(e2e.AWSConfigFromContext(ctx))
	return func() (bool, error) {
		desc, err := cw.DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{
			AlarmNames: []string{alarmName},
			AlarmTypes: []types.AlarmType{types.AlarmTypeCompositeAlarm},
		})
		if err != nil {
			return false, fmt.Errorf("describing cloudwatch alarms: %v", err)
		}
		if len(desc.CompositeAlarms) != 1 {
			return false, fmt.Errorf("expected 1 alarm but got %d", len(desc.CompositeAlarms))
		}

		return desc.CompositeAlarms[0].StateValue == state, nil
	}
}
