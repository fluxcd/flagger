# Flagger Development Guide

This document describes how to build, test and run Flagger from source.

### Setup dev environment

Flagger is written in Go and uses Go modules for dependency management.

On your dev machine install the following tools:
* go >= 1.13
* git >= 2.20
* bash >= 5.0
* make >= 3.81
* kubectl >= 1.16
* kustomize >= 3.5
* helm >= 3.0
* docker >= 19.03

You'll also need a Kubernetes cluster for testing Flagger.
You can use Minikube, Kind, Docker desktop or any remote cluster
(AKS/EKS/GKE/etc) Kubernetes version 1.14 or newer.

To start contributing to Flagger, fork the [repository](https://github.com/weaveworks/flagger) on GitHub.

Create a dir inside your `GOPATH`:

```bash
mkdir -p $GOPATH/src/github.com/weaveworks
```

Clone your fork:

```bash
cd $GOPATH/src/github.com/weaveworks
git clone https://github.com/YOUR_USERNAME/flagger
cd flagger
```

Set Flagger repository as upstream:

```bash
git remote add upstream https://github.com/weaveworks/flagger.git
```

Sync your fork regularly to keep it up-to-date with upstream:

```bash
git fetch upstream
git checkout master
git merge upstream/master
```

### Build

Download Go modules:

```bash
go mod download
```

Build Flagger container image:

```bash
make build
```

Run unit tests:

```bash
make test
```

If you made changes to `go.mod` run:

```bash
go mod tidy
```

If you made changes to `pkg/apis` regenerate Kubernetes client sets with:

```bash
./hack/update-codegen.sh
```

### Manual testing

Install a service mesh and/or an ingress controller on your cluster and deploy Flagger
using one of the install options [listed here](https://docs.flagger.app/install/flagger-install-on-kubernetes).

If you made changes to the CRDs, apply your local copy with:

```bash
kubectl apply -f artifacts/flagger/crd.yaml
```

Shutdown the Flagger instance installed on your cluster (replace the namespace with your mesh/ingress one):

```bash
kubectl -n istio-system scale deployment/flagger --replicas=0
```

Port forward to your Prometheus instance:

```bash
kubectl -n istio-system port-forward svc/prometheus 9090:9090
```

Run Flagger locally against your remote cluster by specifying a kubeconfig path:

```bash
go run cmd/flagger/ -kubeconfig=$HOME/.kube/config \
-log-level=info \
-mesh-provider=istio \
-metrics-server=http://localhost:9090
```

Another option to manually test your changes is to build and push the image to your container registry:

```bash
make build
docker tag weaveworks/flagger:latest <YOUR-DOCKERHUB-USERNAME>/flagger:<YOUR-TAG>
docker push <YOUR-DOCKERHUB-USERNAME>/flagger:<YOUR-TAG>
```

Deploy your image on the cluster and scale up Flagger:

```bash
kubectl -n istio-system set image deployment/flagger flagger=<YOUR-DOCKERHUB-USERNAME>/flagger:<YOUR-TAG>
kubectl -n istio-system scale deployment/flagger --replicas=1
```

Now you can use one of the [tutorials](https://docs.flagger.app/) to manually test your changes.

### Integration testing

Flagger end-to-end tests can be run locally with [Kubernetes Kind](https://github.com/kubernetes-sigs/kind).

Create a Kind cluster:

```bash
kind create cluster
```

Install a service mesh and/or an ingress controller in Kind.

Linkerd example:

```bash
linkerd install | kubectl apply -f -
linkerd check
```

Build Flagger container image and load it on the cluster:

```bash
make build
docker tag weaveworks/flagger:latest test/flagger:latest
kind load docker-image test/flagger:latest
```

Install Flagger on the cluster and set the test image:

```bash
kubectl apply -k ./kustomize/linkerd
kubectl -n linkerd set image deployment/flagger flagger=test/flagger:latest
kubectl -n linkerd rollout status deployment/flagger
```

Run the Linkerd e2e tests:

```bash
./test/e2e-linkerd-tests.sh
```

For each service mesh and ingress controller there is a dedicated e2e test suite,
chose one that matches your changes from this [list](https://github.com/weaveworks/flagger/tree/master/test).

When you open a pull request on Flagger repo, the unit and integration tests will be run in CI.

### Release

To release a new Flagger version (e.g. `2.0.0`) follow these steps:
* create a branch `git checkout -b prep-2.0.0`
* set the version in code and manifests `TAG=2.0.0 make version-set`
* commit changes and merge PR
* checkout master `git checkout master && git pull`
* tag master `make release`

After the tag has been pushed to GitHub, the CI release pipeline does the following:
* creates a GitHub release
* pushes the Flagger binary and change log to GitHub release
* pushes the Flagger container image to Docker Hub
* pushes the Helm chart to github-pages branch
* GitHub pages publishes the new chart version on the Helm repository
