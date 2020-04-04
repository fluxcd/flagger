package controller

import (
	"context"
	"errors"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/metrics/observers"
	"github.com/weaveworks/flagger/pkg/metrics/providers"
)

func (c *Controller) runBuiltinMetricChecks(canary *flaggerv1.Canary) bool {
	// override the global provider if one is specified in the canary spec
	var metricsProvider string
	// set the metrics provider to Crossover Prometheus when Crossover is the mesh provider
	// For example, `crossover` metrics provider should be used for `smi:crossover` mesh provider
	if strings.Contains(c.meshProvider, "crossover") {
		metricsProvider = "crossover"
	} else {
		metricsProvider = c.meshProvider
	}

	if canary.Spec.Provider != "" {
		metricsProvider = canary.Spec.Provider

		// set the metrics provider to Linkerd Prometheus when Linkerd is the default mesh provider
		if strings.Contains(c.meshProvider, "linkerd") {
			metricsProvider = "linkerd"
		}
	}
	// set the metrics provider to query Prometheus for the canary Kubernetes service if the canary target is Service
	if canary.Spec.TargetRef.Kind == "Service" {
		metricsProvider = metricsProvider + MetricsProviderServiceSuffix
	}

	// create observer based on the mesh provider
	observerFactory := c.observerFactory

	// override the global metrics server if one is specified in the canary spec
	if canary.Spec.MetricsServer != "" {
		var err error
		observerFactory, err = observers.NewFactory(canary.Spec.MetricsServer)
		if err != nil {
			c.recordEventErrorf(canary, "Error building Prometheus client for %s %v", canary.Spec.MetricsServer, err)
			return false
		}
	}
	observer := observerFactory.Observer(metricsProvider)

	// run metrics checks
	for _, metric := range canary.GetAnalysis().Metrics {
		if metric.Interval == "" {
			metric.Interval = canary.GetMetricInterval()
		}

		if metric.Name == "request-success-rate" {
			val, err := observer.GetRequestSuccessRate(toMetricModel(canary, metric.Interval))
			if err != nil {
				if errors.Is(err, providers.ErrNoValuesFound) {
					c.recordEventWarningf(canary,
						"Halt advancement no values found for %s metric %s probably %s.%s is not receiving traffic: %v",
						metricsProvider, metric.Name, canary.Spec.TargetRef.Name, canary.Namespace, err)
				} else {
					c.recordEventErrorf(canary, "Prometheus query failed: %v", err)
				}
				return false
			}

			if metric.ThresholdRange != nil {
				tr := *metric.ThresholdRange
				if tr.Min != nil && val < *tr.Min {
					c.recordEventWarningf(canary, "Halt %s.%s advancement success rate %.2f%% < %v%%",
						canary.Name, canary.Namespace, val, *tr.Min)
					return false
				}
				if tr.Max != nil && val > *tr.Max {
					c.recordEventWarningf(canary, "Halt %s.%s advancement success rate %.2f%% > %v%%",
						canary.Name, canary.Namespace, val, *tr.Max)
					return false
				}
			} else if metric.Threshold > val {
				c.recordEventWarningf(canary, "Halt %s.%s advancement success rate %.2f%% < %v%%",
					canary.Name, canary.Namespace, val, metric.Threshold)
				return false
			}
		}

		if metric.Name == "request-duration" {
			val, err := observer.GetRequestDuration(toMetricModel(canary, metric.Interval))
			if err != nil {
				if errors.Is(err, providers.ErrNoValuesFound) {
					c.recordEventWarningf(canary, "Halt advancement no values found for %s metric %s probably %s.%s is not receiving traffic",
						metricsProvider, metric.Name, canary.Spec.TargetRef.Name, canary.Namespace)
				} else {
					c.recordEventErrorf(canary, "Prometheus query failed: %v", err)
				}
				return false
			}
			if metric.ThresholdRange != nil {
				tr := *metric.ThresholdRange
				if tr.Min != nil && val < time.Duration(*tr.Min)*time.Millisecond {
					c.recordEventWarningf(canary, "Halt %s.%s advancement request duration %v < %v",
						canary.Name, canary.Namespace, val, time.Duration(*tr.Min)*time.Millisecond)
					return false
				}
				if tr.Max != nil && val > time.Duration(*tr.Max)*time.Millisecond {
					c.recordEventWarningf(canary, "Halt %s.%s advancement request duration %v > %v",
						canary.Name, canary.Namespace, val, time.Duration(*tr.Max)*time.Millisecond)
					return false
				}
			} else if val > time.Duration(metric.Threshold)*time.Millisecond {
				c.recordEventWarningf(canary, "Halt %s.%s advancement request duration %v > %v",
					canary.Name, canary.Namespace, val, time.Duration(metric.Threshold)*time.Millisecond)
				return false
			}
		}

		// in-line PromQL
		if metric.Query != "" {
			val, err := observerFactory.Client.RunQuery(metric.Query)
			if err != nil {
				if errors.Is(err, providers.ErrNoValuesFound) {
					c.recordEventWarningf(canary, "Halt advancement no values found for metric: %s",
						metric.Name)
				} else {
					c.recordEventErrorf(canary, "Prometheus query failed for %s: %v", metric.Name, err)
				}
				return false
			}
			if metric.ThresholdRange != nil {
				tr := *metric.ThresholdRange
				if tr.Min != nil && val < *tr.Min {
					c.recordEventWarningf(canary, "Halt %s.%s advancement %s %.2f < %v",
						canary.Name, canary.Namespace, metric.Name, val, *tr.Min)
					return false
				}
				if tr.Max != nil && val > *tr.Max {
					c.recordEventWarningf(canary, "Halt %s.%s advancement %s %.2f > %v",
						canary.Name, canary.Namespace, metric.Name, val, *tr.Max)
					return false
				}
			} else if val > metric.Threshold {
				c.recordEventWarningf(canary, "Halt %s.%s advancement %s %.2f > %v",
					canary.Name, canary.Namespace, metric.Name, val, metric.Threshold)
				return false
			}
		}
	}

	return true
}

