package utils

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type AlarmStateProvider interface {
	AlarmState(ctx context.Context, alarmName string) (types.StateValue, error)
}

type CloudWatchAPI interface {
	DescribeAlarms(ctx context.Context, params *cloudwatch.DescribeAlarmsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error)
}

type CloudWatchAlarmStateProvider struct {
	Client CloudWatchAPI
}

func (p *CloudWatchAlarmStateProvider) AlarmState(ctx context.Context, alarmName string) (types.StateValue, error) {
	output, err := p.Client.DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{
		AlarmNames: []string{alarmName},
		AlarmTypes: []types.AlarmType{types.AlarmTypeCompositeAlarm},
	})
	if err != nil {
		return "", err
	}

	if len(output.CompositeAlarms) == 0 {
		return "", fmt.Errorf("alarm not found: %s", alarmName)
	}
	if len(output.CompositeAlarms) > 1 {
		return "", fmt.Errorf("multiple alarms found: %v", output.CompositeAlarms)
	}

	alarm := output.CompositeAlarms[0]
	return alarm.StateValue, nil
}
