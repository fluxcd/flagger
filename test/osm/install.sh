#!/usr/bin/env bash

set -o errexit

OSM_VER="v0.9.1"
REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

cd ${REPO_ROOT}/bin && curl -sL "https://github.com/openservicemesh/osm/releases/download/${OSM_VER}/osm-${OSM_VER}-linux-amd64.tar.gz" | tar xz
chmod +x ${REPO_ROOT}/bin/linux-amd64/osm

echo ">>> Installing Open Service Mesh ${OSM_VER}"
${REPO_ROOT}/bin/linux-amd64/osm install \
--set=OpenServiceMesh.deployPrometheus=true \
--set=OpenServiceMesh.enablePermissiveTrafficPolicy=true \
--set=OpenServiceMesh.osmController.resource.limits.cpu=300m \
--set=OpenServiceMesh.osmController.resource.requests.cpu=300m \
--set=OpenServiceMesh.prometheus.resources.limits.cpu=300m \
--set=OpenServiceMesh.prometheus.resources.requests.cpu=300m \
--set=OpenServiceMesh.injector.resource.limits.cpu=300m \
--set=OpenServiceMesh.injector.resource.requests.cpu=300m

${REPO_ROOT}/bin/linux-amd64/osm version

echo '>>> Installing Flagger'
kubectl apply -k ${REPO_ROOT}/kustomize/osm

kubectl -n osm-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n osm-system rollout status deployment/flagger
