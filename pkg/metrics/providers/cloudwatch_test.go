package providers

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/costandusagereportservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

type cloudWatchClientMock struct {
	o   *cloudwatch.GetMetricDataOutput
	err error
}

func (c cloudWatchClientMock) GetMetricData(_ *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
	return c.o, c.err
}

func TestNewCloudWatchProvider(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		p, err := NewCloudWatchProvider(
			"5m",
			flaggerv1.MetricTemplateProvider{
				Region: costandusagereportservice.AWSRegionApEast1,
			})

		assert.NoError(t, err)
		assert.Equal(t, 5*60*time.Second*cloudWatchStartDeltaMultiplierOnMetricInterval, p.startDelta)
	})

	t.Run("ng", func(t *testing.T) {
		_, err := NewCloudWatchProvider("5m", flaggerv1.MetricTemplateProvider{})
		assert.Error(t, err, "error expected since region was not specified")
	})
}

func TestCloudWatchProvider_IsOnline(t *testing.T) {
	t.Run("forbidden", func(t *testing.T) {
		p := CloudWatchProvider{client: cloudWatchClientMock{
			o:   nil,
			err: awserr.NewRequestFailure(nil, http.StatusForbidden, "request-id"),
		}}

		actual, err := p.IsOnline()
		assert.Error(t, err)
		assert.False(t, actual)
	})

	t.Run("ok", func(t *testing.T) {
		// no error
		p := CloudWatchProvider{client: cloudWatchClientMock{}}
		actual, err := p.IsOnline()
		assert.NoError(t, err)
		assert.True(t, actual)

		// with error but bad request
		p = CloudWatchProvider{client: cloudWatchClientMock{
			err: awserr.NewRequestFailure(nil, http.StatusBadRequest, "request-id"),
		}}
		assert.NoError(t, err)
		assert.True(t, actual)
	})
}

func TestCloudWatchProvider_RunQuery(t *testing.T) {
	// ref: https://aws.amazon.com/premiumsupport/knowledge-center/cloudwatch-getmetricdata-api/
	query := `
[
    {
        "Id": "e1",
        "Expression": "m1 / m2",
        "Label": "ErrorRate"
    },
    {
        "Id": "m1",
        "MetricStat": {
            "Metric": {
                "Namespace": "MyApplication",
                "MetricName": "Errors",
                "Dimensions": [
                    {
                        "Name": "FunctionName",
                        "Value": "MyFunc"
                    }
                ]
            },
            "Period": 300,
            "Stat": "Sum",
            "Unit": "Count"
        },
        "ReturnData": false
    },
    {
        "Id": "m2",
        "MetricStat": {
            "Metric": {
                "Namespace": "MyApplication",
                "MetricName": "Invocations",
                "Dimensions": [
                    {
                        "Name": "FunctionName",
                        "Value": "MyFunc"
                    }
                ]
            },
            "Period": 300,
            "Stat": "Sum",
            "Unit": "Count"
        },
        "ReturnData": false
    }
]`

	t.Run("ok", func(t *testing.T) {
		var exp float64 = 100
		p := CloudWatchProvider{client: cloudWatchClientMock{
			o: &cloudwatch.GetMetricDataOutput{
				MetricDataResults: []*cloudwatch.MetricDataResult{
					{Values: []*float64{aws.Float64(exp)}},
				},
			},
		}}

		actual, err := p.RunQuery(query)
		assert.NoError(t, err)
		assert.Equal(t, exp, actual)
	})

	t.Run("no values", func(t *testing.T) {
		p := CloudWatchProvider{client: cloudWatchClientMock{
			o: &cloudwatch.GetMetricDataOutput{
				MetricDataResults: []*cloudwatch.MetricDataResult{
					{Values: []*float64{}},
				},
			},
		}}

		_, err := p.RunQuery(query)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrNoValuesFound))

		p = CloudWatchProvider{client: cloudWatchClientMock{
			o: &cloudwatch.GetMetricDataOutput{}}}

		_, err = p.RunQuery(query)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})
}
