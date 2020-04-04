package router

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	contourv1 "github.com/weaveworks/flagger/pkg/apis/projectcontour/v1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

// ContourRouter is managing HTTPProxy objects
type ContourRouter struct {
	kubeClient    kubernetes.Interface
	contourClient clientset.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
}

// Reconcile creates or updates the HTTP proxy
func (cr *ContourRouter) Reconcile(canary *flaggerv1.Canary) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()

	newSpec := contourv1.HTTPProxySpec{
		Routes: []contourv1.Route{
			{
				Conditions: []contourv1.Condition{
					{
						Prefix: cr.makePrefix(canary),
					},
				},
				TimeoutPolicy: cr.makeTimeoutPolicy(canary),
				RetryPolicy:   cr.makeRetryPolicy(canary),
				Services: []contourv1.Service{
					{
						Name:   primaryName,
						Port:   int(canary.Spec.Service.Port),
						Weight: uint32(100),
						RequestHeadersPolicy: &contourv1.HeadersPolicy{
							Set: []contourv1.HeaderValue{
								cr.makeLinkerdHeaderValue(canary, primaryName),
							},
						},
					},
					{
						Name:   canaryName,
						Port:   int(canary.Spec.Service.Port),
						Weight: uint32(0),
						RequestHeadersPolicy: &contourv1.HeadersPolicy{
							Set: []contourv1.HeaderValue{
								cr.makeLinkerdHeaderValue(canary, canaryName),
							},
						},
					},
				},
			},
		},
	}

	if len(canary.GetAnalysis().Match) > 0 {
		newSpec = contourv1.HTTPProxySpec{
			Routes: []contourv1.Route{
				{
					Conditions:    cr.makeConditions(canary),
					TimeoutPolicy: cr.makeTimeoutPolicy(canary),
					RetryPolicy:   cr.makeRetryPolicy(canary),
					Services: []contourv1.Service{
						{
							Name:   primaryName,
							Port:   int(canary.Spec.Service.Port),
							Weight: uint32(100),
							RequestHeadersPolicy: &contourv1.HeadersPolicy{
								Set: []contourv1.HeaderValue{
									cr.makeLinkerdHeaderValue(canary, primaryName),
								},
							},
						},
						{
							Name:   canaryName,
							Port:   int(canary.Spec.Service.Port),
							Weight: uint32(0),
							RequestHeadersPolicy: &contourv1.HeadersPolicy{
								Set: []contourv1.HeaderValue{
									cr.makeLinkerdHeaderValue(canary, canaryName),
								},
							},
						},
					},
				},
				{
					Conditions: []contourv1.Condition{
						{
							Prefix: cr.makePrefix(canary),
						},
					},
					TimeoutPolicy: cr.makeTimeoutPolicy(canary),
					RetryPolicy:   cr.makeRetryPolicy(canary),
					Services: []contourv1.Service{
						{
							Name:   primaryName,
							Port:   int(canary.Spec.Service.Port),
							Weight: uint32(100),
							RequestHeadersPolicy: &contourv1.HeadersPolicy{
								Set: []contourv1.HeaderValue{
									cr.makeLinkerdHeaderValue(canary, primaryName),
								},
							},
						},
						{
							Name:   canaryName,
							Port:   int(canary.Spec.Service.Port),
							Weight: uint32(0),
							RequestHeadersPolicy: &contourv1.HeadersPolicy{
								Set: []contourv1.HeaderValue{
									cr.makeLinkerdHeaderValue(canary, canaryName),
								},
							},
						},
					},
				},
			},
		}
	}

	proxy, err := cr.contourClient.ProjectcontourV1().HTTPProxies(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		proxy = &contourv1.HTTPProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      apexName,
				Namespace: canary.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(canary, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: newSpec,
			Status: contourv1.Status{
				CurrentStatus: "valid",
				Description:   "valid HTTPProxy",
			},
		}

		_, err = cr.contourClient.ProjectcontourV1().HTTPProxies(canary.Namespace).Create(context.TODO(), proxy, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("HTTPProxy %s.%s create error: %w", apexName, canary.Namespace, err)
		}
		cr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("HTTPProxy %s.%s created", proxy.GetName(), canary.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("HTTPProxy %s.%s get query error: %w", apexName, canary.Namespace, err)
	}

	// update HTTPProxy but keep the original destination weights
	if proxy != nil {
		if diff := cmp.Diff(
			newSpec,
			proxy.Spec,
			cmpopts.IgnoreFields(contourv1.Service{}, "Weight"),
		); diff != "" {
			clone := proxy.DeepCopy()
			clone.Spec = newSpec

			_, err = cr.contourClient.ProjectcontourV1().HTTPProxies(canary.Namespace).Update(context.TODO(), clone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("HTTPProxy %s.%s update error: %w", apexName, canary.Namespace, err)
			}
			cr.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("HTTPProxy %s.%s updated", proxy.GetName(), canary.Namespace)
		}
	}

	return nil
}

