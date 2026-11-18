/*
Copyright 2026 The Flux authors

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
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/signer/v4"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

const ampService = "aps"

// AmpProvider executes promQL queries against Amazon Managed Service for Prometheus
type AmpProvider struct {
	*PrometheusProvider
}

type ampSigningRoundTripper struct {
	next   http.RoundTripper
	signer *v4.Signer
	region string
}

// NewAmpProvider takes a provider spec, validates the address and region, and
// returns a Prometheus client that signs queries with AWS SigV4
func NewAmpProvider(provider flaggerv1.MetricTemplateProvider) (*AmpProvider, error) {
	if provider.Region == "" {
		return nil, fmt.Errorf("region not specified")
	}

	// AMP uses the default AWS credential chain rather than Prometheus secrets
	promProvider := provider
	promProvider.SecretRef = nil

	prom, err := NewPrometheusProvider(promProvider, nil)
	if err != nil {
		return nil, err
	}

	sess, err := session.NewSession(aws.NewConfig().WithRegion(provider.Region))
	if err != nil {
		return nil, fmt.Errorf("error creating aws session: %s", err.Error())
	}

	next := http.DefaultTransport
	if prom.client != nil && prom.client.Transport != nil {
		next = prom.client.Transport
	}

	prom.client = &http.Client{
		Transport: &ampSigningRoundTripper{
			next:   next,
			signer: v4.NewSigner(sess.Config.Credentials),
			region: provider.Region,
		},
	}

	return &AmpProvider{PrometheusProvider: prom}, nil
}

func (rt *ampSigningRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	signed := req.Clone(req.Context())
	_, err := rt.signer.Sign(signed, nil, ampService, rt.region, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to sign AWS request: %w", err)
	}
	return rt.next.RoundTrip(signed)
}
