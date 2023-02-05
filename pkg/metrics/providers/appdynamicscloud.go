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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"

	"golang.org/x/oauth2/clientcredentials"
)

// https://developer.cisco.com/docs/appdynamics/query-service/#!api-reference-appdynamics-cloud-query-service-api
const (
	clientSecretID  = "appdcloud_client_secret_id"
	clientSecretKey = "appdcloud_client_secret_key"

	metricsQueryPath     = "/monitoring/v1/query/execute"
	tenantLookupEndpoint = "https://observe-tenant-lookup-api.saas.appdynamics.com/tenants/lookup/"
)

type AppDynamicsCloudProvider struct {
	tenantID             string
	tenantAddress        string
	metricsQueryEndpoint string
	clientSecretID       string
	clientSecretKey      string

	timeout time.Duration
	client  *http.Client
}

// NewAppDynamicsCloudProvider takes a provider spec and the credentials map,
// and returns a AppDynamicsCloud client ready to execute queries against the API
func NewAppDynamicsCloudProvider(
	provider flaggerv1.MetricTemplateProvider,
	credentials map[string][]byte) (*AppDynamicsCloudProvider, error) {

	address := provider.Address
	if address == "" {
		return nil, fmt.Errorf("appdynamics cloud endpoint url address is not set")
	}

	tid, err := getTenantID(address)
	if tid == "" || err != nil {
		return nil, fmt.Errorf("failed to retrieve tenant id based on tenant URL address: %s", address)
	}

	appdCloudProvider := AppDynamicsCloudProvider{
		tenantID:             tid,
		tenantAddress:        address,
		metricsQueryEndpoint: address + metricsQueryPath,

		timeout: 5 * time.Second,
	}

	if b, ok := credentials[clientSecretID]; ok {
		appdCloudProvider.clientSecretID = string(b)
	} else {
		return nil, fmt.Errorf("appdynamics cloud credentials does not contain %s", clientSecretID)
	}

	if b, ok := credentials[clientSecretKey]; ok {
		appdCloudProvider.clientSecretKey = string(b)
	} else {
		return nil, fmt.Errorf("appdynamics cloud credentials does not contain %s", clientSecretKey)
	}

	return &appdCloudProvider, nil

}

// RunQuery executes the appdynamics cloud query against AppDynamicsCloudProvider
// metricsQueryEndpoint and returns the result as float64
func (p *AppDynamicsCloudProvider) RunQuery(query string) (float64, error) {
	if p.client == nil {
		if _, err := p.IsOnline(); err != nil {
			return 0, fmt.Errorf("failed to login to query endpoint: %w", err)
		}
	}

	jsonQuery, err := json.Marshal(map[string]string{
		"query": query,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to marshal query: %w", err)
	}

	// output to flagger container runtime log
	// fmt.Print("appdynamicscloud metric query:", string(jsonQuery))
	p.client.Timeout = p.timeout
	resp, err := p.client.Post(p.metricsQueryEndpoint, "application/json", bytes.NewBuffer(jsonQuery))
	if err != nil {
		return 0, fmt.Errorf("failed to get query response: %w", err)
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// we want to extract just a single float value from the result, hence no need to
	// un-marshalling the entire struct using Appdynamics Cloud Query API
	var anyValue []map[string]any
	if err := json.Unmarshal(body, &anyValue); err != nil {
		return 0, fmt.Errorf("failed to un-marshaling result: %s.\n error: %w", string(body), err)
	}
	if len(anyValue) < 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(body), ErrNoValuesFound)
	}
	// actual result (non-meta data) is in the data element of the last item
	// of the json array
	data := anyValue[len(anyValue)-1]["data"].([]any)
	if len(data) < 1 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(body), ErrNoValuesFound)
	}
	// nested in the data element of the first item
	data_data := data[0].([]any)
	if len(data_data) < 2 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(body), ErrNoValuesFound)
	}
	// metrics data is the second element, the first element is the
	// source, e.g. "sys:derived"
	metrics_data := data_data[1].([]any)
	if len(data_data) < 2 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(body), ErrNoValuesFound)
	}
	// get the last metrics from the array of metrics
	metric := metrics_data[len(metrics_data)-1].([]any)
	if len(data_data) < 2 {
		return 0, fmt.Errorf("invalid response: %s: %w", string(body), ErrNoValuesFound)
	}

	return metric[1].(float64), nil
}

// IsOnline calls the Appdynamics Cloud's metrics endpoint with client ID and
// secret and fills the authToken, returns an error if the endpoint fails
func (p *AppDynamicsCloudProvider) IsOnline() (bool, error) {
	// set up the struct according to clientcredentials package
	ccConfig := clientcredentials.Config{

		ClientID:     p.clientSecretID,
		ClientSecret: p.clientSecretKey,
		TokenURL:     p.tenantAddress + "/auth/" + p.tenantID + "/default/oauth2/token",
	}

	// check if we can get the token
	_, err := ccConfig.Token(context.Background())

	if err != nil {
		return false, fmt.Errorf("failed to authenticate : %w", err)
	}
	p.client = ccConfig.Client(context.Background())

	return true, nil
}

// getTenantID make a request to the lookup service and get the tenant id based
// on tenant url address. TenantID is used to get the auth token.
func getTenantID(address string) (string, error) {
	var reqURL string
	if u, err := url.Parse(address); err == nil {
		host, _, _ := net.SplitHostPort(u.Host)
		if host == "" {
			// there is no port specified in the address
			host = u.Host
		}
		reqURL = tenantLookupEndpoint + host
	} else {
		return "", fmt.Errorf("appdynamics cloud endpoint url address is misformed")
	}

	httpResp, err := http.Get(reqURL)
	if err != nil {
		return "", fmt.Errorf("unable to get tenant id, got error: %s", err)
	}
	if httpResp.StatusCode > 300 {
		return "", fmt.Errorf("error code returned, reqURL is %s and got error: %s", reqURL, httpResp.Status)
	}
	// Parse the response body to get the tenant id
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read api response body, got error: %s", err)
	}

	var jsondata map[string]string
	err = json.Unmarshal(body, &jsondata)
	if err != nil {
		return "", fmt.Errorf("unable to unmarshal api response body, got error: %s", body)
	}

	return jsondata["tenantId"], nil
}