// GetRoutes returns the service weight for primary and canary
func (cr *ContourRouter) GetRoutes(canary *flaggerv1.Canary) (
	primaryWeight int,
	canaryWeight int,
	mirrored bool,
	err error,
) {
	apexName, primaryName, _ := canary.GetServiceNames()

	proxy, err := cr.contourClient.ProjectcontourV1().HTTPProxies(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("HTTPProxy %s.%s get query error %w", apexName, canary.Namespace, err)
		return
	}

	if len(proxy.Spec.Routes) < 1 || len(proxy.Spec.Routes[0].Services) < 2 {
		err = fmt.Errorf("HTTPProxy %s.%s services not found", apexName, canary.Namespace)
		return
	}

	for _, dst := range proxy.Spec.Routes[0].Services {
		if dst.Name == primaryName {
			primaryWeight = int(dst.Weight)
			canaryWeight = 100 - primaryWeight
			return
		}
	}

	return
}

// SetRoutes updates the service weight for primary and canary
func (cr *ContourRouter) SetRoutes(
	canary *flaggerv1.Canary,
	primaryWeight int,
	canaryWeight int,
	_ bool,
) error {
	apexName, primaryName, canaryName := canary.GetServiceNames()

	if primaryWeight == 0 && canaryWeight == 0 {
		return fmt.Errorf("HTTPProxy %s.%s update failed: no valid weights", apexName, canary.Namespace)
	}

	proxy, err := cr.contourClient.ProjectcontourV1().HTTPProxies(canary.Namespace).Get(context.TODO(), apexName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("HTTPProxy %s.%s query error: %w", apexName, canary.Namespace, err)
	}

	proxy.Spec = contourv1.HTTPProxySpec{
		Routes: []contourv1.Route{
			{
				Conditions: []contourv1.Condition{
					{
						Prefix: cr.makePrefix(canary),
					},
				},
				TimeoutPolicy: cr.makeTimeoutPolicy(canary),
				RetryPolicy:   cr.makeRetryPolicy(canary),
				Services: []contourv1.Service{
					{
						Name:   primaryName,
						Port:   int(canary.Spec.Service.Port),
						Weight: uint32(primaryWeight),
						RequestHeadersPolicy: &contourv1.HeadersPolicy{
							Set: []contourv1.HeaderValue{
								cr.makeLinkerdHeaderValue(canary, primaryName),
							},
						},
					},
					{
						Name:   canaryName,
						Port:   int(canary.Spec.Service.Port),
						Weight: uint32(canaryWeight),
						RequestHeadersPolicy: &contourv1.HeadersPolicy{
							Set: []contourv1.HeaderValue{
								cr.makeLinkerdHeaderValue(canary, canaryName),
							},
						},
					},
				}},
		},
	}

	if len(canary.GetAnalysis().Match) > 0 {
		proxy.Spec = contourv1.HTTPProxySpec{
			Routes: []contourv1.Route{
				{
					Conditions:    cr.makeConditions(canary),
					TimeoutPolicy: cr.makeTimeoutPolicy(canary),
					RetryPolicy:   cr.makeRetryPolicy(canary),
					Services: []contourv1.Service{
						{
							Name:   primaryName,
							Port:   int(canary.Spec.Service.Port),
							Weight: uint32(primaryWeight),
							RequestHeadersPolicy: &contourv1.HeadersPolicy{
								Set: []contourv1.HeaderValue{
									cr.makeLinkerdHeaderValue(canary, primaryName),
								},
							},
						},
						{
							Name:   canaryName,
							Port:   int(canary.Spec.Service.Port),
							Weight: uint32(canaryWeight),
							RequestHeadersPolicy: &contourv1.HeadersPolicy{
								Set: []contourv1.HeaderValue{
									cr.makeLinkerdHeaderValue(canary, canaryName),
								},
							},
						},
					},
				},
				{
					Conditions: []contourv1.Condition{
						{
							Prefix: cr.makePrefix(canary),
						},
					},
					TimeoutPolicy: cr.makeTimeoutPolicy(canary),
					RetryPolicy:   cr.makeRetryPolicy(canary),
					Services: []contourv1.Service{
						{
							Name:   primaryName,
							Port:   int(canary.Spec.Service.Port),
							Weight: uint32(100),
							RequestHeadersPolicy: &contourv1.HeadersPolicy{
								Set: []contourv1.HeaderValue{
									cr.makeLinkerdHeaderValue(canary, primaryName),
								},
							},
						},
						{
							Name:   canaryName,
							Port:   int(canary.Spec.Service.Port),
							Weight: uint32(0),
							RequestHeadersPolicy: &contourv1.HeadersPolicy{
								Set: []contourv1.HeaderValue{
									cr.makeLinkerdHeaderValue(canary, canaryName),
								},
							},
						},
					},
				},
			},
		}
	}

	_, err = cr.contourClient.ProjectcontourV1().HTTPProxies(canary.Namespace).Update(context.TODO(), proxy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("HTTPProxy %s.%s update error: %w", apexName, canary.Namespace, err)
	}
	return nil
}

