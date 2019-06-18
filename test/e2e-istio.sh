#!/usr/bin/env bash

set -o errexit

ISTIO_VER="1.1.9"
REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo ">>> Installing Helm"
curl https://raw.githubusercontent.com/kubernetes/helm/master/scripts/get | bash

echo '>>> Installing Tiller'
kubectl --namespace kube-system create sa tiller
kubectl create clusterrolebinding tiller-cluster-rule --clusterrole=cluster-admin --serviceaccount=kube-system:tiller
helm init --service-account tiller --upgrade --wait

echo ">>> Installing Istio ${ISTIO_VER}"
helm repo add istio.io https://storage.googleapis.com/istio-release/releases/${ISTIO_VER}/charts

echo '>>> Installing Istio CRDs'
helm upgrade -i istio-init istio.io/istio-init --wait --namespace istio-system

echo '>>> Waiting for Istio CRDs to be ready'
kubectl -n istio-system wait --for=condition=complete job/istio-init-crd-10
kubectl -n istio-system wait --for=condition=complete job/istio-init-crd-11

echo '>>> Installing Istio control plane'
helm upgrade -i istio istio.io/istio --wait --namespace istio-system -f ${REPO_ROOT}/test/e2e-istio-values.yaml

kubectl -n istio-system get all
