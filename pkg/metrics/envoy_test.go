package metrics

import (
	"testing"
)

func Test_EnvoySuccessRateQueryRender(t *testing.T) {
	meta := struct {
		Name      string
		Namespace string
		Interval  string
	}{
		"podinfo",
		"default",
		"1m",
	}

	query, err := render(meta, envoySuccessRateQuery)
	if err != nil {
		t.Fatal(err)
	}

	expected := `sum(rate(envoy_cluster_upstream_rq{kubernetes_namespace="default",kubernetes_pod_name=~"podinfo-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)",envoy_response_code!~"5.*"}[1m])) / sum(rate(envoy_cluster_upstream_rq{kubernetes_namespace="default",kubernetes_pod_name=~"podinfo-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)"}[1m])) * 100`

	if query != expected {
		t.Errorf("\nGot %s \nWanted %s", query, expected)
	}
}

func Test_EnvoyRequestDurationQueryRender(t *testing.T) {
	meta := struct {
		Name      string
		Namespace string
		Interval  string
	}{
		"podinfo",
		"default",
		"1m",
	}

	query, err := render(meta, envoyRequestDurationQuery)
	if err != nil {
		t.Fatal(err)
	}

	expected := `histogram_quantile(0.99, sum(rate(envoy_cluster_upstream_rq_time_bucket{kubernetes_namespace="default",kubernetes_pod_name=~"podinfo-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)"}[1m])) by (le))`

	if query != expected {
		t.Errorf("\nGot %s \nWanted %s", query, expected)
	}
}
