package providers

import (
	"context"
	"fmt"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

type StackDriverProvider struct {
	client  *monitoring.QueryClient
	project string
}

// NewStackDriverProvider takes a provider spec and credential map and
// returns a StackDriverProvider ready to execute queries against the
// Cloud Monitoring API
func NewStackDriverProvider(provider flaggerv1.MetricTemplateProvider,
	credentials map[string][]byte,
) (*StackDriverProvider, error) {
	stackd := &StackDriverProvider{}
	var saKey []byte
	if provider.SecretRef != nil {
		if project, ok := credentials["project"]; ok {
			stackd.project = fmt.Sprintf("projects/%s", string(project))
		} else {
			return nil, fmt.Errorf("%s credentials does not contain a project id", provider.Type)
		}

		if cred, ok := credentials["serviceAccountKey"]; ok {
			saKey = cred
		}
	}

	var client *monitoring.QueryClient
	var err error
	ctx := context.Background()

	if saKey != nil {
		client, err = monitoring.NewQueryClient(ctx, option.WithCredentialsJSON(saKey))
	} else {
		client, err = monitoring.NewQueryClient(ctx)
	}

	if err != nil {
		return nil, err
	}

	stackd.client = client
	return stackd, nil
}

// RunQuery executes Monitoring Query Language(MQL) queries against the
// Cloud Monitoring API
func (s *StackDriverProvider) RunQuery(query string) (float64, error) {
	ctx := context.Background()
	req := &monitoringpb.QueryTimeSeriesRequest{
		Name:  s.project,
		Query: query,
	}

	it := s.client.QueryTimeSeries(ctx, req)

	resp, err := it.Next()
	if err == iterator.Done {
		return 0, fmt.Errorf("invalid response: %s: %w", resp, ErrNoValuesFound)
	}

	if err != nil {
		if s, ok := status.FromError(err); ok {
			errStr := s.Message()
			for _, d := range s.Proto().Details {
				errStr = errStr + " Error Detail: " + d.String()
			}

			return 0, fmt.Errorf("error requesting stackdriver: %s", err)
		}
		return 0, fmt.Errorf("error requesting stackdriver: %s", err)
	}

	pointData := resp.PointData
	if len(pointData) < 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", resp.String(), ErrNoValuesFound)
	}

	values := resp.PointData[0].Values
	if len(values) < 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", resp.String(), ErrNoValuesFound)
	}

	return values[0].GetDoubleValue(), nil
}

// IsOnline calls QueryTimeSeries method with the empty query
// and returns an error if the returned status code is NOT grpc.InvalidArgument.
// For example, if the flagger does not the authorization scope `https://www.googleapis.com/auth/monitoring.read`,
// the returned status code would be grpc.PermissionDenied
func (s *StackDriverProvider) IsOnline() (bool, error) {
	ctx := context.Background()
	req := &monitoringpb.QueryTimeSeriesRequest{
		Name:  s.project,
		Query: "",
	}

	it := s.client.QueryTimeSeries(ctx, req)

	_, err := it.Next()
	if err == nil {
		return true, nil
	}

	stat, ok := status.FromError(err)
	if !ok {
		return false, fmt.Errorf("unexpected error: %s", err)
	}

	if stat.Code() != codes.InvalidArgument {
		return false, fmt.Errorf("unexpected status code: %s", stat.Code().String())
	}

	return true, nil
}
