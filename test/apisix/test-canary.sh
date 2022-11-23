#!/usr/bin/env bash

# This script runs e2e tests for Canary initialization, analysis and promotion
# Prerequisites: Kubernetes Kind, Helm and Apisix ingress controller

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

cat <<EOF | kubectl apply -f -
apiVersion: apisix.apache.org/v2
kind: ApisixRoute
metadata:
  name: podinfo
  namespace: test
spec:
  http:
    - backends:
        - serviceName: podinfo
          servicePort: 80
      match:
        hosts:
          - foobar.com
        methods:
          - GET
        paths:
          - /*
      name: method
      plugins:
        - name: prometheus
          enable: true
          config:
            disable: false
            prefer_name: true
EOF

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: apisix
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  # apisix route reference
  routeRef:
    apiVersion: apisix.apache.org/v2
    kind: ApisixRoute
    name: podinfo
  # the maximum time in seconds for the canary deployment
  # to make progress before it is rollback (default 600s)
  progressDeadlineSeconds: 60
  service:
    # ClusterIP port number
    port: 80
    # container port number or name
    targetPort: 9898
  analysis:
    # schedule interval (default 60s)
    interval: 10s
    # max number of failed metric checks before rollback
    threshold: 10
    # max traffic percentage routed to canary
    # percentage (0-100)
    maxWeight: 50
    # canary increment step
    # percentage (0-100)
    stepWeight: 10
    # APISIX Prometheus checks
    metrics:
    - name: request-success-rate
      # minimum req success rate (non 5xx responses)
      # percentage (0-100)
      thresholdRange:
        min: 99
      interval: 1m
    - name: request-duration
      # builtin Prometheus check
      # maximum req duration P99
      # milliseconds
      thresholdRange:
        max: 500
      interval: 30s
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        type: rollout
        metadata:
          cmd: |-
              hey -z 1m -q 10 -c 2 -h2 -host foobar.com http://apisix-gateway.apisix/api/info
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
        kubectl -n apisix logs deployment/flagger
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
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.1

echo '>>> Waiting for canary promotion'
retries=60
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '6.0.1' && ok=true || ok=false
    sleep 10
    kubectl -n apisix logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n test logs deployment/flagger-loadtester
        kubectl -n apisix logs deployment/flagger
        kubectl -n apisix get all
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
        kubectl -n apisix logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary promotion test passed'