func (cr *ContourRouter) makePrefix(canary *flaggerv1.Canary) string {
	prefix := "/"

	if len(canary.Spec.Service.Match) > 0 &&
		canary.Spec.Service.Match[0].Uri != nil &&
		canary.Spec.Service.Match[0].Uri.Prefix != "" {
		prefix = canary.Spec.Service.Match[0].Uri.Prefix
	}

	return prefix
}

func (cr *ContourRouter) makeConditions(canary *flaggerv1.Canary) []contourv1.Condition {
	list := []contourv1.Condition{}

	if len(canary.GetAnalysis().Match) > 0 {
		for _, match := range canary.GetAnalysis().Match {
			for s, stringMatch := range match.Headers {
				h := &contourv1.HeaderCondition{
					Name:  s,
					Exact: stringMatch.Exact,
				}
				if stringMatch.Suffix != "" {
					h = &contourv1.HeaderCondition{
						Name:     s,
						Contains: stringMatch.Suffix,
					}
				}
				if stringMatch.Prefix != "" {
					h = &contourv1.HeaderCondition{
						Name:     s,
						Contains: stringMatch.Prefix,
					}
				}
				list = append(list, contourv1.Condition{
					Prefix: cr.makePrefix(canary),
					Header: h,
				})
			}
		}
	} else {
		list = []contourv1.Condition{
			{
				Prefix: cr.makePrefix(canary),
			},
		}
	}

	return list
}

func (cr *ContourRouter) makeTimeoutPolicy(canary *flaggerv1.Canary) *contourv1.TimeoutPolicy {
	if canary.Spec.Service.Timeout != "" {
		return &contourv1.TimeoutPolicy{
			Response: canary.Spec.Service.Timeout,
			Idle:     "5m",
		}
	}
	return nil
}

func (cr *ContourRouter) makeRetryPolicy(canary *flaggerv1.Canary) *contourv1.RetryPolicy {
	if canary.Spec.Service.Retries != nil {
		return &contourv1.RetryPolicy{
			NumRetries:    uint32(canary.Spec.Service.Retries.Attempts),
			PerTryTimeout: canary.Spec.Service.Retries.PerTryTimeout,
		}
	}
	return nil
}

func (cr *ContourRouter) makeLinkerdHeaderValue(canary *flaggerv1.Canary, serviceName string) contourv1.HeaderValue {
	return contourv1.HeaderValue{
		Name:  "l5d-dst-override",
		Value: fmt.Sprintf("%s.%s.svc.cluster.local:%v", serviceName, canary.Namespace, canary.Spec.Service.Port),
	}

}

func (cr *ContourRouter) Finalize(_ *flaggerv1.Canary) error {
	return nil
}
