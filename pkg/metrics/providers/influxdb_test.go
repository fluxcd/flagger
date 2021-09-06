/*
Copyright 2021 The Flux authors

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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func TestNewInfluxdbProvider(t *testing.T) {
	t.Run("error when org isn't specified", func(t *testing.T) {
		_, err := NewInfluxdbProvider(flaggerv1.MetricTemplateProvider{
			Type: "influxdb",
			SecretRef: &corev1.LocalObjectReference{
				Name: "test-secret",
			},
		}, map[string][]byte{})

		assert.Error(t, err, "error expected since org is not given")
	})

	t.Run("error when token isn't specified", func(t *testing.T) {
		_, err := NewInfluxdbProvider(flaggerv1.MetricTemplateProvider{
			Type: "influxdb",
			SecretRef: &corev1.LocalObjectReference{
				Name: "test-secret",
			},
		}, map[string][]byte{
			"org": []byte("test-org"),
		})

		assert.Error(t, err, "error expected since token is not given")
	})

	t.Run("ok", func(t *testing.T) {
		_, err := NewInfluxdbProvider(flaggerv1.MetricTemplateProvider{
			Type:    "influxdb",
			Address: "http://localhost/",
			SecretRef: &corev1.LocalObjectReference{
				Name: "test-secret",
			},
		}, map[string][]byte{
			"org":   []byte("test-org"),
			"token": []byte("x"),
		})

		assert.NoError(t, err)
	})
}

func TestInfluxdbProvider_IsOnline(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		client := influxdb2.NewClient(ts.URL, "x")
		provider := InfluxdbProvider{
			client: client,
			org:    "fake-org",
		}
		isOnline, err := provider.IsOnline()
		assert.NoError(t, err)
		assert.True(t, isOnline)
	})

	t.Run("forbidden", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer ts.Close()

		client := influxdb2.NewClient(ts.URL, "x")
		provider := InfluxdbProvider{
			client: client,
			org:    "fake-org",
		}
		isOnline, err := provider.IsOnline()
		assert.Error(t, err)
		assert.False(t, isOnline)
	})
}

func TestInfluxdbProvider_RunQuery(t *testing.T) {

	csvTable := `#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,double,string,string,string,string
#group,false,false,true,true,false,false,true,true,true,true
#default,_result,,,,,,,,,
,result,table,_start,_stop,_time,_value,_field,_measurement,a,b
,,0,2020-02-17T22:19:49.747562847Z,2020-02-18T22:19:49.747562847Z,2020-02-18T10:34:08.135814545Z,1.4,f,test,1,adsfasdf
,,0,2020-02-17T22:19:49.747562847Z,2020-02-18T22:19:49.747562847Z,2020-02-18T22:08:44.850214724Z,6.6,f,test,1,adsfasdf
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.URL.Path)
		w.Write([]byte(csvTable))
	}))
	defer ts.Close()

	client := influxdb2.NewClient(ts.URL, "x")
	provider := InfluxdbProvider{
		client: client,
		org:    "fake-org",
	}
	float, err := provider.RunQuery(`from(bucket: "default")  |> range(start: -2h)`)

	assert.NoError(t, err)
	assert.Equal(t, float, 1.4)
}
