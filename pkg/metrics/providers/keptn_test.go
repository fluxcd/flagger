package providers

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
)

func TestNewKeptnProvider(t *testing.T) {
	provider, err := NewKeptnProvider(&rest.Config{})

	require.Nil(t, err)
	require.NotNil(t, provider)

	isOnline, err := provider.IsOnline()
	require.NoError(t, err)
	require.True(t, isOnline)
}

func TestNewKeptnProvider_NoKubeConfig(t *testing.T) {
	provider, err := NewKeptnProvider(nil)

	require.Error(t, err)
	require.Nil(t, provider)
}

func TestKeptnProvider_RunQuery_KeptnMetric(t *testing.T) {
	tests := []struct {
		name        string
		setupClient func() *fake.FakeDynamicClient
		query       string
		want        float64
		wantErr     bool
	}{
		{
			name: "wrong query format",
			setupClient: func() *fake.FakeDynamicClient {
				fakeClient := fake.NewSimpleDynamicClient(
					runtime.NewScheme(),
					getSampleKeptnMetric("my-metric", "3.0"),
				)
				return fakeClient
			},
			query:   "invalid/default",
			want:    0,
			wantErr: true,
		},
		{
			name: "unsupported resource type",
			setupClient: func() *fake.FakeDynamicClient {
				fakeClient := fake.NewSimpleDynamicClient(
					runtime.NewScheme(),
					getSampleKeptnMetric("my-metric", "3.0"),
				)
				return fakeClient
			},
			query:   "invalid/default/my-metric",
			want:    0,
			wantErr: true,
		},
		{
			name: "get KeptnMetric value",
			setupClient: func() *fake.FakeDynamicClient {
				fakeClient := fake.NewSimpleDynamicClient(
					runtime.NewScheme(),
					getSampleKeptnMetric("my-metric", "3.0"),
				)
				return fakeClient
			},
			query:   "keptnmetric/default/my-metric",
			want:    3.0,
			wantErr: false,
		},
		{
			name: "KeptnMetric not found",
			setupClient: func() *fake.FakeDynamicClient {
				fakeClient := fake.NewSimpleDynamicClient(
					runtime.NewScheme(),
				)
				return fakeClient
			},
			query:   "keptnmetric/default/my-metric",
			want:    0,
			wantErr: true,
		},
		{
			name: "KeptnMetric with invalid value",
			setupClient: func() *fake.FakeDynamicClient {
				fakeClient := fake.NewSimpleDynamicClient(
					runtime.NewScheme(),
					getSampleKeptnMetric("my-metric", "invalid"),
				)
				return fakeClient
			},
			query:   "keptnmetric/default/my-metric",
			want:    0,
			wantErr: true,
		},
		{
			name: "KeptnMetric with no value",
			setupClient: func() *fake.FakeDynamicClient {
				keptnMetric := getSampleKeptnMetric("my-metric", "")

				data := keptnMetric.Object
				delete(data, "status")

				keptnMetric.SetUnstructuredContent(data)
				fakeClient := fake.NewSimpleDynamicClient(
					runtime.NewScheme(),
					keptnMetric,
				)
				return fakeClient
			},
			query:   "keptnmetric/default/my-metric",
			want:    0,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &KeptnProvider{
				client: tt.setupClient(),
			}
			got, err := k.RunQuery(tt.query)
			if tt.wantErr {
				require.NotNil(t, err)
			} else {
				require.Nil(t, err)
			}

			require.Equalf(t, tt.want, got, "RunQuery(%v)", tt.query)
		})
	}
}

