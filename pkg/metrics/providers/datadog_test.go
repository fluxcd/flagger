package providers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	require.NoError(t, err)

	dp, err := NewDatadogProvider("100s", flaggerv1.MetricTemplateProvider{}, cs)
	require.NoError(t, err)
	assert.Equal(t, "https://api.datadoghq.com/api/v1/validate", dp.apiKeyValidationEndpoint)
	assert.Equal(t, "https://api.datadoghq.com/api/v1/query", dp.metricsQueryEndpoint)
	assert.Equal(t, int64(md.Seconds()*datadogFromDeltaMultiplierOnMetricInterval), dp.fromDelta)
	assert.Equal(t, appKey, dp.applicationKey)
	assert.Equal(t, apiKey, dp.apiKey)
}

func TestDatadogProvider_RunQuery(t *testing.T) {
	appKey := "app-key"
	apiKey := "api-key"
	t.Run("ok", func(t *testing.T) {
		expected := 1.11111
		eq := `avg:system.cpu.user{*}by{host}`
		now := time.Now().Unix()
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			aq := r.URL.Query().Get("query")
			assert.Equal(t, eq, aq)
			assert.Equal(t, appKey, r.Header.Get(datadogApplicationKeyHeaderKey))
			assert.Equal(t, apiKey, r.Header.Get(datadogAPIKeyHeaderKey))

			from, err := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
			if assert.NoError(t, err) {
				assert.Less(t, from, now)
			}

			to, err := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
			if assert.NoError(t, err) {
				assert.GreaterOrEqual(t, to, now)
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
		require.NoError(t, err)

		f, err := dp.RunQuery(eq)
		require.NoError(t, err)
		assert.Equal(t, expected, f)
	})

	t.Run("no values", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json := fmt.Sprintf(`{"series": [{"pointlist": []}]}`)
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
		require.NoError(t, err)
		_, err = dp.RunQuery("")
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})
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
				assert.Equal(t, appKey, r.Header.Get(datadogApplicationKeyHeaderKey))
				assert.Equal(t, apiKey, r.Header.Get(datadogAPIKeyHeaderKey))
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
			require.NoError(t, err)

			_, err = dp.IsOnline()
			if c.errExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
