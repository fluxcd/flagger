package router

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSkipperRouter_Reconcile(t *testing.T) {
	assert := assert.New(t)
	mocks := newFixture(nil)

	for _, tt := range []struct {
		name    string
		mocks   func() fixture
		wantErr bool
	}{
		{
			"creating new canary ingress w/ default settings",
			func() fixture { return mocks },
			false,
		}, {
			"updating existing canary ingress",
			func() fixture {
				ti := newTestIngress()
				ti.Annotations["something"] = "changed"
				_, err := mocks.kubeClient.NetworkingV1beta1().Ingresses("default").Update(
					context.TODO(), ti, metav1.UpdateOptions{})
				assert.NoError(err)
				return mocks
			},
			false,
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			mocks := tt.mocks()
			router := &SkipperRouter{
				kubeClient: mocks.kubeClient,
				logger:     mocks.logger,
			}
			assert.NoError(router.Reconcile(mocks.ingressCanary))
			canaryName := fmt.Sprintf("%s-canary", mocks.ingressCanary.Spec.IngressRef.Name)
			inCanary, err := router.kubeClient.NetworkingV1beta1().Ingresses("default").Get(
				context.TODO(), canaryName, metav1.GetOptions{})
			assert.NoError(err)
			// test initialisation
			assert.JSONEq(`{ "podinfo-primary":  100, "podinfo-canary":  0 }`, inCanary.Annotations["zalando.org/backend-weights"])
			assert.Equal("podinfo-primary", inCanary.Spec.Rules[0].HTTP.Paths[0].Backend.ServiceName, "backend flipped over")
			assert.Equal("podinfo-canary", inCanary.Spec.Rules[0].HTTP.Paths[1].Backend.ServiceName, "backend flipped over")
			assert.Len(inCanary.Spec.Rules[0].HTTP.Paths, 2)
			inApex, err := router.kubeClient.NetworkingV1beta1().Ingresses("default").Get(
				context.TODO(), mocks.ingressCanary.Spec.IngressRef.Name, metav1.GetOptions{})
			assert.NoError(err)
			assert.Equal(inCanary.Spec.Rules[0].HTTP.Paths[0].Backend.ServicePort,
				inApex.Spec.Rules[0].HTTP.Paths[0].Backend.ServicePort, "canary backend not cloned")
			assert.Equal(inCanary.Spec.Rules[0].HTTP.Paths[0].Backend.ServicePort,
				inCanary.Spec.Rules[0].HTTP.Paths[1].Backend.ServicePort, "canary backend not cloned")
		})
	}
}

func TestSkipperRouter_GetSetRoutes(t *testing.T) {
	assert := assert.New(t)
	mocks := newFixture(nil)

	router := &SkipperRouter{logger: mocks.logger, kubeClient: mocks.kubeClient}
	assert.NoError(router.Reconcile(mocks.ingressCanary))

	p, c, m, err := router.GetRoutes(mocks.ingressCanary)
	assert.NoError(err)
	assert.Equal(100, p)
	assert.Equal(0, c)
	assert.Equal(false, m)

	tests := []struct {
		name            string
		primary, canary int
	}{
		{name: "0%", primary: 100, canary: 0},
		{name: "10%", primary: 90, canary: 10},
		{name: "20%", primary: 80, canary: 20},
		{name: "30%", primary: 70, canary: 30},
		{name: "85%", primary: 15, canary: 85},
		{name: "100%", primary: 0, canary: 100},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(router.SetRoutes(mocks.ingressCanary, tt.primary, tt.canary, false))
			inCanary, err := router.kubeClient.NetworkingV1beta1().Ingresses("default").Get(
				context.TODO(), fmt.Sprintf("%s-canary", mocks.ingressCanary.Spec.IngressRef.Name), metav1.GetOptions{})
			assert.NoError(err)
			assert.JSONEq(fmt.Sprintf(`{"podinfo-primary": %d,"podinfo-canary": %d}`, tt.primary, tt.canary),
				inCanary.Annotations["zalando.org/backend-weights"])
			p, c, m, err = router.GetRoutes(mocks.ingressCanary)
			assert.NoError(err)
			assert.Equal(tt.primary, p)
			assert.Equal(tt.canary, c)
			assert.Equal(false, m)
		})
	}

}
