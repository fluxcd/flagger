# Development Guide

This document describes how to build, test and run Flagger from source.

## Setup dev environment

Flagger is written in Go and uses Go modules for dependency management.

On your dev machine install the following tools:

* go &gt;= 1.14
* git &gt;= 2.20
* bash &gt;= 5.0
* make &gt;= 3.81
* kubectl &gt;= 1.16
* kustomize &gt;= 3.5
* helm &gt;= 3.0
* docker &gt;= 19.03

You'll also need a Kubernetes cluster for testing Flagger.
You can use Minikube, Kind, Docker desktop or any remote cluster (AKS/EKS/GKE/etc) Kubernetes version 1.16 or newer.

To start contributing to Flagger, fork the [repository](https://github.com/fluxcd/flagger) on GitHub.

Create a dir inside your `GOPATH`:

```bash
mkdir -p $GOPATH/src/github.com/fluxcd
```

Clone your fork:

```bash
cd $GOPATH/src/github.com/fluxcd
git clone https://github.com/YOUR_USERNAME/flagger
cd flagger
```

Set Flagger repository as upstream:

```bash
git remote add upstream https://github.com/fluxcd/flagger.git
```

Sync your fork regularly to keep it up-to-date with upstream:

```bash
git fetch upstream
git checkout main
git merge upstream/main
```

## Build

Download Go modules:

```bash
go mod download
```

Build Flagger binary:

```bash
make build
```

Build load tester binary:

```bash
make loadtester-build
```

## Code changes

We require all commits to be signed. By signing off with your signature, you
certify that you wrote the patch or otherwise have the right to contribute the
material by the rules of the [DCO](https://raw.githubusercontent.com/fluxcd/flagger/main/DCO).

If your `user.name` and `user.email` are configured in your Git config,
you can sign your commit automatically with:

```bash
git commit -s
```

Before submitting a PR, make sure your changes are covered by unit tests.

If you made changes to `go.mod` run:

```bash
go mod tidy
```

If you made changes to `pkg/apis` regenerate Kubernetes client sets with:

```bash
make codegen
```

Run code formatters:

```bash
make fmt
```

Run unit tests:

```bash
make test
```

## API changes

If you made changes to `pkg/apis` regenerate the Kubernetes client sets with:

```bash
make codegen
```

Update the validation spec in `artifacts/flagger/crd.yaml` and run:

```bash
make crd
```

Note that any change to the CRDs must be accompanied by an update to the Open API schema.

## Manual testing

Install a service mesh and/or an ingress controller on your cluster
and deploy Flagger using one of the install options
[listed here](https://docs.flagger.app/install/flagger-install-on-kubernetes).

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
docker build -t <YOUR-DOCKERHUB-USERNAME>/flagger:<YOUR-TAG> .
docker push <YOUR-DOCKERHUB-USERNAME>/flagger:<YOUR-TAG>
```

Deploy your image on the cluster and scale up Flagger:

```bash
kubectl -n istio-system set image deployment/flagger flagger=<YOUR-DOCKERHUB-USERNAME>/flagger:<YOUR-TAG>
kubectl -n istio-system scale deployment/flagger --replicas=1
```

Now you can use one of the [tutorials](https://docs.flagger.app/) to manually test your changes.

## Integration testing

Flagger end-to-end tests can be run locally with [Kubernetes Kind](https://github.com/kubernetes-sigs/kind).

Create a Kind cluster:

```bash
kind create cluster
```

Build Flagger container image and load it on the cluster:

```bash
make build
docker build -t test/flagger:latest .
kind load docker-image test/flagger:latest
```


Run the Istio e2e tests:

```bash
./test/istio/run.sh
```

For each service mesh and ingress controller, there is a dedicated e2e test suite,
choose one that matches your changes from this [list](https://github.com/fluxcd/flagger/tree/main/test).

When you open a pull request on Flagger repo, the unit and integration tests will be run in CI.
