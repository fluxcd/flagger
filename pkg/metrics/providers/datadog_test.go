package providers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

func TestNewDatadogProvider(t *testing.T) {
	appKey := "app-key"
	apiKey := "api-key"
	cs := map[string][]byte{
		datadogApplicationKeySecretKey: []byte(appKey),
		datadogAPIKeySecretKey:         []byte(apiKey),
	}

	mi := "100s"
	md, err := time.ParseDuration(mi)
	if err != nil {
		t.Fatal(err)
	}

	dp, err := NewDatadogProvider("100s", flaggerv1.MetricTemplateProvider{}, cs)

	if err != nil {
		t.Fatal(err)
	}

	if exp := "https://api.datadoghq.com/api/v1/validate"; dp.apiKeyValidationEndpoint != exp {
		t.Fatalf("apiKeyValidationEndpoint expected %s but got %s", exp, dp.apiKeyValidationEndpoint)
	}

	if exp := "https://api.datadoghq.com/api/v1/query"; dp.metricsQueryEndpoint != exp {
		t.Fatalf("metricsQueryEndpoint expected %s but got %s", exp, dp.metricsQueryEndpoint)
	}

	if exp := int64(md.Seconds() * datadogFromDeltaMultiplierOnMetricInterval); dp.fromDelta != exp {
		t.Fatalf("fromDelta expected %d but got %d", exp, dp.fromDelta)
	}

	if dp.applicationKey != appKey {
		t.Fatalf("application key expected %s but got %s", appKey, dp.applicationKey)
	}

	if dp.apiKey != apiKey {
		t.Fatalf("api key expected %s but got %s", apiKey, dp.apiKey)
	}
}

func TestDatadogProvider_RunQuery(t *testing.T) {
	eq := `avg:system.cpu.user\{*}by{host}`
	appKey := "app-key"
	apiKey := "api-key"
	expected := 1.11111

	now := time.Now().Unix()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aq := r.URL.Query().Get("query")
		if aq != eq {
			t.Errorf("\nquery expected %s bug got %s", eq, aq)
		}

		if vs := r.Header.Get(datadogApplicationKeyHeaderKey); vs != appKey {
			t.Errorf("\n%s header expected %s but got %s", datadogApplicationKeyHeaderKey, appKey, vs)
		}
		if vs := r.Header.Get(datadogAPIKeyHeaderKey); vs != apiKey {
			t.Errorf("\n%s header expected %s but got %s", datadogAPIKeyHeaderKey, apiKey, vs)
		}

		rf := r.URL.Query().Get("from")
		if from, err := strconv.ParseInt(rf, 10, 64); err == nil && from >= now {
			t.Errorf("\nfrom %d should be less than %d", from, now)
		} else if err != nil {
			t.Errorf("\nfailed to parse from: %v", err)
		}

		rt := r.URL.Query().Get("to")
		if to, err := strconv.ParseInt(rt, 10, 64); err == nil && to < now {
			t.Errorf("\nto %d should be greater than or equals %d", to, now)
		} else if err != nil {
			t.Errorf("\nfailed to parse to: %v", err)
		}

		json := fmt.Sprintf(`{"series": [{"pointlist": [[1577232000000,29325.102158814265],[1577318400000,56294.46758591842],[1577404800000,%f]]}]}`, expected)
		w.Write([]byte(json))
	}))
	defer ts.Close()

	dp, err := NewDatadogProvider("1m",
		flaggerv1.MetricTemplateProvider{Address: ts.URL},
		map[string][]byte{
			datadogApplicationKeySecretKey: []byte(appKey),
			datadogAPIKeySecretKey:         []byte(apiKey),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	f, err := dp.RunQuery(eq)
	if err != nil {
		t.Fatal(err)
	}

	if f != expected {
		t.Fatalf("metric value expected %f but got %f", expected, f)
	}
}

func TestDatadogProvider_IsOnline(t *testing.T) {
	for _, c := range []struct {
		code        int
		errExpected bool
	}{
		{code: http.StatusOK, errExpected: false},
		{code: http.StatusUnauthorized, errExpected: true},
	} {
		t.Run(fmt.Sprintf("%d", c.code), func(t *testing.T) {
			appKey := "app-key"
			apiKey := "api-key"
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if vs := r.Header.Get(datadogApplicationKeyHeaderKey); vs != appKey {
					t.Errorf("\n%s header expected %s but got %s", datadogApplicationKeyHeaderKey, appKey, vs)
				}
				if vs := r.Header.Get(datadogAPIKeyHeaderKey); vs != apiKey {
					t.Errorf("\n%s header expected %s but got %s", datadogAPIKeyHeaderKey, apiKey, vs)
				}
				w.WriteHeader(c.code)
			}))
			defer ts.Close()

			dp, err := NewDatadogProvider("1m",
				flaggerv1.MetricTemplateProvider{Address: ts.URL},
				map[string][]byte{
					datadogApplicationKeySecretKey: []byte(appKey),
					datadogAPIKeySecretKey:         []byte(apiKey),
				},
			)
			if err != nil {
				t.Fatal(err)
			}

			_, err = dp.IsOnline()
			if c.errExpected && err == nil {
				t.Fatal("error expected but got no error")
			} else if !c.errExpected && err != nil {
				t.Fatalf("no error expected but got %v", err)
			}
		})
	}
}
