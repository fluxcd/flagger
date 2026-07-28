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
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	corev1 "k8s.io/api/core/v1"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

const (
	azureClientIDSecretKey     = "clientId"
	azureTenantIDSecretKey     = "tenantId"
	azureClientSecretSecretKey = "clientSecret"

	azureTokenTimeout = 30 * time.Second

	azureCredentialCacheSize = 100
)

// credentials are cached because a provider is built for every metric on every
// analysis interval and each credential holds its own token cache
var (
	azureCredentialCacheMu sync.Mutex
	azureCredentialCache   = map[string]azcore.TokenCredential{}
)

var azureClouds = []struct {
	suffix string
	config cloud.Configuration
}{
	{suffix: ".prometheus.monitor.azure.us", config: cloud.AzureGovernment},
	{suffix: ".prometheus.monitor.azure.cn", config: cloud.AzureChina},
	{suffix: ".prometheus.monitor.azure.com", config: cloud.AzurePublic},
}

// AzureMonitorProvider executes promQL queries against an Azure Monitor Workspace
type AzureMonitorProvider struct {
	*PrometheusProvider
}

// NewAzureMonitorProvider takes a provider spec and the credentials map, validates the
// address and returns a Prometheus client holding a Microsoft Entra ID token
// for the workspace
func NewAzureMonitorProvider(provider flaggerv1.MetricTemplateProvider, credentials map[string][]byte) (*AzureMonitorProvider, error) {
	if provider.InsecureSkipVerify {
		return nil, fmt.Errorf("%s provider does not support insecureSkipVerify", provider.Type)
	}

	address, err := url.Parse(provider.Address)
	if err != nil {
		return nil, fmt.Errorf("%s address %s is not a valid URL: %w", provider.Type, provider.Address, err)
	}

	if !strings.EqualFold(address.Scheme, "https") {
		return nil, fmt.Errorf("%s address %s must use https", provider.Type, provider.Address)
	}

	cloudConfig, audience, err := azureCloudForHost(address.Hostname())
	if err != nil {
		return nil, fmt.Errorf("%s address %s %w", provider.Type, provider.Address, err)
	}

	cred, err := azureCredential(provider.Type, credentials, cloudConfig)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), azureTokenTimeout)
	defer cancel()

	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{azureScope(audience)}})
	if err != nil {
		return nil, fmt.Errorf("%s token request failed: %w", provider.Type, err)
	}

	// RunQuery adds the bearer token to the header map it was given, and that map
	// is shared by every canary using this template, so it is copied first
	promProvider := provider
	promProvider.Headers = provider.Headers.Clone()

	// the Prometheus provider only reads credentials when a secret is referenced
	if promProvider.SecretRef == nil {
		promProvider.SecretRef = &corev1.LocalObjectReference{}
	}

	prom, err := NewPrometheusProvider(promProvider, map[string][]byte{"token": []byte(token.Token)})
	if err != nil {
		return nil, err
	}

	return &AzureMonitorProvider{PrometheusProvider: prom}, nil
}

func azureCredential(providerType string, credentials map[string][]byte,
	cloudConfig cloud.Configuration,
) (azcore.TokenCredential, error) {
	key := azureCredentialKey(credentials, cloudConfig)

	azureCredentialCacheMu.Lock()
	defer azureCredentialCacheMu.Unlock()

	if cred, ok := azureCredentialCache[key]; ok {
		return cred, nil
	}

	cred, err := newAzureCredential(providerType, credentials, cloudConfig)
	if err != nil {
		return nil, err
	}

	// the cache is emptied instead of grown without limit, rebuilding a
	// credential provider is not expensive
	if len(azureCredentialCache) >= azureCredentialCacheSize {
		clear(azureCredentialCache)
	}

	azureCredentialCache[key] = cred
	return cred, nil
}

func azureCredentialKey(credentials map[string][]byte, cloudConfig cloud.Configuration) string {
	return fmt.Sprintf("%s|%s|%s|%x",
		cloudConfig.ActiveDirectoryAuthorityHost,
		credentials[azureClientIDSecretKey],
		credentials[azureTenantIDSecretKey],
		sha256.Sum256(credentials[azureClientSecretSecretKey]))
}

// newAzureCredential selects an identity from the keys found in the provider
// secret, defaulting to the workload identity of the Flagger pod
func newAzureCredential(providerType string, credentials map[string][]byte,
	cloudConfig cloud.Configuration,
) (azcore.TokenCredential, error) {
	clientOptions := azcore.ClientOptions{Cloud: cloudConfig}

	clientID := string(credentials[azureClientIDSecretKey])
	tenantID := string(credentials[azureTenantIDSecretKey])
	clientSecret := string(credentials[azureClientSecretSecretKey])

	switch {
	case clientSecret != "":
		if clientID == "" {
			return nil, fmt.Errorf("%s credentials does not contain a clientId", providerType)
		}
		if tenantID == "" {
			return nil, fmt.Errorf("%s credentials does not contain a tenantId", providerType)
		}
		return azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret,
			&azidentity.ClientSecretCredentialOptions{ClientOptions: clientOptions})
	case clientID != "" && tenantID != "":
		return azidentity.NewWorkloadIdentityCredential(&azidentity.WorkloadIdentityCredentialOptions{
			ClientOptions: clientOptions,
			ClientID:      clientID,
			TenantID:      tenantID,
		})
	case clientID != "":
		return nil, fmt.Errorf("%s credentials does not contain a tenantId", providerType)
	case tenantID != "":
		return nil, fmt.Errorf("%s credentials does not contain a clientId", providerType)
	default:
		return azidentity.NewWorkloadIdentityCredential(&azidentity.WorkloadIdentityCredentialOptions{
			ClientOptions: clientOptions,
		})
	}
}

// azureCloudForHost returns the Azure cloud and the token audience matching
// the query endpoint
func azureCloudForHost(host string) (cloud.Configuration, string, error) {
	host = strings.ToLower(host)
	for _, c := range azureClouds {
		if strings.HasSuffix(host, c.suffix) {
			return c.config, "https://" + strings.TrimPrefix(c.suffix, "."), nil
		}
	}
	suffixes := make([]string, len(azureClouds))
	for i, c := range azureClouds {
		suffixes[i] = c.suffix
	}
	return cloud.Configuration{}, "", fmt.Errorf(
		"is not an Azure Monitor Workspace query endpoint, expected a host ending in %s", strings.Join(suffixes, ", "))
}

func azureScope(audience string) string {
	if strings.HasSuffix(audience, "/.default") {
		return audience
	}
	return strings.TrimSuffix(audience, "/") + "/.default"
}
