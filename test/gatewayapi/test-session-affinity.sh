#!/usr/bin/env bash

# This script runs e2e tests for progressive traffic shifting with session affinity, Canary analysis and promotion
# Prerequisites: Kubernetes Kind and Contour with GatewayAPI

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

source ${REPO_ROOT}/test/gatewayapi/test-utils.sh

create_latency_metric_template
create_error_rate_metric_template

echo '>>> Deploy podinfo in sa-test namespace'
kubectl create ns sa-test
kubectl apply -f ${REPO_ROOT}/test/workloads/secret.yaml -n sa-test
kubectl apply -f ${REPO_ROOT}/test/workloads/deployment.yaml -n sa-test

echo '>>> Installing Canary'
cat <<EOF | kubectl apply -f -
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: podinfo
  namespace: sa-test
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: podinfo
  progressDeadlineSeconds: 60
  service:
    port: 9898
    portName: http
    hosts:
     - localproject.contour.io
    gatewayRefs:
      - name: contour
        namespace: projectcontour
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 50
    stepWeight: 10
    sessionAffinity:
      cookieName: flagger-cookie
    metrics:
      - name: error-rate
        templateRef:
          name: error-rate
          namespace: flagger-system
        thresholdRange:
          max: 1
        interval: 1m
      - name: latency
        templateRef:
          name: latency
          namespace: flagger-system
        thresholdRange:
          max: 0.5
        interval: 30s
    webhooks:
      - name: load-test
        type: rollout
        url: http://flagger-loadtester.test/
        timeout: 5s
        metadata:
          cmd: "hey -z 2m -q 10 -c 2 -host localproject.contour.io http://envoy-contour.projectcontour/"
          logCmdOutput: "true"
EOF

check_primary "sa-test"

display_httproute "sa-test"

echo '>>> Port forwarding load balancer'
kubectl port-forward -n projectcontour svc/envoy-contour 8888:80 2>&1 > /dev/null &
pf_pid=$!

cleanup() {
    echo ">> Killing port forward process ${pf_pid}"
    kill -9 $pf_pid
}
trap "cleanup" EXIT SIGINT

echo '>>> Triggering canary deployment'
kubectl -n sa-test set image deployment/podinfo podinfod=stefanprodan/podinfo:6.1.0

echo '>>> Waiting for initial traffic shift'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n sa-test get canary podinfo -o=jsonpath='{.status.canaryWeight}' | grep '10' && ok=true || ok=false
    sleep 5
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '>>> Verifying session affinity'
if ! URL=http://localhost:8888 HOST=localproject.contour.io VERSION=6.1.0 COOKIE_NAME=flagger-cookie \
    go run ${REPO_ROOT}/test/gatewayapi/verify_session_affinity.go; then
    echo "failed to verify session affinity"
    exit $?
fi

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n sa-test describe deployment/podinfo-primary | grep '6.1.0' && ok=true || ok=false
    sleep 10
    kubectl -n flagger-system logs deployment/flagger --tail 1
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

display_httproute "sa-test"

echo '>>> Waiting for canary finalization'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n sa-test get canary/podinfo | grep 'Succeeded' && ok=true || ok=false
    sleep 5
    count=$(($count + 1))
    if [[ ${count} -eq ${retries} ]]; then
        kubectl -n flagger-system logs deployment/flagger
        echo "No more retries left"
        exit 1
    fi
done

echo '>>> Verifying cookie cleanup'
canary_cookie=$(kubectl -n sa-test get canary podinfo -o=jsonpath='{.status.previousSessionAffinityCookie}' | xargs)
response=$(curl -H "Host: localproject.contour.io" -H "Cookie: $canary_cookie" -D - http://localhost:8888)

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

kubectl delete -n sa-test canary podinfo
