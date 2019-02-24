#!/usr/bin/env bash

REPO_ROOT=$(git rev-parse --show-toplevel)

echo ">>> Building sigs.k8s.io/kind with docker"
docker build -t kind:src . -f ${REPO_ROOT}/test/Dockerfile
docker create -ti --name dummy kind:src sh
docker cp dummy:/go/bin/kind ./kind
docker rm -f dummy

echo ">>> Installing kind"
chmod +x kind && \
sudo mv kind /usr/local/bin/

echo ">>> Installing helm"
curl https://raw.githubusercontent.com/kubernetes/helm/master/scripts/get | bash

echo ">>> Installing kubectl"
curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl && \
chmod +x kubectl && \
sudo mv kubectl /usr/local/bin/
