# Deployment Strategies

Flagger can run automated application analysis, promotion and rollback for the following deployment strategies:

* **Canary Release** \(progressive traffic shifting\)
  * Istio, Linkerd, App Mesh, NGINX, Skipper, Contour, Gloo Edge, Traefik, Open Service Mesh, Kuma, Gateway API, Apache APISIX
* **A/B Testing** \(HTTP headers and cookies traffic routing\)
  * Istio, App Mesh, NGINX, Contour, Gloo Edge, Gateway API
* **Blue/Green** \(traffic switching\)
  * Kubernetes CNI, Istio, Linkerd, App Mesh, NGINX, Contour, Gloo Edge, Open Service Mesh, Gateway API
* **Blue/Green Mirroring** \(traffic shadowing\)
  * Istio
* **Canary Release with Session Affinity** \(progressive traffic shifting combined with cookie based routing\)
  * Istio, Gateway API

For Canary releases and A/B testing you'll need a Layer 7 traffic management solution like
a service mesh or an ingress controller. For Blue/Green deployments no service mesh or ingress controller is required.

A canary analysis is triggered by changes in any of the following objects:

* Deployment PodSpec \(container image, command, ports, env, resources, etc\)
* ConfigMaps mounted as volumes or mapped to environment variables
* Secrets mounted as volumes or mapped to environment variables

## Canary Release

Flagger implements a control loop that gradually shifts traffic to the canary while measuring
key performance indicators like HTTP requests success rate, requests average duration and pod health.
Based on analysis of the KPIs a canary is promoted or aborted.

![Flagger Canary Stages](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-canary-steps.png)

The canary analysis runs periodically until it reaches the maximum traffic weight or the failed checks threshold.

Spec:

```yaml
  analysis:
    # schedule interval (default 60s)
    interval: 1m
    # max number of failed metric checks before rollback
    threshold: 10
    # max traffic percentage routed to canary
    # percentage (0-100)
    maxWeight: 50
    # canary increment step
    # percentage (0-100)
    stepWeight: 2
    # promotion increment step (default 100)
    # percentage (0-100)
    stepWeightPromotion: 100
  # deploy straight to production without
  # the metrics and webhook checks
  skipAnalysis: false
```

The above analysis, if it succeeds, will run for 25 minutes while validating the HTTP metrics and webhooks every minute.
You can determine the minimum time it takes to validate and promote a canary deployment using this formula:

```text
interval * (maxWeight / stepWeight)
```

And the time it takes for a canary to be rollback when the metrics or webhook checks are failing:

```text
interval * threshold
```

When `stepWeightPromotion` is specified, the promotion phase happens in stages, the traffic is routed back
to the primary pods in a progressive manner, the primary weight is increased until it reaches 100%.

In emergency cases, you may want to skip the analysis phase and ship changes directly to production.
At any time you can set the `spec.skipAnalysis: true`. When skip analysis is enabled,
Flagger checks if the canary deployment is healthy and promotes it without analysing it.
If an analysis is underway, Flagger cancels it and runs the promotion.

Gated canary promotion stages:

* scan for canary deployments
* check primary and canary deployment status
  * halt advancement if a rolling update is underway
  * halt advancement if pods are unhealthy
* call confirm-rollout webhooks and check results
  * halt advancement if any hook returns a non HTTP 2xx result
* call pre-rollout webhooks and check results
  * halt advancement if any hook returns a non HTTP 2xx result
  * increment the failed checks counter
* increase canary traffic weight percentage from 0% to 2% \(step weight\)
* call rollout webhooks and check results
* check canary HTTP request success rate and latency
  * halt advancement if any metric is under the specified threshold
  * increment the failed checks counter
* check if the number of failed checks reached the threshold
  * route all traffic to primary
  * scale to zero the canary deployment and mark it as failed
  * call post-rollout webhooks
  * post the analysis result to Slack
  * wait for the canary deployment to be updated and start over
* increase canary traffic weight by 2% \(step weight\) till it reaches 50% \(max weight\) 
  * halt advancement if any webhook call fails
  * halt advancement while canary request success rate is under the threshold
  * halt advancement while canary request duration P99 is over the threshold
  * halt advancement while any custom metric check fails
  * halt advancement if the primary or canary deployment becomes unhealthy 
  * halt advancement while canary deployment is being scaled up/down by HPA
* call confirm-promotion webhooks and check results
  * halt advancement if any hook returns a non HTTP 2xx result
* promote canary to primary
  * copy ConfigMaps and Secrets from canary to primary
  * copy canary deployment spec template over primary
* wait for primary rolling update to finish
  * halt advancement if pods are unhealthy
* route all traffic to primary
* scale to zero the canary deployment
* mark rollout as finished
* call post-rollout webhooks
* send notification with the canary analysis result
* wait for the canary deployment to be updated and start over

### Rollout Weights

