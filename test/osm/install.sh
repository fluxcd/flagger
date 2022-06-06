#!/usr/bin/env bash

set -o errexit

OSM_VER="v1.1.1"
REPO_ROOT=$(git rev-parse --show-toplevel)

mkdir -p ${REPO_ROOT}/bin

cd ${REPO_ROOT}/bin && curl -sL "https://github.com/openservicemesh/osm/releases/download/${OSM_VER}/osm-${OSM_VER}-linux-amd64.tar.gz" | tar xz
chmod +x ${REPO_ROOT}/bin/linux-amd64/osm

echo ">>> Installing Open Service Mesh ${OSM_VER}"
${REPO_ROOT}/bin/linux-amd64/osm install \
--set=osm.deployPrometheus=true \
--set=osm.enablePermissiveTrafficPolicy=true \
--set=osm.osmController.resource.limits.cpu=250m \
--set=osm.osmController.resource.requests.cpu=250m \
--set=osm.prometheus.resources.limits.cpu=250m \
--set=osm.prometheus.resources.requests.cpu=250m \
--set=osm.injector.resource.limits.cpu=250m \
--set=osm.injector.resource.requests.cpu=250m \
--set=osm.osmBootstrap.resource.limits.cpu=250m \
--set=osm.osmBootstrap.resource.requests.cpu=250m

${REPO_ROOT}/bin/linux-amd64/osm version

echo '>>> Installing Flagger'
kubectl apply -k ${REPO_ROOT}/kustomize/osm

kubectl -n osm-system set image deployment/flagger flagger=test/flagger:latest
kubectl -n osm-system rollout status deployment/flagger
