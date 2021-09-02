package providers

import (
	"context"
	"log"
	"net"
	"os"
	"testing"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

// clientOpt is the option tests should use to connect to the test server.
// It is initialized by TestMain.
var clientOpt option.ClientOption

var mockQueryPolicy mockQueryPolicyServer

type mockQueryPolicyServer struct {
	monitoringpb.QueryServiceServer

	req []proto.Message

	err error

	resps []proto.Message
}

type stackdriverClientMock struct {
	iter *monitoring.TimeSeriesDataIterator
}

func (m *mockQueryPolicyServer) QueryTimeSeries(ctx context.Context, req *monitoringpb.QueryTimeSeriesRequest) (*monitoringpb.QueryTimeSeriesResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.resps[0].(*monitoringpb.QueryTimeSeriesResponse), nil
}

func (s *stackdriverClientMock) QueryTimeSeries(ctx context.Context, req *monitoringpb.QueryTimeSeriesRequest, opts ...gax.CallOption) *monitoring.TimeSeriesDataIterator {
	return s.iter
}

func TestMain(m *testing.M) {
	serv := grpc.NewServer()
	monitoringpb.RegisterQueryServiceServer(serv, &mockQueryPolicy)
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatal(err)
	}
	go serv.Serve(lis)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	clientOpt = option.WithGRPCConn(conn)
	os.Exit(m.Run())
}

func TestNewStackDriverProvider(t *testing.T) {
	t.Run("error when project id isn't provided", func(t *testing.T) {
		_, err := NewStackDriverProvider(flaggerv1.MetricTemplateProvider{
			SecretRef: &corev1.LocalObjectReference{
				Name: "test-secret",
			},
		}, map[string][]byte{})

		assert.Error(t, err, "error expected since project id is not given")
	})
}

func TestStackDriverProvider_IsOnline(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		mockQueryPolicy.err = status.Error(codes.InvalidArgument, "invalid arg")
		c, err := monitoring.NewQueryClient(context.Background(), clientOpt)
		if err != nil {
			t.Fatal(err)
		}
		p := StackDriverProvider{client: c}
		actual, err := p.IsOnline()
		assert.NoError(t, err)
		assert.True(t, actual)
	})

	t.Run("forbidden", func(t *testing.T) {
		mockQueryPolicy.err = status.Error(codes.PermissionDenied, "insufficient permissions")
		c, err := monitoring.NewQueryClient(context.Background(), clientOpt)
		if err != nil {
			t.Fatal(err)
		}
		p := StackDriverProvider{client: c}
		actual, err := p.IsOnline()
		assert.Error(t, err)
		assert.False(t, actual)
	})
}

func TestStackDriverProvider_RunQuery(t *testing.T) {
	query := `
    fetch k8s_container
    | metric 'istio.io/service/server/response_latencies'
    | filter
        (metric.destination_service_name == '{{ service }}-canary'
        && metric.destination_service_namespace == '{{ namespace }}')
    | align delta(1m)
    | every 1m
    | group_by [],
        [value_response_latencies_percentile:
          percentile(value.response_latencies, 99)]
`

	t.Run("ok", func(t *testing.T) {
		var exp float64 = 100
		expectedResponse := &monitoringpb.QueryTimeSeriesResponse{
			NextPageToken: "",
			TimeSeriesData: []*monitoringpb.TimeSeriesData{
				{
					PointData: []*monitoringpb.TimeSeriesData_PointData{
						{
							Values: []*monitoringpb.TypedValue{
								{
									Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: exp},
								},
							},
						},
					},
				},
			},
		}

		mockQueryPolicy.resps = append(mockQueryPolicy.resps[:0], expectedResponse)
		mockQueryPolicy.err = nil

		c, err := monitoring.NewQueryClient(context.Background(), clientOpt)
		if err != nil {
			t.Fatal(err)
		}
		p := StackDriverProvider{client: c}
		actual, err := p.RunQuery(query)
		assert.NoError(t, err)
		assert.Equal(t, actual, exp)
	})

	t.Run("no values", func(t *testing.T) {
		expectedResps := &monitoringpb.QueryTimeSeriesResponse{
			NextPageToken:  "",
			TimeSeriesData: []*monitoringpb.TimeSeriesData{},
		}

		mockQueryPolicy.resps = append(mockQueryPolicy.resps[:0], expectedResps)
		mockQueryPolicy.err = nil

		c, err := monitoring.NewQueryClient(context.Background(), clientOpt)
		if err != nil {
			t.Fatal(err)
		}
		p := StackDriverProvider{client: c}
		_, err = p.RunQuery(query)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrNoValuesFound)
	})
}
