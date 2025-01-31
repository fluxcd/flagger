#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)

echo '>>> Delete test namespace'
kubectl delete namespace test --ignore-not-found=true --wait=true

echo '>>> Creating test namespace'
kubectl create namespace test

echo '>>> Installing the load tester'
kubectl apply -k ${REPO_ROOT}/kustomize/tester
kubectl -n test rollout status deployment/flagger-loadtester

echo '>>> Deploy Knative Service'
cat <<EOF | kubectl apply -f -
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: podinfo
  namespace: test
spec:
  template:
    spec:
      containers:
        - image: ghcr.io/stefanprodan/podinfo:6.0.0
          ports:
            - containerPort: 9898
              protocol: TCP
          command:
            - ./podinfo
            - --port=9898
            - --port-metrics=9797
            - --grpc-port=9999
            - --grpc-service-name=podinfo
            - --level=info
            - --random-delay=false
            - --random-error=false
EOF
