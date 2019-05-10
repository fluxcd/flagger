package metrics

import (
	"testing"
)

func Test_NginxSuccessRateQueryRender(t *testing.T) {
	meta := struct {
		Name      string
		Namespace string
		Interval  string
	}{
		"podinfo",
		"nginx",
		"1m",
	}

	query, err := render(meta, nginxSuccessRateQuery)
	if err != nil {
		t.Fatal(err)
	}

	expected := `sum(rate(nginx_ingress_controller_requests{namespace="nginx",ingress="podinfo",status!~"5.*"}[1m])) / sum(rate(nginx_ingress_controller_requests{namespace="nginx",ingress="podinfo"}[1m])) * 100`

	if query != expected {
		t.Errorf("\nGot %s \nWanted %s", query, expected)
	}
}

func Test_NginxRequestDurationQueryRender(t *testing.T) {
	meta := struct {
		Name      string
		Namespace string
		Interval  string
	}{
		"podinfo",
		"nginx",
		"1m",
	}

	query, err := render(meta, nginxRequestDurationQuery)
	if err != nil {
		t.Fatal(err)
	}

	expected := `sum(rate(nginx_ingress_controller_ingress_upstream_latency_seconds_sum{namespace="nginx",ingress="podinfo"}[1m])) /sum(rate(nginx_ingress_controller_ingress_upstream_latency_seconds_count{namespace="nginx",ingress="podinfo"}[1m])) * 1000`

	if query != expected {
		t.Errorf("\nGot %s \nWanted %s", query, expected)
	}
}