By default Flagger uses linear weight values for the promotion, with the start value,
the step and the maximum weight value in 0 to 100 range.

Example:

```yaml
# canary.yaml
spec:
  analysis:
    maxWeight: 50
    stepWeight: 20
```

This configuration performs analysis starting from 20, increasing by 20 until weight goes above 50.  
We would have steps (canary weight : primary weight):

* 20 (20 : 80)
* 40 (40 : 60)
* 60 (60 : 40)
* promotion

In order to enable non-linear promotion a new parameter was introduced:

* `stepWeights` - determines the ordered array of weights, which shall be used during canary promotion.

Example:

```yaml
# canary.yaml
spec:
  analysis:
    stepWeights: [1, 2, 10, 80]
```

This configuration performs analysis starting from 1, going through `stepWeights` values till 80.  
We would have steps (canary weight : primary weight):

* 1 (1 : 99)
* 2 (2 : 98)
* 10 (10 : 90)
* 80 (20 : 60)
* promotion

## A/B Testing

For frontend applications that require session affinity you should use
HTTP headers or cookies match conditions to ensure a set of users
will stay on the same version for the whole duration of the canary analysis.

![Flagger A/B Testing Stages](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-abtest-steps.png)

You can enable A/B testing by specifying the HTTP match conditions and the number of iterations.
If Flagger finds a HTTP match condition, it will ignore the `maxWeight` and `stepWeight` settings.

Istio example:

```yaml
  analysis:
    # schedule interval (default 60s)
    interval: 1m
    # total number of iterations
    iterations: 10
    # max number of failed iterations before rollback
    threshold: 2
    # canary match condition
    match:
      - headers:
          x-canary:
            regex: ".*insider.*"
      - headers:
          cookie:
            regex: "^(.*?;)?(canary=always)(;.*)?$"
```

The above configuration will run an analysis for ten minutes targeting the Safari users and those that have a test cookie.
You can determine the minimum time that it takes to validate and promote a canary deployment using this formula:

```text
interval * iterations
```

And the time it takes for a canary to be rollback when the metrics or webhook checks are failing:

```text
interval * threshold
```

Istio example:

```yaml
  analysis:
    interval: 1m
    threshold: 10
    iterations: 2
    match:
      - headers:
          x-canary:
            exact: "insider"
      - headers:
          cookie:
            regex: "^(.*?;)?(canary=always)(;.*)?$"
      - sourceLabels:
          app.kubernetes.io/name: "scheduler"
```

The header keys must be lowercase and use hyphen as the separator.
Header values are case-sensitive and formatted as follows:

