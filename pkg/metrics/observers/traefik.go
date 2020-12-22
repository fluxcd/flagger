package observers

import (
	"fmt"
	"time"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/metrics/providers"
)

var traefikQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			traefik_service_request_duration_seconds_bucket{
				service=~"{{ namespace }}-{{ target }}-canary-[0-9a-zA-Z-]+@kubernetescrd",
				code!~"5..",
				le="+Inf"
			}[{{ interval }}]
		)
	)
	/
	sum(
		rate(
			traefik_service_request_duration_seconds_bucket{
				service=~"{{ namespace }}-{{ target }}-canary-[0-9a-zA-Z-]+@kubernetescrd",
				le="+Inf"
			}[{{ interval }}]
		)
	) * 100`,
	"request-duration": `
	histogram_quantile(
		0.99,
		sum(
			rate(
				traefik_service_request_duration_seconds_bucket{
					service=~"{{ namespace }}-{{ target }}-canary-[0-9a-zA-Z-]+@kubernetescrd"
				}[{{ interval }}]
			)
		) by (le)
	)`,
}

type TraefikObserver struct {
	client providers.Interface
}

func (ob *TraefikObserver) GetRequestSuccessRate(model flaggerv1.MetricTemplateModel) (float64, error) {

	query, err := RenderQuery(traefikQueries["request-success-rate"], model)
	if err != nil {
		return 0, fmt.Errorf("rendering query failed: %w", err)
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, fmt.Errorf("running query failed: %w", err)
	}

	return value, nil
}

func (ob *TraefikObserver) GetRequestDuration(model flaggerv1.MetricTemplateModel) (time.Duration, error) {
	query, err := RenderQuery(traefikQueries["request-duration"], model)
	if err != nil {
		return 0, fmt.Errorf("rendering query failed: %w", err)
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, fmt.Errorf("running query failed: %w", err)
	}

	ms := time.Duration(int64(value)) * time.Millisecond
	return ms, nil
}
