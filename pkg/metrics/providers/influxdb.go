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
	"context"
	"fmt"
	"net/url"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

type InfluxdbProvider struct {
	client influxdb2.Client
	org    string
}

func NewInfluxdbProvider(provider flaggerv1.MetricTemplateProvider,
	credentials map[string][]byte) (*InfluxdbProvider, error) {
	influxURL, err := url.Parse(provider.Address)
	var token string
	var influxProvider InfluxdbProvider

	if provider.Address == "" || err != nil {
		return nil, fmt.Errorf("%s address %s is not a valid URL", provider.Type, provider.Address)
	}

	if provider.SecretRef != nil {
		if authToken, ok := credentials["token"]; ok {
			token = string(authToken)
		} else {
			return nil, fmt.Errorf("%s credentials does not contain an authentication token", provider.Type)
		}

		if org, ok := credentials["org"]; ok {
			influxProvider.org = string(org)
		} else {
			return nil, fmt.Errorf("%s credentials does not contain an organisation", provider.Type)
		}
	}

	client := influxdb2.NewClient(influxURL.String(), token)
	influxProvider.client = client

	return &influxProvider, nil
}

func (i *InfluxdbProvider) RunQuery(query string) (float64, error) {
	queryAPI := i.client.QueryAPI(i.org)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := queryAPI.Query(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("error accessing influxdb query api: %s", err)
	}
	for result.Next() {
		if result.Err() != nil {
			return 0, fmt.Errorf("query error: %s", err)
		}
		if result.Record().Value() == nil {
			return 0, fmt.Errorf("invalid response: %s: %w", result.Record().String(), ErrNoValuesFound)
		}

		if float, ok := result.Record().Value().(float64); ok {
			return float, nil
		}

		return 0, fmt.Errorf("invalid response: %s", result.Record().String())
	}

	return 0, nil
}

// IsOnline runs a simple query against the default bucket.
func (i *InfluxdbProvider) IsOnline() (bool, error) {
	queryAPI := i.client.QueryAPI(i.org)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := queryAPI.Query(ctx, `from(bucket: "default") |> range(start: -2h)`)
	if err != nil {
		return false, fmt.Errorf("error accessing influxdb query api: %s", err)
	}
	for result.Next() {
		if result.Err() != nil {
			return false, fmt.Errorf("query error: %s", err)
		}
	}

	return true, nil
}
