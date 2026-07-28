/*
Copyright 2025 The Flux authors

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
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

const azureMonitorTestAddress = "https://flagger-abc1.eastus.prometheus.monitor.azure.com"

type fakeAzureCredential struct {
	token  string
	scopes []string
	err    error
}

func (c *fakeAzureCredential) GetToken(_ context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	c.scopes = opts.Scopes
	if c.err != nil {
		return azcore.AccessToken{}, c.err
	}
	return azcore.AccessToken{Token: c.token, ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// azureWorkloadIdentityEnv sets the variables injected by the workload identity webhook
func azureWorkloadIdentityEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AZURE_FEDERATED_TOKEN_FILE", "/var/run/secrets/azure/tokens/azure-identity-token")
	t.Setenv("AZURE_CLIENT_ID", "pod-client-id")
	t.Setenv("AZURE_TENANT_ID", "pod-tenant-id")
}

func azureMonitorTestProvider(address string) flaggerv1.MetricTemplateProvider {
	return flaggerv1.MetricTemplateProvider{
		Type:      "azuremonitor",
		Address:   address,
		SecretRef: &corev1.LocalObjectReference{Name: "azuremonitor"},
	}
}

// azureUseCredential seeds the credential cache so the constructor mints its token from cred
func azureUseCredential(t *testing.T, cred azcore.TokenCredential) {
	t.Helper()
	azureWorkloadIdentityEnv(t)

	key := azureCredentialKey(nil, cloud.AzurePublic)

	azureCredentialCacheMu.Lock()
	azureCredentialCache[key] = cred
	azureCredentialCacheMu.Unlock()

	t.Cleanup(func() {
		azureCredentialCacheMu.Lock()
		delete(azureCredentialCache, key)
		azureCredentialCacheMu.Unlock()
	})
}

// azureMonitorTestClient points a provider at the test server, since only workspace addresses are accepted
func azureMonitorTestClient(t *testing.T, serverURL string, provider flaggerv1.MetricTemplateProvider,
	cred azcore.TokenCredential,
) *AzureMonitorProvider {
	t.Helper()
	azureUseCredential(t, cred)

	az, err := NewAzureMonitorProvider(provider, nil)
	require.NoError(t, err)

	u, err := url.Parse(serverURL)
	require.NoError(t, err)
	az.url = *u

	return az
}

func TestNewAzureMonitorProvider(t *testing.T) {
	azureWorkloadIdentityEnv(t)

	t.Run("ok", func(t *testing.T) {
		cred := &fakeAzureCredential{token: "token"}
		azureUseCredential(t, cred)

		az, err := NewAzureMonitorProvider(azureMonitorTestProvider(azureMonitorTestAddress), nil)
		require.NoError(t, err)

		assert.Equal(t, azureMonitorTestAddress, az.url.String())
		assert.Equal(t, "token", az.token)
		assert.Equal(t, []string{"https://prometheus.monitor.azure.com/.default"}, cred.scopes)
	})

	t.Run("no secret", func(t *testing.T) {
		azureUseCredential(t, &fakeAzureCredential{token: "token"})

		provider := azureMonitorTestProvider(azureMonitorTestAddress)
		provider.SecretRef = nil

		az, err := NewAzureMonitorProvider(provider, nil)
		require.NoError(t, err)
		assert.Equal(t, "token", az.token)
	})

	t.Run("token error", func(t *testing.T) {
		azureUseCredential(t, &fakeAzureCredential{err: assert.AnError})

		_, err := NewAzureMonitorProvider(azureMonitorTestProvider(azureMonitorTestAddress), nil)
		require.ErrorContains(t, err, "azuremonitor token request failed")
	})

	t.Run("invalid address", func(t *testing.T) {
		_, err := NewAzureMonitorProvider(azureMonitorTestProvider(""), nil)
		require.Error(t, err)
	})

	t.Run("address outside of azure monitor", func(t *testing.T) {
		_, err := NewAzureMonitorProvider(azureMonitorTestProvider("https://prometheus.example.com"), nil)
		require.EqualError(t, err,
			"azuremonitor address https://prometheus.example.com is not an Azure Monitor Workspace query endpoint, "+
				"expected a host ending in .prometheus.monitor.azure.us, .prometheus.monitor.azure.cn, .prometheus.monitor.azure.com")
	})

	t.Run("address without tls", func(t *testing.T) {
		address := "http://flagger-abc1.eastus.prometheus.monitor.azure.com"

		_, err := NewAzureMonitorProvider(azureMonitorTestProvider(address), nil)
		require.EqualError(t, err, "azuremonitor address "+address+" must use https")
	})

	t.Run("insecure skip verify", func(t *testing.T) {
		provider := azureMonitorTestProvider(azureMonitorTestAddress)
		provider.InsecureSkipVerify = true

		_, err := NewAzureMonitorProvider(provider, nil)
		require.EqualError(t, err, "azuremonitor provider does not support insecureSkipVerify")
	})
}

func TestAzureMonitorProvider_RunQuery(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		expected := `sum(envoy_cluster_upstream_rq)`
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v1/query", r.URL.Path)
			assert.Equal(t, expected, r.URL.Query()["query"][0])
			assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))

			json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"100"]}]}}`
			w.Write([]byte(json))
		}))
		defer ts.Close()

		cred := &fakeAzureCredential{token: "token"}
		az := azureMonitorTestClient(t, ts.URL, azureMonitorTestProvider(azureMonitorTestAddress), cred)

		val, err := az.RunQuery(expected)
		require.NoError(t, err)
		assert.Equal(t, float64(100), val)
		assert.Equal(t, []string{"https://prometheus.monitor.azure.com/.default"}, cred.scopes)
	})
}

func TestAzureMonitorProvider_IsOnline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))

		json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"1"]}]}}`
		w.Write([]byte(json))
	}))
	defer ts.Close()

	az := azureMonitorTestClient(t, ts.URL, azureMonitorTestProvider(azureMonitorTestAddress), &fakeAzureCredential{token: "token"})

	ok, err := az.IsOnline()
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestAzureMonitorProvider_Headers(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		assert.Equal(t, "bar", r.Header.Get("Foo"))

		json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"1"]}]}}`
		w.Write([]byte(json))
	}))
	defer ts.Close()

	provider := azureMonitorTestProvider(azureMonitorTestAddress)
	provider.Headers = http.Header{"Foo": []string{"bar"}}

	az := azureMonitorTestClient(t, ts.URL, provider, &fakeAzureCredential{token: "token"})

	for i := 0; i < 2; i++ {
		_, err := az.RunQuery("vector(1)")
		require.NoError(t, err)
	}

	assert.Empty(t, provider.Headers.Get("Authorization"),
		"the header map comes from the informer cache and must not be modified")
}

// TestAzureMonitorProvider_Redirect relies on the http client dropping the Authorization
// header when a redirect leaves the workspace host. The hosts are faked because the
// client compares host names and every test server listens on the loopback address.
func TestAzureMonitorProvider_Redirect(t *testing.T) {
	var received string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get("Authorization")

		json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"1"]}]}}`
		w.Write([]byte(json))
	}))
	defer target.Close()

	workspace := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		http.Redirect(w, r, "http://elsewhere.invalid/api/v1/query?query=vector(1)", http.StatusFound)
	}))
	defer workspace.Close()

	az := azureMonitorTestClient(t, "http://workspace.invalid", azureMonitorTestProvider(azureMonitorTestAddress),
		&fakeAzureCredential{token: "token"})

	hosts := map[string]string{
		"workspace.invalid:80": workspace.Listener.Addr().String(),
		"elsewhere.invalid:80": target.Listener.Addr().String(),
	}
	az.client = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if mapped, ok := hosts[addr]; ok {
				addr = mapped
			}
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}}

	val, err := az.RunQuery("vector(1)")
	require.NoError(t, err)
	assert.Equal(t, float64(1), val, "the redirect must have been followed")
	assert.Empty(t, received, "the token must not be sent to another host")
}

func TestNewAzureCredential(t *testing.T) {
	tests := []struct {
		name         string
		credentials  map[string][]byte
		expectedType any
		expectedErr  string
	}{
		{
			name: "service principal",
			credentials: map[string][]byte{
				azureClientIDSecretKey:     []byte("client-id"),
				azureTenantIDSecretKey:     []byte("tenant-id"),
				azureClientSecretSecretKey: []byte("client-secret"),
			},
			expectedType: &azidentity.ClientSecretCredential{},
		},
		{
			name: "workload identity",
			credentials: map[string][]byte{
				azureClientIDSecretKey: []byte("client-id"),
				azureTenantIDSecretKey: []byte("tenant-id"),
			},
			expectedType: &azidentity.WorkloadIdentityCredential{},
		},
		{
			name:         "pod workload identity",
			credentials:  nil,
			expectedType: &azidentity.WorkloadIdentityCredential{},
		},
		{
			name: "client secret without client id",
			credentials: map[string][]byte{
				azureTenantIDSecretKey:     []byte("tenant-id"),
				azureClientSecretSecretKey: []byte("client-secret"),
			},
			expectedErr: "azuremonitor credentials does not contain a clientId",
		},
		{
			name: "client secret without tenant id",
			credentials: map[string][]byte{
				azureClientIDSecretKey:     []byte("client-id"),
				azureClientSecretSecretKey: []byte("client-secret"),
			},
			expectedErr: "azuremonitor credentials does not contain a tenantId",
		},
		{
			name: "client id without tenant id",
			credentials: map[string][]byte{
				azureClientIDSecretKey: []byte("client-id"),
			},
			expectedErr: "azuremonitor credentials does not contain a tenantId",
		},
		{
			name: "tenant id without client id",
			credentials: map[string][]byte{
				azureTenantIDSecretKey: []byte("tenant-id"),
			},
			expectedErr: "azuremonitor credentials does not contain a clientId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			azureWorkloadIdentityEnv(t)

			cred, err := newAzureCredential("azuremonitor", tt.credentials, cloud.AzurePublic)
			if tt.expectedErr != "" {
				require.EqualError(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			assert.IsType(t, tt.expectedType, cred)
		})
	}
}

func TestAzureCredentialCache(t *testing.T) {
	azureWorkloadIdentityEnv(t)

	credentials := map[string][]byte{
		azureClientIDSecretKey: []byte("cache-test-client-id"),
		azureTenantIDSecretKey: []byte("tenant-id"),
	}

	first, err := azureCredential("azuremonitor", credentials, cloud.AzurePublic)
	require.NoError(t, err)

	second, err := azureCredential("azuremonitor", credentials, cloud.AzurePublic)
	require.NoError(t, err)

	assert.Same(t, first, second)

	other, err := azureCredential("azuremonitor", map[string][]byte{
		azureClientIDSecretKey: []byte("another-client-id"),
		azureTenantIDSecretKey: []byte("tenant-id"),
	}, cloud.AzurePublic)
	require.NoError(t, err)

	assert.NotSame(t, first, other)
}

func TestAzureCloudForHost(t *testing.T) {
	tests := []struct {
		host             string
		expectedCloud    cloud.Configuration
		expectedAudience string
		expectedErr      bool
	}{
		{
			host:             "flagger-abcd.eastus.prometheus.monitor.azure.com",
			expectedCloud:    cloud.AzurePublic,
			expectedAudience: "https://prometheus.monitor.azure.com",
		},
		{
			host:             "flagger-abcd.usgovvirginia.prometheus.monitor.azure.us",
			expectedCloud:    cloud.AzureGovernment,
			expectedAudience: "https://prometheus.monitor.azure.us",
		},
		{
			host:             "flagger-abcd.chinaeast.prometheus.monitor.azure.cn",
			expectedCloud:    cloud.AzureChina,
			expectedAudience: "https://prometheus.monitor.azure.cn",
		},
		{
			host:             "FLAGGER-ABCD.EASTUS.PROMETHEUS.MONITOR.AZURE.US",
			expectedCloud:    cloud.AzureGovernment,
			expectedAudience: "https://prometheus.monitor.azure.us",
		},
		{
			host:        "prometheus.example.com",
			expectedErr: true,
		},
		{
			host:        "prometheus.monitor.azure.com.example.com",
			expectedErr: true,
		},
		{
			host:        "notprometheus.monitor.azure.com",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			config, audience, err := azureCloudForHost(tt.host)
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCloud, config)
			assert.Equal(t, tt.expectedAudience, audience)
		})
	}
}

func TestAzureScope(t *testing.T) {
	tests := []struct {
		audience string
		expected string
	}{
		{audience: "https://prometheus.monitor.azure.com", expected: "https://prometheus.monitor.azure.com/.default"},
		{audience: "https://prometheus.monitor.azure.com/", expected: "https://prometheus.monitor.azure.com/.default"},
		{audience: "https://prometheus.monitor.azure.com/.default", expected: "https://prometheus.monitor.azure.com/.default"},
	}

	for _, tt := range tests {
		t.Run(tt.audience, func(t *testing.T) {
			assert.Equal(t, tt.expected, azureScope(tt.audience))
		})
	}
}