* `exact: "value"` for exact string match
* `prefix: "value"` for prefix-based match
* `suffix: "value"` for suffix-based match
* `regex: "value"` for [RE2](https://github.com/google/re2/wiki/Syntax) style regex-based match

Note that the `sourceLabels` match conditions are applicable only when
the `mesh` gateway is included in the `canary.service.gateways` list.

App Mesh example:

```yaml
  analysis:
    interval: 1m
    threshold: 10
    iterations: 2
    match:
      - headers:
          user-agent:
            regex: ".*Chrome.*"
```

Note that App Mesh supports a single condition.

Contour example:

```yaml
  analysis:
    interval: 1m
    threshold: 10
    iterations: 2
    match:
      - headers:
          user-agent:
            prefix: "Chrome"
```

Note that Contour does not support regex, you can use prefix, suffix or exact.

NGINX example:

```yaml
  analysis:
    interval: 1m
    threshold: 10
    iterations: 2
    match:
      - headers:
          x-canary:
            exact: "insider"
      - headers:
          cookie:
            exact: "canary"
```

Note that the NGINX ingress controller supports only exact matching for
cookies names where the value must be set to `always`.
Starting with NGINX ingress v0.31, regex matching is supported for header values.

The above configurations will route users with the x-canary header
or canary cookie to the canary instance during analysis:

```bash
curl -H 'X-Canary: insider' http://app.example.com
curl -b 'canary=always' http://app.example.com
```

## Blue/Green Deployments

For applications that are not deployed on a service mesh,
Flagger can orchestrate blue/green style deployments with Kubernetes L4 networking.
When using Istio you have the option to mirror traffic between blue and green.

![Flagger Blue/Green Stages](https://raw.githubusercontent.com/fluxcd/flagger/main/docs/diagrams/flagger-bluegreen-steps.png)

You can use the blue/green deployment strategy by replacing
`stepWeight/maxWeight` with `iterations` in the `analysis` spec:

```yaml
  analysis:
    # schedule interval (default 60s)
    interval: 1m
    # total number of iterations
    iterations: 10
    # max number of failed iterations before rollback
    threshold: 2
```

With the above configuration Flagger will run conformance and load tests on the canary pods for ten minutes.
If the metrics analysis succeeds, live traffic will be switched from
the old version to the new one when the canary is promoted.

The blue/green deployment strategy is supported for all service mesh providers.

Blue/Green rollout steps for service mesh:

* detect new revision (deployment spec, secrets or configmaps changes)
* scale up the canary (green)
* run conformance tests for the canary pods
* run load tests and metric checks for the canary pods every minute
* abort the canary release if the failure threshold is reached
* route traffic to canary (This doesn't happen when using the kubernetes provider)
* promote canary spec over primary (blue)
* wait for primary rollout
* route traffic to primary
* scale down canary

After the analysis finishes, the traffic is routed to the canary (green) before
triggering the primary (blue) rolling update,
this ensures a smooth transition to the new version avoiding dropping
in-flight requests during the Kubernetes deployment rollout.

## Blue/Green with Traffic Mirroring

Traffic Mirroring is a pre-stage in a Canary (progressive traffic shifting) or Blue/Green deployment strategy.
Traffic mirroring will copy each incoming request, sending one request to the primary and one to the canary service.
The response from the primary is sent back to the user. The response from the canary is discarded.
Metrics are collected on both requests so that the deployment will only proceed if the canary metrics are healthy.

Mirroring should be used for requests that are **idempotent** or capable of being processed
twice (once by the primary and once by the canary).
Reads are idempotent. Before using mirroring on requests that may be writes,
you should consider what will happen if a write is duplicated and handled by the primary and canary.

To use mirroring, set `spec.analysis.mirror` to `true`.

Istio example:

```yaml
  analysis:
    # schedule interval (default 60s)
    interval: 1m
    # total number of iterations
    iterations: 10
    # max number of failed iterations before rollback
    threshold: 2
    # Traffic shadowing (compatible with Istio only)
    mirror: true
    # Weight of the traffic mirrored to your canary (defaults to 100%)
    mirrorWeight: 100
```

Mirroring rollout steps for service mesh:

* detect new revision (deployment spec, secrets or configmaps changes)
* scale from zero the canary deployment
* wait for the HPA to set the canary minimum replicas
* check canary pods health
* run the acceptance tests
* abort the canary release if tests fail
* start the load tests
* mirror 100% of the traffic from primary to canary
* check request success rate and request duration every minute
* abort the canary release if the failure threshold is reached
* stop traffic mirroring after the number of iterations is reached
* route live traffic to the canary pods
* promote the canary \(update the primary secrets, configmaps and deployment spec\)
* wait for the primary deployment rollout to finish
* wait for the HPA to set the primary minimum replicas
* check primary pods health
* switch live traffic back to primary
* scale to zero the canary
* send notification with the canary analysis result

After the analysis finishes, the traffic is routed to the canary (green) before
triggering the primary (blue) rolling update, this ensures a smooth transition
to the new version avoiding dropping in-flight requests during the Kubernetes deployment rollout.

## Canary Release with Session Affinity

This deployment strategy mixes a Canary Release with A/B testing. A Canary Release is helpful when
we're trying to expose new features to users progressively, but because of the very nature of its
routing (weight based), users can land on the application's old version even after they have been
routed to the new version previously. This can be annoying, or worse break how other services interact
with our application. To address this issue, we borrow some things from A/B testing.

Since A/B testing is particularly helpful for applications that require session affinity, we integrate
cookie based routing with regular weight based routing. This means once a user is exposed to the new
version of our application (based on the traffic weights), they're always routed to that version, i.e.
they're never routed back to the old version of our application.

You can enable this, by specifying `.spec.analsyis.sessionAffinity` in the Canary:

```yaml
  analysis:
    # schedule interval (default 60s)
    interval: 1m
    # max number of failed metric checks before rollback
    threshold: 10
    # max traffic percentage routed to canary
    # percentage (0-100)
    maxWeight: 50
    # canary increment step
    # percentage (0-100)
    stepWeight: 2
    # session affinity config
    sessionAffinity:
      # name of the cookie used
      cookieName: flagger-cookie
      # max age of the cookie (in seconds)
      # optional; defaults to 86400
      maxAge: 21600
```

`.spec.analysis.sessionAffinity.cookieName` is the name of the Cookie that is stored. The value of the
cookie is a randomly generated string of characters that act as a unique identifier. For the above
config, the response header of a request routed to the canary deployment during a Canary run will look like:
```
Set-Cookie: flagger-cookie=LpsIaLdoNZ; Max-Age=21600
```

After a Canary run is over and all traffic is shifted back to the primary deployment, all responses will
have the following header:
```
Set-Cookie: flagger-cookie=LpsIaLdoNZ; Max-Age=-1
```
This tells the client to delete the cookie, making sure there are no junk cookies lying around in the user's
system.

If a new Canary run is triggered, the response header will set a new cookie for all requests routed to
the Canary deployment:
```
Set-Cookie: flagger-cookie=McxKdLQoIN; Max-Age=21600
```
