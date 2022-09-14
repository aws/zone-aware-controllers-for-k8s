package utils

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
)

type mockCloudWatchAPI func(ctx context.Context, params *cloudwatch.DescribeAlarmsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error)

func (m mockCloudWatchAPI) DescribeAlarms(ctx context.Context, params *cloudwatch.DescribeAlarmsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error) {
	return m(ctx, params, optFns...)
}

func TestCloudWatchAlarmStatusProvider(t *testing.T) {
	okAlarm := types.CompositeAlarm{
		StateValue: types.StateValueOk,
	}
	tests := []struct {
		name        string
		output      *cloudwatch.DescribeAlarmsOutput
		expectError bool
	}{
		{
			name: "return ok state",
			output: &cloudwatch.DescribeAlarmsOutput{
				CompositeAlarms: []types.CompositeAlarm{okAlarm},
			},
			expectError: false,
		},
		{
			name: "return no alarm",
			output: &cloudwatch.DescribeAlarmsOutput{
				CompositeAlarms: []types.CompositeAlarm{},
			},
			expectError: true,
		},
		{
			name: "return multiple alarms",
			output: &cloudwatch.DescribeAlarmsOutput{
				CompositeAlarms: []types.CompositeAlarm{okAlarm, okAlarm},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := CloudWatchAlarmStateProvider{
				Client: mockCloudWatchAPI(func(ctx context.Context, params *cloudwatch.DescribeAlarmsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error) {
					return tt.output, nil
				}),
			}
			state, err := provider.AlarmState(context.TODO(), "anyAlarmName")
			if tt.expectError {
				assert.NotNil(t, err)
				assert.Equal(t, state, types.StateValue(""))
			} else {
				assert.Nil(t, err)
				assert.Equal(t, state, types.StateValueOk)
			}
		})
	}
}