func (c *Controller) runMetricChecks(canary *flaggerv1.Canary) bool {
	for _, metric := range canary.GetAnalysis().Metrics {
		if metric.TemplateRef != nil {
			namespace := canary.Namespace
			if metric.TemplateRef.Namespace != "" {
				namespace = metric.TemplateRef.Namespace
			}

			template, err := c.flaggerInformers.MetricInformer.Lister().MetricTemplates(namespace).Get(metric.TemplateRef.Name)
			if err != nil {
				c.recordEventErrorf(canary, "Metric template %s.%s error: %v", metric.TemplateRef.Name, namespace, err)
				return false
			}

			var credentials map[string][]byte
			if template.Spec.Provider.SecretRef != nil {
				secret, err := c.kubeClient.CoreV1().Secrets(namespace).Get(context.TODO(), template.Spec.Provider.SecretRef.Name, metav1.GetOptions{})
				if err != nil {
					c.recordEventErrorf(canary, "Metric template %s.%s secret %s error: %v",
						metric.TemplateRef.Name, namespace, template.Spec.Provider.SecretRef.Name, err)
					return false
				}
				credentials = secret.Data
			}

			factory := providers.Factory{}
			provider, err := factory.Provider(metric.Interval, template.Spec.Provider, credentials)
			if err != nil {
				c.recordEventErrorf(canary, "Metric template %s.%s provider %s error: %v",
					metric.TemplateRef.Name, namespace, template.Spec.Provider.Type, err)
				return false
			}

			query, err := observers.RenderQuery(template.Spec.Query, toMetricModel(canary, metric.Interval))
			if err != nil {
				c.recordEventErrorf(canary, "Metric template %s.%s query render error: %v",
					metric.TemplateRef.Name, namespace, err)
				return false
			}

			val, err := provider.RunQuery(query)
			if err != nil {
				if errors.Is(err, providers.ErrNoValuesFound) {
					c.recordEventWarningf(canary, "Halt advancement no values found for custom metric: %s: %v",
						metric.Name, err)
				} else {
					c.recordEventErrorf(canary, "Metric query failed for %s: %v", metric.Name, err)
				}
				return false
			}

			if metric.ThresholdRange != nil {
				tr := *metric.ThresholdRange
				if tr.Min != nil && val < *tr.Min {
					c.recordEventWarningf(canary, "Halt %s.%s advancement %s %.2f < %v",
						canary.Name, canary.Namespace, metric.Name, val, *tr.Min)
					return false
				}
				if tr.Max != nil && val > *tr.Max {
					c.recordEventWarningf(canary, "Halt %s.%s advancement %s %.2f > %v",
						canary.Name, canary.Namespace, metric.Name, val, *tr.Max)
					return false
				}
			} else if val > metric.Threshold {
				c.recordEventWarningf(canary, "Halt %s.%s advancement %s %.2f > %v",
					canary.Name, canary.Namespace, metric.Name, val, metric.Threshold)
				return false
			}
		}
	}

	return true
}

func toMetricModel(r *flaggerv1.Canary, interval string) flaggerv1.MetricTemplateModel {
	service := r.Spec.TargetRef.Name
	if r.Spec.Service.Name != "" {
		service = r.Spec.Service.Name
	}
	ingress := r.Spec.TargetRef.Name
	if r.Spec.IngressRef != nil {
		ingress = r.Spec.IngressRef.Name
	}
	return flaggerv1.MetricTemplateModel{
		Name:      r.Name,
		Namespace: r.Namespace,
		Target:    r.Spec.TargetRef.Name,
		Service:   service,
		Ingress:   ingress,
		Interval:  interval,
	}
}
