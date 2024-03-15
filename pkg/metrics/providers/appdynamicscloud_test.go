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
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

var (
	clientID  = "secretID"
	clientKey = "secretKey"

	secrets = map[string][]byte{
		"appdcloud_client_secret_id":  []byte(clientID),
		"appdcloud_client_secret_key": []byte(clientKey),
	}
)

func TestNewAppDynamicsCloudProvider(t *testing.T) {
	appdcloud, err := NewAppDynamicsCloudProvider(flaggerv1.MetricTemplateProvider{
		Address: "https://lab1.observe.appdynamics.com"}, secrets)
	require.NoError(t, err)
	assert.Equal(t, "https://lab1.observe.appdynamics.com/monitoring/v1/query/execute", appdcloud.metricsQueryEndpoint)
	assert.Equal(t, clientID, appdcloud.clientSecretID)
	assert.Equal(t, clientKey, appdcloud.clientSecretKey)
}

func TestAppDynamicsCloudProvider_RunQuery(t *testing.T) {
	goodResponse := `
[{
	"type" : "model",
	"model" : {
		"name" : "m:main",
		"fields" : [ {
		"alias" : "cpm",
		"type" : "complex",
		"hints" : {
			"kind" : "metric",
			"type" : "apm:response_time"
		},
		"form" : "reference",
		"model" : {
			"name" : "m:cpm",
			"fields" : [ {
			"alias" : "source",
			"type" : "string",
			"hints" : {
				"kind" : "metric",
				"field" : "source"
			}
			}, {
			"alias" : "metrics",
			"type" : "timeseries",
			"hints" : {
				"kind" : "metric",
				"type" : "apm:response_time"
			},
			"form" : "inline",
			"model" : {
				"name" : "m:metrics",
				"fields" : [ {
				"alias" : "timestamp",
				"type" : "timestamp",
				"hints" : {
					"kind" : "metric",
					"field" : "timestamp",
					"type" : "apm:response_time"
				}
				}, {
				"alias" : "value",
				"type" : "number",
				"hints" : {
					"kind" : "metric",
					"field" : "value",
					"type" : "apm:response_time"
				}
				} ]
			}
			} ]
		}
		} ]
	}
	},{
	"type" : "data",
	"model" : {
		"$jsonPath" : "$..[?(@.type == 'model')]..[?(@.name == 'm:main')]",
		"$model" : "m:main"
	},
	"metadata" : {
		"since" : "2023-02-02T16:53:36.983726752Z",
		"until" : "2023-02-02T17:03:36.983726752Z"
	},
	"dataset" : "d:main",
	"data" : [ [ {
		"$dataset" : "d:metrics-1",
		"$jsonPath" : "$..[?(@.type == 'data' && @.dataset == 'd:metrics-1')]"
	} ] ]
	},{
	"type" : "data",
	"model" : {
		"$jsonPath" : "$..[?(@.type == 'model')]..[?(@.name == 'm:cpm')]",
		"$model" : "m:cpm"
	},
	"metadata" : {
		"granularitySeconds" : 60
	},
	"dataset" : "d:metrics-1",
	"data" : [ [ "sys:derived", [ [ "2023-02-02T16:55Z", 334438.4 ], [ "2023-02-02T16:57Z", 425362.28571428574 ], [ "2023-02-02T16:59Z", 364288.0 ] ] ] ]
	}]`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(goodResponse))
	}))
	defer ts.Close()

	provider, err := NewAppDynamicsCloudProvider(flaggerv1.MetricTemplateProvider{
		Address: "https://lab1.observe.appdynamics.com"}, secrets)

	assert.NoError(t, err)
	// alter the metrics endpoint for testing
	provider.tenantAddress = ts.URL
	provider.metricsQueryEndpoint = provider.tenantAddress + metricsQueryPath
	provider.client = http.DefaultClient

	float, err := provider.RunQuery(`fake request`)
	assert.NoError(t, err)
	assert.Equal(t, 364288.0, float)
}

func TestAppDynamicsCloudProvider_IsOnline(t *testing.T) {
	// test if we have client secret id and secret key defined
	envID, idPresent := os.LookupEnv("APPD_CLOUD_CLIENT_ID")
	envKey, keyPresent := os.LookupEnv("APPD_CLOUD_CLIENT_SECRET")

	if !idPresent || !keyPresent {
		t.Log("test skipped since no credentials are set in env variables")
		return
	}

	secrets := map[string][]byte{
		"appdcloud_client_secret_id":  []byte(envID),
		"appdcloud_client_secret_key": []byte(envKey),
	}

	appdcloud, err := NewAppDynamicsCloudProvider(flaggerv1.MetricTemplateProvider{
		Address: "https://lab1.observe.appdynamics.com"}, secrets)
	require.NoError(t, err)

	_, err = appdcloud.IsOnline()
	require.NoError(t, err)

}
