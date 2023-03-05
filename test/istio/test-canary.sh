#!/usr/bin/env bash

# This script runs e2e tests for Canary, B/G and A/B initialization, analysis and promotion
# Prerequisites: Kubernetes Kind and Istio

set -o errexit

echo '>>> Create latency metric template'
cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: latency
  namespace: istio-system
spec:
  provider:
    type: prometheus
    address: http://prometheus.istio-system:9090
  query: |
    histogram_quantile(
        0.99,
        sum(
            rate(
                istio_request_duration_milliseconds_bucket{
                    reporter="{{ variables.reporter }}",
                    destination_workload_namespace="{{ namespace }}",
                    destination_workload=~"{{ target }}"
                }[{{ interval }}]
            )
        ) by (le)
    )
EOF

echo '>>> Initialising canaries'
cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 9898
    portDiscovery: true
    apex:
      annotations:
        test: "annotations-test"
      labels:
        test: "labels-test"
    headers:
      request:
        add:
          x-envoy-upstream-rq-timeout-ms: "15000"
          x-envoy-max-retries: "10"
          x-envoy-retry-on: "gateway-error,connect-failure,refused-stream"
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 30
    stepWeight: 10
    metrics:
    - name: request-success-rate
      thresholdRange:
        min: 99
      interval: 1m
    - name: latency
      templateRef:
        name: latency
        namespace: istio-system
      thresholdRange:
        max: 500
      interval: 1m
      templateVariables:
        reporter: destination
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 http://podinfo.test:9898/"
          logCmdOutput: "true"
EOF

echo '>>> Waiting for primary to be ready'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Initialized' && ok=true || ok=false
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary initialization test passed'

passed=$(kubectl -n test get svc/podinfo -oyaml 2>&1 | { grep annotations-test || true; })
if [ -z "$passed" ]; then
  echo -e '\u2716 podinfo annotations test failed'
  exit 1
fi
passed=$(kubectl -n test get svc/podinfo -oyaml 2>&1 | { grep labels-test || true; })
if [ -z "$passed" ]; then
  echo -e '\u2716 podinfo labels test failed'
  exit 1
fi
passed=$(kubectl -n test get svc/podinfo -o jsonpath='{.spec.selector.app}' 2>&1 | { grep podinfo-primary || true; })
if [ -z "$passed" ]; then
  echo -e '\u2716 podinfo selector test failed'
  exit 1
fi

echo '✔ Canary service custom metadata test passed'

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.1

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '6.0.1' && ok=true || ok=false
    sleep 10
    kubectl -n istio-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '>>> Waiting for canary finalization'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Succeeded' && ok=true || ok=false
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary promotion test passed'

if [[ "$1" = "canary" ]]; then
  exit 0
fi

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    portDiscovery: true
    port: 80
    targetPort: 9898
    portName: http-podinfo
  analysis:
    interval: 10s
    threshold: 5
    iterations: 5
    metrics:
    - name: request-success-rate
      thresholdRange:
        min: 99
      interval: 1m
    - name: latency
      templateRef:
        name: latency
        namespace: istio-system
      thresholdRange:
        max: 500
      interval: 30s
      templateVariables:
        reporter: destination
    webhooks:
      - name: http-acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 30s
        metadata:
          type: bash
          cmd: "curl -sd 'test' http://podinfo-canary/token | grep token"
      - name: grpc-acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 30s
        metadata:
          type: bash
          cmd: "grpc_health_probe -connect-timeout=1s -addr=podinfo-canary:9999"
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 5m -q 10 -c 2 http://podinfo-canary.test/"
EOF

echo '>>> Triggering B/G deployment'
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.2

echo '>>> Waiting for B/G promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '6.0.2' && ok=true || ok=false
    sleep 10
    kubectl -n istio-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '>>> Waiting for B/G finalization'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Succeeded' && ok=true || ok=false
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ B/G promotion test passed'

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    portDiscovery: true
    port: 80
    portName: http-podinfo
    targetPort: http
  analysis:
    interval: 10s
    threshold: 5
    iterations: 5
    match:
      - headers:
          cookie:
            regex: "^(.*?;)?(type=insider)(;.*)?$"
    metrics:
    - name: request-success-rate
      thresholdRange:
        min: 99
      interval: 1m
    - name: latency
      templateRef:
        name: latency
        namespace: istio-system
      thresholdRange:
        max: 500
      interval: 30s
      templateVariables:
        reporter: destination
    webhooks:
      - name: pre
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 -H 'Cookie: type=insider' http://podinfo-canary.test/"
          logCmdOutput: "true"
      - name: promote-gate
        type: confirm-promotion
        url: http://flagger-loadtester.test/gate/approve
      - name: post
        type: post-rollout
        url: http://flagger-loadtester.test/
        timeout: 15s
        metadata:
          type: cmd
          cmd: "curl -s http://podinfo.test/"
          logCmdOutput: "true"
