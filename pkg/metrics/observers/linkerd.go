package observers

import (
	"fmt"
	"time"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/metrics/providers"
)

var linkerdQueries = map[string]string{
	"request-success-rate": `
	sum(
		rate(
			response_total{
				namespace="{{ namespace }}",
				deployment=~"{{ target }}",
				classification!="failure",
				direction="inbound"
			}[{{ interval }}]
		)
	) 
	/ 
	sum(
		rate(
			response_total{
				namespace="{{ namespace }}",
				deployment=~"{{ target }}",
				direction="inbound"
			}[{{ interval }}]
		)
	) 
	* 100`,
	"request-duration": `
	histogram_quantile(
		0.99,
		sum(
			rate(
				response_latency_ms_bucket{
					namespace="{{ namespace }}",
					deployment=~"{{ target }}",
					direction="inbound"
				}[{{ interval }}]
			)
		) by (le)
	)`,
}

type LinkerdObserver struct {
	client providers.Interface
}

func (ob *LinkerdObserver) GetRequestSuccessRate(model flaggerv1.MetricTemplateModel) (float64, error) {
	query, err := RenderQuery(linkerdQueries["request-success-rate"], model)
	if err != nil {
		return 0, fmt.Errorf("rendering query failed: %w", err)
	}

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, fmt.Errorf("running query failed: %w", err)
	}

	return value, nil
}

func (ob *LinkerdObserver) GetRequestDuration(model flaggerv1.MetricTemplateModel) (time.Duration, error) {
	query, err := RenderQuery(linkerdQueries["request-duration"], model)
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
