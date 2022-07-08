#!/usr/bin/env bash

# This script runs e2e tests targetting Flagger's integration with KEDA ScaledObjects.

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

cat <<EOF | kubectl apply -f -
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: podinfo-so
  namespace: test
spec:
  scaleTargetRef:
    name: podinfo
  pollingInterval: 10
  cooldownPeriod: 20
  minReplicaCount: 1
  maxReplicaCount: 3
  triggers:
  - type: prometheus
    metadata:
      serverAddress: http://flagger-prometheus.flagger-system:9090
      metricName: http_requests_total
      query: sum(rate(http_requests_total{ app="podinfo" }[30s]))
      threshold: '5'
EOF

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: podinfo-svc
  namespace: test
spec:
  type: ClusterIP
  selector:
    app: podinfo
  ports:
    - name: http
      port: 9898
      protocol: TCP
      targetPort: http
---
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: test
spec:
  provider: kubernetes
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  autoscalerRef:
    apiVersion: keda.sh/v1alpha1
    kind: ScaledObject
    name: podinfo-so
  progressDeadlineSeconds: 60
  service:
    port: 80
    targetPort: 9898
    name: podinfo-svc
    portDiscovery: true
  analysis:
    interval: 15s
    threshold: 10
    iterations: 8
    metrics:
      - name: request-success-rate
        interval: 1m
        thresholdRange:
          min: 99
      - name: request-duration
        interval: 30s
        thresholdRange:
          max: 500
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 2m -q 20 -c 2 http://podinfo-svc-canary.test/"
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
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary initialization test passed'

expectedQuery='sum(rate(http_requests_total{ app="podinfo-primary" }[30s]))'
if kubectl -n test get scaledobjects podinfo-so-primary; then
    query=$(kubectl -n test get scaledobjects podinfo-so-primary -o=jsonpath='{.spec.triggers[0].metadata.query}')
    if [[ "$query" = "$expectedQuery" ]]; then
        echo '✔ Primary ScaledObject successfully reconciled'
    else
        kubectl -n test get scaledobjects podinfo-so-primary -oyaml
        echo '⨯ Primary ScaledObject query does not match expected query'
        exit 1
    fi
else
    echo '⨯ Primary ScaledObject not found'
    exit 1
fi

val=$(kubectl -n test get scaledobject podinfo-so -o=jsonpath='{.metadata.annotations.autoscaling\.keda\.sh\/paused-replicas}' | xargs)
if [[ "$val" = "0" ]]; then
    echo '✔ Successfully paused autoscaling for target ScaledObject'
else
    echo '⨯ Could not pause autoscaling for target ScaledObject'
fi

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.1

echo '>>> Waiting for ScaledObject autoscaling to get unpaused'
retries=20
count=0
ok=false
until ${ok}; do
    val=$(kubectl -n test get scaledobject podinfo-so -o=jsonpath='{.metadata.annotations.autoscaling\.keda\.sh\/paused-replicas}' | xargs)
    if [[ "$val" = "" ]]; then
        ok=true
    fi
    sleep 2
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        kubectl -n test get scaledobject podinfo-so -oyaml
        echo "No more retries left"
        exit 1
    fi
done

echo '>>> Waiting for canary deployment to be scaled up'
retries=20
count=0
ok=false
until ${ok}; do
    kubectl -n test get deployment/podinfo -oyaml | grep 'replicas: 3' && ok=true || ok=false
    sleep 5
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        kubectl -n test get deploy/podinfo -oyaml
        echo "No more retries left"
        exit 1
    fi
done

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '6.0.1' && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        kubectl -n test get httpproxy podinfo -oyaml
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary promotion test passed'

echo '>>> Waiting for canary finalization'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Succeeded' && ok=true || ok=false
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

val=$(kubectl -n test get scaledobject podinfo-so -o=jsonpath='{.metadata.annotations.autoscaling\.keda\.sh\/paused-replicas}' | xargs)
if [[ "$val" = "0" ]]; then
    echo '✔ Successfully paused autoscaling for target ScaledObject'
else
    echo '⨯ Could not pause autoscaling for target ScaledObject'
fi