EOF

echo '>>> Triggering A/B testing'
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.3

echo '>>> Waiting for A/B testing promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '6.0.3' && ok=true || ok=false
    sleep 10
    kubectl -n istio-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ A/B testing promotion test passed'

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  revertOnDeletion: true
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    portDiscovery: true
    port: 80
    portName: http-podinfo
    targetPort: http
  analysis:
    interval: 10s
    threshold: 5
    iterations: 5
    match:
      - headers:
          cookie:
            regex: "^(.*?;)?(type=insider)(;.*)?$"
    metrics:
    - name: request-success-rate
      thresholdRange:
        min: 99
      interval: 1m
    - name: latency
      templateRef:
        name: latency
        namespace: istio-system
      thresholdRange:
        max: 500
      interval: 30s
      templateVariables:
        reporter: destination
    webhooks:
      - name: pre
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 -H 'Cookie: type=insider' http://podinfo-canary.test/"
          logCmdOutput: "true"
      - name: promote-gate
        type: confirm-promotion
        url: http://flagger-loadtester.test/gate/approve
      - name: post
        type: post-rollout
        url: http://flagger-loadtester.test/
        timeout: 15s
        metadata:
          type: cmd
          cmd: "curl -s http://podinfo.test/"
          logCmdOutput: "true"
EOF

echo '>>> Waiting for finalizers to be present'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl get canary podinfo -n test -o jsonpath='{.metadata.finalizers}' | grep "finalizer.flagger.app" && ok=true || ok=false
    sleep 10
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe canary/podinfo
        echo "No more retries left"
        exit 1
    fi
done

kubectl delete canary podinfo -n test

echo '>>> Waiting for primary to revert'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl get deployment podinfo -n test -o jsonpath='{.spec.replicas}' | grep 1 && ok=true || ok=false
    sleep 10
    kubectl -n istio-system logs deployment/flagger --tail 10
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe canary/podinfo
        echo "No more retries left"
        exit 1
    fi
done
echo '✔ Delete testing passed'



cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  labels:
    app: podinfo
  name: podinfo
  namespace: test
spec:
  ports:
  - name: http
    port: 9898
    protocol: TCP
    targetPort: http
  selector:
    app: podinfo
  type: ClusterIP
---
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: podinfo
  namespace: test
spec:
  gateways:
  - istio-system/ingressgateway
  hosts:
  - app.example.com
  - podinfo
  http:
  - retries:
      attempts: 3
      perTryTimeout: 1s
      retryOn: gateway-error,connect-failure,refused-stream
    route:
    - destination:
        host: podinfo
---
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  revertOnDeletion: true
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    portDiscovery: true
    port: 80
    portName: http-podinfo
    targetPort: http
  analysis:
    interval: 10s
    threshold: 5
    iterations: 5
    match:
      - headers:
          cookie:
            regex: "^(.*?;)?(type=insider)(;.*)?$"
    metrics:
    - name: request-success-rate
      thresholdRange:
        min: 99
      interval: 1m
    - name: latency
      templateRef:
        name: latency
        namespace: istio-system
      thresholdRange:
        max: 500
      interval: 30s
      templateVariables:
        reporter: destination
    webhooks:
      - name: pre
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 -H 'Cookie: type=insider' http://podinfo-canary.test/"
          logCmdOutput: "true"
      - name: promote-gate
        type: confirm-promotion
        url: http://flagger-loadtester.test/gate/approve
      - name: post
        type: post-rollout
        url: http://flagger-loadtester.test/
        timeout: 15s
        metadata:
          type: cmd
          cmd: "curl -s http://podinfo.test/"
          logCmdOutput: "true"

EOF

echo '>>> Waiting for canary to initialize'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Initialized' && ok=true || ok=false
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

kubectl delete canary podinfo -n test

echo '>>> Waiting for revert'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl get svc/podinfo vs/podinfo -n test -o jsonpath="{range .items[*]}{.metadata.name}{'\n'}{end}" | wc -l | grep 2 && ok=true || ok=false
    sleep 10
    kubectl -n istio-system logs deployment/flagger --tail 10
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe canary/podinfo
        kubectl -n test describe svc/podinfo
        kubectl -n test describe vs/podinfo
        echo "No more retries left"
        exit 1
    fi
done
echo '✔ Revert testing passed'


kubectl -n istio-system logs deployment/flagger

echo '✔ All tests passed'
