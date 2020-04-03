#!/usr/bin/env bash

# This script runs e2e tests for Canary initialization, analysis and promotion
# Prerequisites: Kubernetes Kind, Helm and NGINX ingress controller

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Creating test namespace'
kubectl create namespace test

echo '>>> Installing load tester'
kubectl apply -k ${REPO_ROOT}/kustomize/tester
kubectl -n test rollout status deployment/flagger-loadtester

echo '>>> Initialising canary'
kubectl apply -f ${REPO_ROOT}/test/e2e-workload.yaml
kubectl apply -f ${REPO_ROOT}/test/e2e-ingress.yaml

echo '>>> Create metric templates'
cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: error-rate
  namespace: ingress-nginx
spec:
  provider:
    type: prometheus
    address: http://flagger-prometheus.ingress-nginx:9090
  query: |
    100 - sum(
            rate(
                http_request_duration_seconds_count{
                    kubernetes_namespace="{{ namespace }}",
                    kubernetes_pod_name=~"{{ target }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)",
                    path="root",
                    status!~"5.*"
                }[{{ interval }}]
            )
        )
        /
        sum(
            rate(
                http_request_duration_seconds_count{
                    kubernetes_namespace="{{ namespace }}",
                    kubernetes_pod_name=~"{{ target }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)",
                    path="root"
                }[{{ interval }}]
            )
        )
        * 100
EOF

cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: MetricTemplate
metadata:
  name: latency
  namespace: ingress-nginx
spec:
  provider:
    type: prometheus
    address: http://flagger-prometheus.ingress-nginx:9090
  query: |
    histogram_quantile(0.99,
      sum(
        rate(
          http_request_duration_seconds_bucket{
            kubernetes_namespace="{{ namespace }}",
            kubernetes_pod_name=~"{{ target }}-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)",
            path="root"
          }[{{ interval }}]
        )
      ) by (le)
    )
EOF

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
  ingressRef:
    apiVersion: networking.k8s.io/v1beta1
    kind: Ingress
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 80
    targetPort: http
  analysis:
    interval: 15s
    threshold: 5
    maxWeight: 40
    stepWeight: 20
    metrics:
    - name: error-rate
      templateRef:
        name: error-rate
        namespace: ingress-nginx
      thresholdRange:
        max: 1
      interval: 30s
    - name: latency
      templateRef:
        name: latency
        namespace: ingress-nginx
      thresholdRange:
        max: 0.5
      interval: 30s
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        metadata:
          type: cmd
          cmd: "hey -z 2m -q 10 -c 2 -host app.example.com http://nginx-ingress-controller.ingress-nginx"
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
        kubectl -n ingress-nginx logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary initialization test passed'

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=stefanprodan/podinfo:3.1.1

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
failed=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Failed' && failed=true || failed=false
    if ${failed}; then
        kubectl -n ingress-nginx logs deployment/flagger
        echo "Canary failed!"
        exit 1
    fi
    kubectl -n test describe deployment/podinfo-primary | grep '3.1.1' && ok=true || ok=false
    sleep 10
    kubectl -n ingress-nginx logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n ingress-nginx logs deployment/flagger
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
        kubectl -n ingress-nginx logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary promotion test passed'

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
  ingressRef:
    apiVersion: networking.k8s.io/v1beta1
    kind: Ingress
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 80
    targetPort: http
  analysis:
    interval: 15s
    threshold: 5
    iterations: 3
    match:
    - headers:
        x-user:
          exact: "insider"
    metrics:
    - name: error-rate
      templateRef:
        name: error-rate
        namespace: ingress-nginx
      thresholdRange:
        max: 1
      interval: 30s
    - name: latency
      templateRef:
        name: latency
        namespace: ingress-nginx
      thresholdRange:
        max: 0.5
      interval: 30s
    webhooks:
      - name: test-header-routing
        type: rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: bash
          cmd: "curl -sH 'x-user: insider' -H 'Host: app.example.com' http://nginx-ingress-controller.ingress-nginx | grep '3.1.2'"
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        metadata:
          type: cmd
          cmd: "hey -z 2m -q 10 -c 2 -H 'x-user: insider' -host app.example.com http://nginx-ingress-controller.ingress-nginx"
EOF

echo '>>> Triggering A/B testing'
kubectl -n test set image deployment/podinfo podinfod=stefanprodan/podinfo:3.1.2

echo '>>> Waiting for A/B testing promotion'
retries=6
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '3.1.2' && ok=true || ok=false
    sleep 30
    kubectl -n ingress-nginx logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n test describe deployment/podinfo
        kubectl -n test describe deployment/podinfo-primary
        kubectl -n ingress-nginx logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ A/B testing promotion test passed'

kubectl -n ingress-nginx logs deployment/flagger

echo '✔ All tests passed'
