/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package providers

import (
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/signalfx/signalflow-client-go/signalflow"
	"github.com/signalfx/signalflow-client-go/signalflow/messages"
	"github.com/signalfx/signalfx-go/idtool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func TestNewSplunkProvider(t *testing.T) {
	token := "token"
	cs := map[string][]byte{
		signalFxTokenSecretKey: []byte(token),
	}

	mi := "100s"
	md, err := time.ParseDuration(mi)
	require.NoError(t, err)

	sp, err := NewSplunkProvider("100s", flaggerv1.MetricTemplateProvider{Address: "https://api.us1.signalfx.com"}, cs)
	require.NoError(t, err)
	assert.Equal(t, "wss://stream.us1.signalfx.com/v2/signalflow", sp.metricsQueryEndpoint)
	assert.Equal(t, "https://api.us1.signalfx.com/v2/metric?limit=1", sp.apiValidationEndpoint)
	assert.Equal(t, int64(md.Milliseconds()*signalFxFromDeltaMultiplierOnMetricInterval), sp.fromDelta)
	assert.Equal(t, token, sp.token)
}

func TestSplunkProvider_RunQuery(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		fakeBackend := signalflow.NewRunningFakeBackend()
		go func() {
			<-time.After(3 * time.Second)
			fakeBackend.Stop()
		}()

		tsids := []idtool.ID{idtool.ID(rand.Int63())}
		var expected float64 = float64(len(tsids))

		for i, tsid := range tsids {
			fakeBackend.AddTSIDMetadata(tsid, &messages.MetadataProperties{
				Metric: "service.request.count",
			})
			fakeBackend.SetTSIDFloatData(tsid, float64(i+1))
		}

		pg := `data('service.request.count', filter=filter('service.name', 'myservice')).sum().publish()`
		fakeBackend.AddProgramTSIDs(pg, tsids)

		parsedUrl, err := url.Parse(fakeBackend.URL())
		require.NoError(t, err)

		sp, err := NewSplunkProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: fmt.Sprintf("http://%s", parsedUrl.Host)},
			map[string][]byte{
				signalFxTokenSecretKey: []byte(fakeBackend.AccessToken),
			},
		)
		require.NoError(t, err)

		f, err := sp.RunQuery(pg)
		require.NoError(t, err)
		assert.Equal(t, expected, f)
	})

	t.Run("no values", func(t *testing.T) {
		fakeBackend := signalflow.NewRunningFakeBackend()
		go func() {
			<-time.After(3 * time.Second)
			fakeBackend.Stop()
		}()

		tsids := []idtool.ID{idtool.ID(rand.Int63()), idtool.ID(rand.Int63()), idtool.ID(rand.Int63())}
		for _, tsid := range tsids {
			fakeBackend.AddTSIDMetadata(tsid, &messages.MetadataProperties{
				Metric: "service.request.count",
			})
		}

		pg := `data('service.request.count', filter=filter('service.name', 'myservice')).sum().publish()`
		fakeBackend.AddProgramTSIDs(pg, tsids)

		parsedUrl, err := url.Parse(fakeBackend.URL())
		require.NoError(t, err)

		sp, err := NewSplunkProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: fmt.Sprintf("http://%s", parsedUrl.Host)},
			map[string][]byte{
				signalFxTokenSecretKey: []byte(fakeBackend.AccessToken),
			},
		)
		require.NoError(t, err)
		_, err = sp.RunQuery(pg)
		require.True(t, errors.Is(err, ErrNoValuesFound))
	})

	t.Run("multiple values", func(t *testing.T) {
		fakeBackend := signalflow.NewRunningFakeBackend()
		go func() {
			<-time.After(3 * time.Second)
			fakeBackend.Stop()
		}()

		tsids := []idtool.ID{idtool.ID(rand.Int63()), idtool.ID(rand.Int63()), idtool.ID(rand.Int63())}
		for i, tsid := range tsids {
			fakeBackend.AddTSIDMetadata(tsid, &messages.MetadataProperties{
				Metric: "service.request.count",
			})
			fakeBackend.SetTSIDFloatData(tsid, float64(i+1))
		}
		pg := `data('service.request.count', filter=filter('service.name', 'myservice')).sum().publish(); data('service.request.count', filter=filter('service.name', 'myservice2')).sum().publish()`
		fakeBackend.AddProgramTSIDs(pg, tsids)

		parsedUrl, err := url.Parse(fakeBackend.URL())
		require.NoError(t, err)

		sp, err := NewSplunkProvider("1m",
			flaggerv1.MetricTemplateProvider{Address: fmt.Sprintf("http://%s", parsedUrl.Host)},
			map[string][]byte{
				signalFxTokenSecretKey: []byte(fakeBackend.AccessToken),
			},
		)
		require.NoError(t, err)
		_, err = sp.RunQuery(pg)
		require.True(t, errors.Is(err, ErrMultipleValuesReturned))
	})
}

func TestSplunkProvider_IsOnline(t *testing.T) {
	for _, c := range []struct {
		code        int
		errExpected bool
	}{
		{code: http.StatusOK, errExpected: false},
		{code: http.StatusUnauthorized, errExpected: true},
	} {
		t.Run(fmt.Sprintf("%d", c.code), func(t *testing.T) {
			token := "token"
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, token, r.Header.Get(signalFxTokenHeaderKey))
				w.WriteHeader(c.code)
			}))
			defer ts.Close()

			sp, err := NewSplunkProvider("1m",
				flaggerv1.MetricTemplateProvider{Address: ts.URL},
				map[string][]byte{
					signalFxTokenSecretKey: []byte(token),
				},
			)
			require.NoError(t, err)

			_, err = sp.IsOnline()
			if c.errExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
