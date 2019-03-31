package metrics

import (
	"testing"
)

func Test_IstioSuccessRateQueryRender(t *testing.T) {
	meta := struct {
		Name      string
		Namespace string
		Interval  string
	}{
		"podinfo",
		"default",
		"1m",
	}

	query, err := render(meta, istioSuccessRateQuery)
	if err != nil {
		t.Fatal(err)
	}

	expected := `sum(rate(istio_requests_total{reporter="destination",destination_workload_namespace="default",destination_workload=~"podinfo",response_code!~"5.*"}[1m])) / sum(rate(istio_requests_total{reporter="destination",destination_workload_namespace="default",destination_workload=~"podinfo"}[1m])) * 100`

	if query != expected {
		t.Errorf("\nGot %s \nWanted %s", query, expected)
	}
}

func Test_IstioRequestDurationQueryRender(t *testing.T) {
	meta := struct {
		Name      string
		Namespace string
		Interval  string
	}{
		"podinfo",
		"default",
		"1m",
	}

	query, err := render(meta, istioRequestDurationQuery)
	if err != nil {
		t.Fatal(err)
	}

	expected := `histogram_quantile(0.99, sum(rate(istio_request_duration_seconds_bucket{reporter="destination",destination_workload_namespace="default",destination_workload=~"podinfo"}[1m])) by (le))`

	if query != expected {
		t.Errorf("\nGot %s \nWanted %s", query, expected)
	}
}
