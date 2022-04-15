#!/usr/bin/env bash

# This script runs e2e tests for Canary initialization, analysis and promotion
# Prerequisites: Kubernetes Kind, Helm and NGINX ingress controller

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

cat <<EOF | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: podinfo
  namespace: test
  labels:
    app: podinfo
  annotations:
    kubernetes.io/ingress.class: "nginx"
spec:
  rules:
  - host: "app.example.com"
    http:
      paths:
      - pathType: Prefix
        path: "/"
        backend:
          service:
            name: podinfo
            port:
              number: 80
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
    apiVersion: networking.k8s.io/v1
    kind: Ingress
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 80
    targetPort: http
  analysis:
    interval: 10s
    threshold: 2
    maxWeight: 40
    stepWeight: 20
    metrics:
    - name: request-success-rate
      thresholdRange:
        min: 1
      interval: 30s
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        metadata:
          type: cmd
          cmd: "hey -z 2m -q 10 -c 2 -host app.example.com http://ingress-nginx-controller.ingress-nginx/status/500"
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
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.1

echo '>>> Waiting for canary rollback'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Failed' && ok=true || ok=false
    sleep 10
    kubectl -n ingress-nginx logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n ingress-nginx logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '✔ Canary rollback test passed'

pod_hash=$(kubectl get pods -l app=podinfo-primary -n test -o=jsonpath='{.items[0].metadata.labels.pod-template-hash}')

echo '>>> Reverting canary deployment to match primary'
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.0

sleep 15

new_pod_hash=$(kubectl get pods -l app=podinfo-primary -n test -o=jsonpath='{.items[0].metadata.labels.pod-template-hash}')
failed=false
kubectl -n test get canary/podinfo | grep 'Failed' && failed=true || ok=false

if [ "$new_pod_hash" = "$pod_hash" -a "$failed" = true ]; then
    echo '✔ Canary not triggered upon reverting canary image to match primary '
else
    echo '⨯ Canary got triggered upon reverting canary image to match primary'
    exit 1
fi
 
echo '>>> Triggering canary deployment again'
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.1

echo '>>> Waiting for canary to start progress'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Progressing' && ok=true || ok=false
    sleep 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n ingress-nginx logs deployment/flagger
        kubectl -n test get httpproxy podinfo -oyaml
        echo "No more retries left"
        exit 1
    fi
done

echo '>>> Waiting for canary rollback'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary/podinfo | grep 'Failed' && ok=true || ok=false
    sleep 10
    kubectl -n ingress-nginx logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n ingress-nginx logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

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
    apiVersion: networking.k8s.io/v1
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
    - name: request-success-rate
      thresholdRange:
        min: 1
      interval: 30s
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        metadata:
          type: cmd
          cmd: "hey -z 2m -q 10 -c 2 -host app.example.com http://ingress-nginx-controller.ingress-nginx/"
EOF

echo '>>> Retrying failed canary run'
kubectl -n test patch deploy/podinfo -p '[{"op": "add", "path":"/spec/template/metadata/annotations", "value": {"thisis": "theway"}}]' --type=json

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '6.0.1' && ok=true || ok=false
    sleep 10
    kubectl -n ingress-nginx logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
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
