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
    apiVersion: extensions/v1beta1
    kind: Ingress
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 80
    targetPort: http
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 30
    stepWeight: 10
    metrics:
    - name: "http-request-success-rate"
      threshold: 99
      interval: 1m
      query: |
        100 - sum(
                rate(
                    http_request_duration_seconds_count{
                        kubernetes_namespace="test",
                        kubernetes_pod_name=~"podinfo-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)",
                        path="root",
                        status!~"5.*"
                    }[1m]
                )
            )
            /
            sum(
                rate(
                    http_request_duration_seconds_count{
                        kubernetes_namespace="test",
                        kubernetes_pod_name=~"podinfo-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)",
                        path="root"
                    }[1m]
                )
            )
            * 100
    - name: "latency"
      threshold: 0.5
      interval: 1m
      query: |
        histogram_quantile(0.99,
          sum(
            rate(
              http_request_duration_seconds_bucket{
                kubernetes_namespace="test",
                kubernetes_pod_name=~"podinfo-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)",
                path="root"
              }[1m]
            )
          ) by (le)
        )
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 -host app.example.com http://nginx-ingress-controller.ingress-nginx"
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
    apiVersion: extensions/v1beta1
    kind: Ingress
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 80
    targetPort: 9898
  analysis:
    interval: 10s
    threshold: 5
    iterations: 5
    match:
      - headers:
          x-canary:
            exact: "insider"
      - headers:
          cookie:
            exact: "canary"
    metrics:
    - name: "http-request-success-rate"
      threshold: 99
      interval: 1m
      query: |
        100 - sum(
                rate(
                    http_request_duration_seconds_count{
                        kubernetes_namespace="test",
                        kubernetes_pod_name=~"podinfo-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)",
                        path="root",
                        status!~"5.*"
                    }[1m]
                )
            )
            /
            sum(
                rate(
                    http_request_duration_seconds_count{
                        kubernetes_namespace="test",
                        kubernetes_pod_name=~"podinfo-[0-9a-zA-Z]+(-[0-9a-zA-Z]+)",
                        path="root"
                    }[1m]
                )
            )
            * 100
    webhooks:
      - name: pre
        type: pre-rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 -H 'X-Canary: insider' -host app.example.com http://nginx-ingress-controller.ingress-nginx"
          logCmdOutput: "true"
      - name: post
        type: post-rollout
        url: http://flagger-loadtester.test/
        timeout: 15s
        metadata:
          type: cmd
          cmd: "curl -sH 'Host: app.example.com' http://nginx-ingress-controller.ingress-nginx"
          logCmdOutput: "true"
EOF

echo '>>> Triggering A/B testing'
kubectl -n test set image deployment/podinfo podinfod=stefanprodan/podinfo:3.1.2

echo '>>> Waiting for A/B testing promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '3.1.2' && ok=true || ok=false
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

echo '✔ A/B testing promotion test passed'

kubectl -n ingress-nginx logs deployment/flagger

echo '✔ All tests passed'