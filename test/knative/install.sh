#!/usr/bin/env bash

set -o errexit

KNATIVE_VER="1.17.0"
REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

echo '>>> Installing Knative'
kubectl apply -f https://github.com/knative/serving/releases/download/knative-v${KNATIVE_VER}/serving-crds.yaml
kubectl apply -f https://github.com/knative/serving/releases/download/knative-v${KNATIVE_VER}/serving-core.yaml
kubectl apply -f https://github.com/knative/net-kourier/releases/download/knative-v${KNATIVE_VER}/kourier.yaml
kubectl patch configmap/config-network \
  --namespace knative-serving \
  --type merge \
  --patch '{"data":{"ingress-class":"kourier.ingress.networking.knative.dev"}}'

kubectl -n knative-serving rollout status deployment
kubectl -n kourier-system rollout status deployment

echo '>>> Installing Flagger'
kubectl apply -k ${REPO_ROOT}/kustomize/knative

kubectl -n flagger-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n flagger-system rollout status deployment/flagger
