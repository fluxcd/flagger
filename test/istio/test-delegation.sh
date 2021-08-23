#!/usr/bin/env bash

# This script runs e2e tests for when the canary delegation is enabled
# Prerequisites: Kubernetes Kind and Istio

set -o errexit

#echo '>>> Set pilot env to enable virtual service delegate'
#kubectl -n istio-system set env deploy istiod PILOT_ENABLE_VIRTUAL_SERVICE_DELEGATE=true
#kubectl -n istio-system rollout status deploy istiod

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

echo '>>> Initialising root virtual service'
cat <<EOF | kubectl apply -f -
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: root-vs
  namespace: test
spec:
  gateways:
    - istio-system/istio-ingressgateway
  hosts:
    - "*"
  http:
    - match:
        - uri:
            prefix: "/podinfo"
      rewrite:
        uri: "/"
      delegate:
        name: podinfo
        namespace: test
EOF

echo '>>> Initialising canary for delegate'
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
    portDiscovery: true
    delegation: true
  analysis:
    interval: 15s
    threshold: 15
    maxWeight: 30
    stepWeight: 10
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

echo '>>> Triggering canary deployment'
kubectl -n test set image deployment/podinfo podinfod=stefanprodan/podinfo:3.1.1

echo '>>> Waiting for canary promotion'
retries=50
count=0
ok=false
until ${ok}; do
    kubectl -n test describe deployment/podinfo-primary | grep '3.1.1' && ok=true || ok=false
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