func TestKeptnProvider_RunQueryAnalysis(t *testing.T) {
	tests := []struct {
		name        string
		setupClient func() *fake.FakeDynamicClient
		// verificationFunc() will run in a separate go routine
		// and check if the expected resources are created
		verificationFunc func(fakeClient *fake.FakeDynamicClient) error
		query            string
		want             float64
		wantErr          bool
	}{
		{
			name: "get passed Analysis",
			setupClient: func() *fake.FakeDynamicClient {

				scheme := runtime.NewScheme()
				scheme.AddKnownTypes(analysisResource.GroupVersion())
				scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: analysisResource.Group, Version: analysisResource.Version, Kind: "AnalysisList"}, &unstructured.UnstructuredList{})
				fakeClient := fake.NewSimpleDynamicClientWithCustomListKinds(
					scheme,
					map[schema.GroupVersionResource]string{
						analysisResource: "AnalysisList",
					},
				)

				return fakeClient
			},
			verificationFunc: func(fakeClient *fake.FakeDynamicClient) error {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				for {
					select {
					case <-ctx.Done():
						return errors.New("timed out waiting for the condition")
					case <-time.After(100 * time.Millisecond):
						// verify the creation of the expected resource
						list, err := fakeClient.Resource(analysisResource).
							Namespace("default").
							List(ctx, v1.ListOptions{
								Limit: 1,
							})
						if err != nil || len(list.Items) == 0 {
							continue
						}
						createdAnalysis := list.Items[0]
						require.Equal(t, map[string]interface{}{
							"analysisDefinition": map[string]interface{}{
								"name": "my-analysis",
							},
							"args": map[string]interface{}{
								"foo": "bar",
								"bar": "foo",
							},
							"timeframe": map[string]interface{}{
								"recent": "5m",
							},
						}, createdAnalysis.Object["spec"])

						err = unstructured.SetNestedMap(
							createdAnalysis.Object,
							map[string]interface{}{
								"state": "Completed",
								"pass":  true,
							},
							"status",
						)

						require.Nil(t, err)

						_, err = fakeClient.Resource(analysisResource).Namespace("default").Update(ctx, &createdAnalysis, v1.UpdateOptions{})

						require.Nil(t, err)
						return nil
					}
				}
			},
			query:   "analysis/default/my-analysis/5m/foo=bar;bar=foo",
			want:    1,
			wantErr: false,
		},
		{
			name: "get failed Analysis",
			setupClient: func() *fake.FakeDynamicClient {

				scheme := runtime.NewScheme()
				scheme.AddKnownTypes(analysisResource.GroupVersion())
				scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: analysisResource.Group, Version: analysisResource.Version, Kind: "AnalysisList"}, &unstructured.UnstructuredList{})
				fakeClient := fake.NewSimpleDynamicClientWithCustomListKinds(
					scheme,
					map[schema.GroupVersionResource]string{
						analysisResource: "AnalysisList",
					},
				)

				return fakeClient
			},
			verificationFunc: func(fakeClient *fake.FakeDynamicClient) error {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				for {
					select {
					case <-ctx.Done():
						return errors.New("timed out waiting for the condition")
					case <-time.After(10 * time.Millisecond):
						// verify the creation of the expected resource
						list, err := fakeClient.Resource(analysisResource).
							Namespace("default").
							List(ctx, v1.ListOptions{
								Limit: 1,
							})
						if err != nil || len(list.Items) == 0 {
							continue
						}
						createdAnalysis := list.Items[0]
						require.Equal(t, map[string]interface{}{
							"analysisDefinition": map[string]interface{}{
								"name": "my-analysis",
							},
							"args": map[string]interface{}{},
							"timeframe": map[string]interface{}{
								"recent": "1m",
							},
						}, createdAnalysis.Object["spec"])

						err = unstructured.SetNestedMap(
							createdAnalysis.Object,
							map[string]interface{}{
								"state": "Completed",
								"pass":  false,
							},
							"status",
						)

						require.Nil(t, err)

						_, err = fakeClient.Resource(analysisResource).Namespace("default").Update(ctx, &createdAnalysis, v1.UpdateOptions{})

						require.Nil(t, err)
						return nil
					}
				}
			},
			query:   "analysis/default/my-analysis",
			want:    0,
			wantErr: false,
		},
		{
			name: "analysis does not finish",
			setupClient: func() *fake.FakeDynamicClient {

				scheme := runtime.NewScheme()
				scheme.AddKnownTypes(analysisResource.GroupVersion())
				scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: analysisResource.Group, Version: analysisResource.Version, Kind: "AnalysisList"}, &unstructured.UnstructuredList{})
				fakeClient := fake.NewSimpleDynamicClientWithCustomListKinds(
					scheme,
					map[schema.GroupVersionResource]string{
						analysisResource: "AnalysisList",
					},
				)

				return fakeClient
			},
			verificationFunc: func(fakeClient *fake.FakeDynamicClient) error {
				return nil
			},
			query:   "analysis/default/my-analysis",
			want:    0,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := tt.setupClient()
			k := &KeptnProvider{
				client:          fakeClient,
				analysisTimeout: 1 * time.Second,
			}

			ctx := context.Background()
			grp, ctx := errgroup.WithContext(ctx)

			grp.Go(func() error {
				return tt.verificationFunc(fakeClient)
			})

			got, err := k.RunQuery(tt.query)
			if tt.wantErr {
				require.NotNil(t, err)
			} else {
				require.Nil(t, err)
			}

			err = grp.Wait()
			require.Nil(t, err)

			require.Equalf(t, tt.want, got, "RunQuery(%v)", tt.query)

			// verify that all created Analysis resources have been cleaned up
			list, err := fakeClient.Resource(analysisResource).
				Namespace("default").
				List(ctx, v1.ListOptions{
					Limit: 1,
				})
			require.NoError(t, err)

			require.Empty(t, list.Items)
		})
	}
}

func getSampleKeptnMetric(metricName, value string) *unstructured.Unstructured {
	keptnMetric := &unstructured.Unstructured{}
	keptnMetric.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": fmt.Sprintf("metrics.keptn.sh/%s", apiVersion),
		"kind":       "KeptnMetric",
		"metadata": map[string]interface{}{
			"name":      metricName,
			"namespace": "default",
		},
		"spec": map[string]interface{}{
			"fetchIntervalSeconds": "2",
			"provider": map[string]interface{}{
				"name": "my-provider",
			},
			"query": "my-query",
		},
		"status": map[string]interface{}{
			"value": value,
		},
	})

	return keptnMetric
}
