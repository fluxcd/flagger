#!/usr/bin/env bash

set -o errexit

REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

echo ">>> Installing Helm"
curl https://raw.githubusercontent.com/kubernetes/helm/master/scripts/get | bash

echo '>>> Installing Tiller'
kubectl --namespace kube-system create sa tiller
kubectl create clusterrolebinding tiller-cluster-rule --clusterrole=cluster-admin --serviceaccount=kube-system:tiller
helm init --service-account tiller --upgrade --wait

echo '>>> Installing NGINX Ingress'
helm upgrade -i nginx-ingress stable/nginx-ingress \
--wait \
--namespace ingress-nginx \
--set controller.stats.enabled=true \
--set controller.metrics.enabled=true \
--set controller.podAnnotations."prometheus\.io/scrape"=true \
--set controller.podAnnotations."prometheus\.io/port"=10254 \
--set controller.service.type=NodePort

kubectl -n ingress-nginx rollout status deployment/nginx-ingress-controller
kubectl -n ingress-nginx get all


