package providers

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudwatch"

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
	p, err := NewCloudWatchProvider(
		"5m",
		flaggerv1.MetricTemplateProvider{
			Address: "monitoring.ap-northeast-1.amazonaws.com",
		})

	if err != nil {
		t.Fatal(err)
	}

	if exp := 5 * 60 * time.Second * cloudWatchStartDeltaMultiplierOnMetricInterval; p.startDelta != exp {
		t.Fatalf("expected %d but got %d", exp, p.startDelta)
	}
}

func TestCloudWatchProvider_IsOnline(t *testing.T) {
	t.Run("forbidden", func(t *testing.T) {
		p := CloudWatchProvider{client: cloudWatchClientMock{
			o:   nil,
			err: awserr.NewRequestFailure(nil, http.StatusForbidden, "request-id"),
		}}

		actual, err := p.IsOnline()
		if err == nil {
			t.Error("error expected")
		}
		if actual {
			t.Error("false expected")
		}
	})

	t.Run("ok", func(t *testing.T) {
		// no error
		p := CloudWatchProvider{client: cloudWatchClientMock{}}
		actual, err := p.IsOnline()
		if err != nil {
			t.Errorf("no error expected but got %v", err)
		}
		if !actual {
			t.Error("true expected")
		}

		// with error but bad request
		p = CloudWatchProvider{client: cloudWatchClientMock{
			err: awserr.NewRequestFailure(nil, http.StatusBadRequest, "request-id"),
		}}
		if err != nil {
			t.Errorf("no error expected but got %v", err)
		}
		if !actual {
			t.Error("true expected")
		}
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
		if err != nil {
			t.Fatalf("no error expected but got %v", err)
		}

		if actual != exp {
			t.Errorf("expected %f but got %f", exp, actual)
		}
	})

	t.Run("no values", func(t *testing.T) {
		assert := func(err error) {
			if err == nil {
				t.Fatal("error expected")
			}
			if !strings.Contains(err.Error(), "no values") {
				t.Fatalf("no values expected in %v", err)
			}
		}

		p := CloudWatchProvider{client: cloudWatchClientMock{
			o: &cloudwatch.GetMetricDataOutput{
				MetricDataResults: []*cloudwatch.MetricDataResult{
					{Values: []*float64{}},
				},
			},
		}}

		_, err := p.RunQuery(query)
		assert(err)

		p = CloudWatchProvider{client: cloudWatchClientMock{
			o: &cloudwatch.GetMetricDataOutput{}}}

		_, err = p.RunQuery(query)
		assert(err)
	})
}
