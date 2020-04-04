package router

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	istiov1alpha1 "github.com/weaveworks/flagger/pkg/apis/istio/common/v1alpha1"
	istiov1alpha3 "github.com/weaveworks/flagger/pkg/apis/istio/v1alpha3"
)

func TestIngressRouter_Reconcile(t *testing.T) {
	mocks := newFixture(nil)
	router := &IngressRouter{
		logger:            mocks.logger,
		kubeClient:        mocks.kubeClient,
		annotationsPrefix: "custom.ingress.kubernetes.io",
	}

	err := router.Reconcile(mocks.ingressCanary)
	require.NoError(t, err)

	canaryAn := "custom.ingress.kubernetes.io/canary"

	canaryName := fmt.Sprintf("%s-canary", mocks.ingressCanary.Spec.IngressRef.Name)
	inCanary, err := router.kubeClient.NetworkingV1beta1().Ingresses("default").Get(context.TODO(), canaryName, metav1.GetOptions{})
	require.NoError(t, err)

	// test initialisation
	assert.Equal(t, "false", inCanary.Annotations[canaryAn])
}

func TestIngressRouter_GetSetRoutes(t *testing.T) {
	mocks := newFixture(nil)
	router := &IngressRouter{
		logger:            mocks.logger,
		kubeClient:        mocks.kubeClient,
		annotationsPrefix: "prefix1.nginx.ingress.kubernetes.io",
	}

	err := router.Reconcile(mocks.ingressCanary)
	require.NoError(t, err)

	p, c, m, err := router.GetRoutes(mocks.ingressCanary)
	require.NoError(t, err)

	p = 50
	c = 50
	m = false

	err = router.SetRoutes(mocks.ingressCanary, p, c, m)
	require.NoError(t, err)

	canaryAn := "prefix1.nginx.ingress.kubernetes.io/canary"
	canaryWeightAn := "prefix1.nginx.ingress.kubernetes.io/canary-weight"

	canaryName := fmt.Sprintf("%s-canary", mocks.ingressCanary.Spec.IngressRef.Name)
	inCanary, err := router.kubeClient.NetworkingV1beta1().Ingresses("default").Get(context.TODO(), canaryName, metav1.GetOptions{})
	require.NoError(t, err)

	// test rollout
	assert.Equal(t, "true", inCanary.Annotations[canaryAn])
	assert.Equal(t, "50", inCanary.Annotations[canaryWeightAn])

	p = 100
	c = 0
	m = false

	err = router.SetRoutes(mocks.ingressCanary, p, c, m)
	require.NoError(t, err)

	inCanary, err = router.kubeClient.NetworkingV1beta1().Ingresses("default").Get(context.TODO(), canaryName, metav1.GetOptions{})
	require.NoError(t, err)

	// test promotion
	assert.Equal(t, "false", inCanary.Annotations[canaryAn])
}

func TestIngressRouter_ABTest(t *testing.T) {
	mocks := newFixture(nil)
	router := &IngressRouter{
		logger:            mocks.logger,
		kubeClient:        mocks.kubeClient,
		annotationsPrefix: "nginx.ingress.kubernetes.io",
	}

	tables := []struct {
		makeCanary func() *flaggerv1.Canary
		annotation string
	}{
		// Header exact match
		{
			makeCanary: func() *flaggerv1.Canary {
				mocks.ingressCanary.Spec.Analysis.Iterations = 1
				mocks.ingressCanary.Spec.Analysis.Match = []istiov1alpha3.HTTPMatchRequest{
					{
						Headers: map[string]istiov1alpha1.StringMatch{
							"x-user-type": {
								Exact: "test",
							},
						},
					},
				}
				return mocks.ingressCanary
			},
			annotation: router.GetAnnotationWithPrefix("canary-by-header-value"),
		},
		// Header regex match
		{
			makeCanary: func() *flaggerv1.Canary {
				mocks.ingressCanary.Spec.Analysis.Iterations = 1
				mocks.ingressCanary.Spec.Analysis.Match = []istiov1alpha3.HTTPMatchRequest{
					{
						Headers: map[string]istiov1alpha1.StringMatch{
							"x-user-type": {
								Regex: "test",
							},
						},
					},
				}
				return mocks.ingressCanary
			},
			annotation: router.GetAnnotationWithPrefix("canary-by-header-pattern"),
		},
		// Cookie exact match
		{
			makeCanary: func() *flaggerv1.Canary {
				mocks.ingressCanary.Spec.Analysis.Iterations = 1
				mocks.ingressCanary.Spec.Analysis.Match = []istiov1alpha3.HTTPMatchRequest{
					{
						Headers: map[string]istiov1alpha1.StringMatch{
							"cookie": {
								Exact: "test",
							},
						},
					},
				}
				return mocks.ingressCanary
			},
			annotation: router.GetAnnotationWithPrefix("canary-by-cookie"),
		},
	}

	for _, table := range tables {
		err := router.Reconcile(table.makeCanary())
		require.NoError(t, err)

		err = router.SetRoutes(table.makeCanary(), 50, 50, false)
		require.NoError(t, err)

		canaryAn := router.GetAnnotationWithPrefix("canary")

		canaryName := fmt.Sprintf("%s-canary", table.makeCanary().Spec.IngressRef.Name)
		inCanary, err := router.kubeClient.NetworkingV1beta1().Ingresses("default").Get(context.TODO(), canaryName, metav1.GetOptions{})
		require.NoError(t, err)

		// test initialisation
		assert.Equal(t, "true", inCanary.Annotations[canaryAn])
		assert.Equal(t, "test", inCanary.Annotations[table.annotation])
	}

}
