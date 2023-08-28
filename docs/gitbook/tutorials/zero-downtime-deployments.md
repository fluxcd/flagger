# Zero downtime deployments

This is a list of things you should consider when dealing with a high traffic production environment if you want to minimise the impact of rolling updates and downscaling.

## Deployment strategy

Limit the number of unavailable pods during a rolling update:

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  progressDeadlineSeconds: 120
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0
```

The default progress deadline for a deployment is ten minutes. You should consider adjusting this value to make the deployment process fail faster.

## Liveness health check

You application should expose a HTTP endpoint that Kubernetes can call to determine if your app transitioned to a broken state from which it can't recover and needs to be restarted.

```yaml
livenessProbe:
  exec:
    command:
    - wget
    - --quiet
    - --tries=1
    - --timeout=4
    - --spider
    - http://localhost:8080/healthz
  timeoutSeconds: 5
  initialDelaySeconds: 5
```

If you've enabled mTLS, you'll have to use `exec` for liveness and readiness checks since kubelet is not part of the service mesh and doesn't have access to the TLS cert.

## Readiness health check

You application should expose a HTTP endpoint that Kubernetes can call to determine if your app is ready to receive traffic.

```yaml
readinessProbe:
  exec:
    command:
    - wget
    - --quiet
    - --tries=1
    - --timeout=4
    - --spider
    - http://localhost:8080/readyz
  timeoutSeconds: 5
  initialDelaySeconds: 5
  periodSeconds: 5
```

If your app depends on external services, you should check if those services are available before allowing Kubernetes to route traffic to an app instance. Keep in mind that the Envoy sidecar can have a slower startup than your app. This means that on application start you should retry for at least a couple of seconds any external connection.

## Graceful shutdown

Before a pod gets terminated, Kubernetes sends a `SIGTERM` signal to every container and waits for period of time \(30s by default\) for all containers to exit gracefully. If your app doesn't handle the `SIGTERM` signal or if it doesn't exit within the grace period, Kubernetes will kill the container and any inflight requests that your app is processing will fail.

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      terminationGracePeriodSeconds: 60
      containers:
      - name: app
        lifecycle:
          preStop:
            exec:
              command:
              - sleep
              - "10"
```

Your app container should have a `preStop` hook that delays the container shutdown. This will allow the service mesh to drain the traffic and remove this pod from all other Envoy sidecars before your app becomes unavailable.

## Delay Envoy shutdown

Even if your app reacts to `SIGTERM` and tries to complete the inflight requests before shutdown, that doesn't mean that the response will make it back to the caller. If the Envoy sidecar shuts down before your app, then the caller will receive a 503 error.

To mitigate this issue you can add a `preStop` hook to the Istio proxy and wait for the main app to exit before Envoy exits.

```bash
#!/bin/bash
set -e
if ! pidof envoy &>/dev/null; then
  exit 0
fi

if ! pidof pilot-agent &>/dev/null; then
  exit 0
fi

while [ $(netstat -plunt | grep tcp | grep -v envoy | wc -l | xargs) -ne 0 ]; do
  sleep 1;
done

exit 0
```

You'll have to build your own Envoy docker image with the above script and modify the Istio injection webhook with the `preStop` directive.

Thanks to Stono for his excellent [tips](https://github.com/istio/istio/issues/12183) on minimising 503s.

## Resource requests and limits

Setting CPU and memory requests/limits for all workloads is a mandatory step if you're running a production system. Without limits your nodes could run out of memory or become unresponsive due to CPU exhausting. Without CPU and memory requests, the Kubernetes scheduler will not be able to make decisions about which nodes to place pods on.

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: app
        resources:
          limits:
            cpu: 1000m
            memory: 1Gi
          requests:
            cpu: 100m
            memory: 128Mi
```

Note that without resource requests the horizontal pod autoscaler can't determine when to scale your app.

## Autoscaling

A production environment should be able to handle traffic bursts without impacting the quality of service. This can be achieved with Kubernetes autoscaling capabilities. Autoscaling in Kubernetes has two dimensions: the Cluster Autoscaler that deals with node scaling operations and the Horizontal Pod Autoscaler that automatically scales the number of pods in a deployment.

```yaml
apiVersion: autoscaling/v2beta2
kind: HorizontalPodAutoscaler
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: app
  minReplicas: 2
  maxReplicas: 4
  metrics:
  - type: Resource
    resource:
      name: cpu
      targetAverageValue: 900m
  - type: Resource
    resource:
      name: memory
      targetAverageValue: 768Mi
```

The above HPA ensures your app will be scaled up before the pods reach the CPU or memory limits.

## Ingress retries

To minimise the impact of downscaling operations you can make use of Envoy retry capabilities.

```yaml
apiVersion: flagger.app/v1beta1
kind: Canary
spec:
  service:
    port: 9898
    gateways:
    - istio-system/public-gateway
    hosts:
    - app.example.com
    retries:
      attempts: 10
      perTryTimeout: 5s
      retryOn: "gateway-error,connect-failure,refused-stream"
```

When the HPA scales down your app, your users could run into 503 errors. The above configuration will make Envoy retry the HTTP requests that failed due to gateway errors.

