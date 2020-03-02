# Upgrade Guide

This document describes how to upgrade Flagger.

### Upgrade canaries v1alpha3 to v1beta1

Canary CRD changes in `canaries.flagger.app/v1beta1`:
* the `spec.canaryAnalysis` field has been deprecated and replaced with `spec.analysis`
* the `spec.analysis.interval` and `spec.analysis.threshold` fields are required
* the `status.lastAppliedSpec` and `status.lastPromotedSpec` hashing algorithm changed to `hash/fnv`
* the `spec.analysis.alerts` array can reference `alertproviders.flagger.app/v1beta1` resources
* the `spec.analysis.metrics[].templateRef` can reference a `metrictemplate.flagger.app/v1beta1` resource
* the `metric.threshold` field has been deprecated and replaced with `metric.thresholdRange`
* the `spec.targetRef` can reference `DaemonSet` kind

Upgrade procedure:
* install the `v1beta1` CRDs
* update Flagger deployment
* replace `apiVersion: flagger.app/v1alpha3` with `apiVersion: flagger.app/v1beta1` in all canary manifests
* replace `spec.canaryAnalysis` with `spec.analysis` in all canary manifests
* update canary manifests in cluster

**Note** that after upgrading Flagger, all canaries will be triggered as the hash value used for tracking changes
is computed differently. You can set `spec.skipAnalysis: true` in all canary manifests before upgrading Flagger,
do the upgrade, wait for Flagger to finish the no-op promotions and finally set `skipAnalysis` to `false`.

Update builtin metrics:
* replace `threshold` with `thresholdRange.min` for request-success-rate
* replace `threshold` with `thresholdRange.max` for request-duration

```yaml
metrics:
- name: request-success-rate
  thresholdRange:
    min: 99
  interval: 1m
- name: request-duration
  thresholdRange:
    max: 500
  interval: 1m
```
