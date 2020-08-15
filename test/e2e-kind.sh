#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
HELM_VERSION=v3.3.0
KUSTOMIZE_VERSION=3.8.1
KIND_VERSION=v0.8.1
KUBE_VERSION=v1.16.9

if [[ "$1" ]]; then
  KUBE_VERSION=$1
fi

echo ">>> Installing kubectl"
curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && \
chmod +x kubectl && \
sudo mv kubectl /usr/local/bin/

echo ">>> Installing kind"
curl -sSLo kind "https://github.com/kubernetes-sigs/kind/releases/download/$KIND_VERSION/kind-linux-amd64"
chmod +x kind
sudo mv kind /usr/local/bin/kind

echo ">>> Creating kind cluster"
kind create cluster --wait 5m --image kindest/node:${KUBE_VERSION}

kubectl get pods --all-namespaces

cd /tmp

echo ">>> Installing Helm"
curl -sSL https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz | tar xz && sudo mv linux-amd64/helm /usr/local/bin/ && rm -rf linux-amd64
helm repo add stable https://kubernetes-charts.storage.googleapis.com/

echo ">>> Installing Kustomize"
kustomize_url=https://github.com/kubernetes-sigs/kustomize/releases/download && \
curl -sL ${kustomize_url}/kustomize%2Fv${KUSTOMIZE_VERSION}/kustomize_v${KUSTOMIZE_VERSION}_linux_amd64.tar.gz | tar xz
chmod +x kustomize
sudo mv kustomize /usr/local/bin/kustomize
