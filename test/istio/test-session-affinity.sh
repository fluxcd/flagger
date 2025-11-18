#!/usr/bin/env bash

# This script runs e2e tests for progressive traffic shifting with session affinity, Canary analysis and promotion
# Prerequisites: Kubernetes Kind and Istio

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Initialising Gateway'
cat <<EOF | kubectl apply -f -
apiVersion: networking.istio.io/v1alpha3
kind: Gateway
metadata:
  name: istio-ingressgateway
  namespace: istio-system
spec:
  selector:
    app: istio-ingressgateway
    istio: ingressgateway
  servers:
  - port:
      number: 80
      name: http
      protocol: HTTP
    hosts:
      - "*"
EOF

echo '>>> Initialising canary for session affinity'
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
    port: 80
    targetPort: 9898
    gateways:
      - istio-system/istio-ingressgateway
    hosts:
      - "*"
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 30
    stepWeight: 10
    sessionAffinity:
      cookieName: canary-flagger-cookie
      primaryCookieName: primary-flagger-cookie
    webhooks:
      - name: load-test
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          type: cmd
          cmd: "hey -z 10m -q 10 -c 2 http://istio-ingressgateway.istio-system/podinfo"
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

echo '>>> Port forwarding load balancer'
INGRESS_GATEWAY_POD=$(kubectl get pods -n istio-system -l app=istio-ingressgateway -o name | awk -F'/' '{print $2}')
kubectl port-forward -n istio-system "$INGRESS_GATEWAY_POD" 8080:8080 2>&1 > /dev/null &
pf_pid=$!

cleanup() {
    echo ">> Killing port forward process ${pf_pid}"
    kill -9 $pf_pid
}
trap "cleanup" EXIT SIGINT

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=ghcr.io/stefanprodan/podinfo:6.0.1

echo '>>> Waiting for initial traffic shift'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test get canary podinfo -o=jsonpath='{.status.canaryWeight}' | grep '10' && ok=true || ok=false
    sleep 5
    kubectl -n istio-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n istio-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '>>> Verifying session affinity'
if ! URL=http://localhost:8080 HOST=localhost CANARY_VERSION=6.0.1 \
  CANARY_COOKIE_NAME=canary-flagger-cookie PRIMARY_VERSION=6.0.0 PRIMARY_COOKIE_NAME=primary-flagger-cookie \
    go run ${REPO_ROOT}/test/verify_session_affinity.go; then
    echo "failed to verify session affinity"
    exit $?
fi

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

echo '>>> Verifying cookie cleanup'
canary_cookie=$(kubectl -n test get canary podinfo -o=jsonpath='{.status.previousSessionAffinityCookie}' | xargs)
response=$(curl -H "Cookie: $canary_cookie" -D - http://localhost:8080)

if [[ $response == *"$canary_cookie"* ]]; then
  echo "✔ Found previous cookie in response"
else
  echo "⨯ Previous cookie ${canary_cookie} not found in response"
  exit 1
fi

if [[ $response == *"Max-Age=-1"* ]]; then
  echo "✔ Found Max-Age attribute in cookie"
else
  echo "⨯ Max-Age attribute not present in cookie"
  exit 1
fi

echo '✔ Canary release with session affinity promotion test passed'

kubectl delete -n test canary podinfo
