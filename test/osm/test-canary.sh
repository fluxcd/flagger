#!/usr/bin/env bash

# This script runs OSM e2e tests for Canary initialization, analysis and promotion

set -o errexit

echo '>>> Create latency metric template'
cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: latency
  namespace: osm-system
spec:
  provider:
    type: prometheus
    address: http://osm-prometheus.osm-system.svc:7070
  query: |
    histogram_quantile(
        0.99,
        sum(
            rate(
                osm_request_duration_ms_bucket{
                    destination_namespace="{{ namespace }}",
                    destination_kind="Deployment",
                    destination_name=~"{{ target }}"
                }[{{ interval }}]
            )
        ) by (le)
    )
EOF

echo '>>> Initialising canary'
cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: osm
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 9898
    targetPort: 9898
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 50
    stepWeight: 10
    metrics:
    - name: request-success-rate
      thresholdRange:
        min: 99
      interval: 1m
    - name: request-duration
      thresholdRange:
        max: 500
      interval: 30s
    - name: latency
      templateRef:
        name: latency
        namespace: osm-system
      threshold: 300
      interval: 1m
    webhooks:
      - name: acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 15s
        metadata:
          type: bash
          cmd: "curl -sd 'test' http://podinfo-canary.test:9898/token | grep token"
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 10m -q 10 -c 2 http://podinfo-canary.test:9898/"
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
        kubectl -n osm-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary initialization test passed'

passed=$(kubectl -n test get svc/podinfo -o jsonpath='{.spec.selector.app}' 2>&1 | { grep podinfo-primary || true; })
if [ -z "$passed" ]; then
  echo -e '\u2716 podinfo selector test failed'
  exit 1
fi

echo '✔ Canary service custom metadata test passed'

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=stefanprodan/podinfo:3.1.1

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '3.1.1' && ok=true || ok=false
    sleep 10
    kubectl -n osm-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        echo ">>> kubectl -n osm-system logs deployment/flagger"
        kubectl -n osm-system logs deployment/flagger
        echo ">>> kubectl -n test logs pod/$(kubectl get pods -n test | grep flagger-loadtester | awk '{print $1}') loadtester"
        kubectl -n test logs pod/$(kubectl get pods -n test | grep flagger-loadtester | awk '{print $1}') loadtester
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
        kubectl -n osm-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary promotion test passed'

echo '>>> Initialising canary for canary rollback test'
cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: osm
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 9898
    targetPort: 9898
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 50
    stepWeight: 10
    metrics:
    - name: request-success-rate
      thresholdRange:
        min: 99
      interval: 1m
    - name: request-duration
      thresholdRange:
        max: 500
      interval: 30s
    - name: latency
      templateRef:
        name: latency
        namespace: osm-system
      threshold: 300
      interval: 1m
    webhooks:
      - name: acceptance-test
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 15s
        metadata:
          type: bash
          cmd: "curl -sd 'test' http://podinfo-canary.test:9898/token | grep token"
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 10m -q 10 -c 2 http://podinfo-canary.test:9898/status/500"
EOF

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=stefanprodan/podinfo:3.1.2

echo '>>> Waiting for canary rollback'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Failed' && ok=true || ok=false
    sleep 10
    kubectl -n osm-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n osm-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary rollback test passed'
