# Flagger end-to-end testing

The e2e testing infrastructure is powered by CircleCI and [Kubernetes Kind](https://github.com/kubernetes-sigs/kind).

### CircleCI e2e Istio workflow

* install latest stable kubectl [e2e-kind.sh](e2e-kind.sh)
* install Kubernetes Kind [e2e-kind.sh](e2e-kind.sh)
* create local Kubernetes cluster with kind [e2e-kind.sh](e2e-kind.sh)
* install latest stable Helm CLI [e2e-istio.sh](e2e-istio.sh)
* deploy Tiller on the local cluster [e2e-istio.sh](e2e-istio.sh)
* install Istio CRDs with Helm [e2e-istio.sh](e2e-istio.sh)
* install Istio control plane and Prometheus with Helm [e2e-istio.sh](e2e-istio.sh)
* load Flagger image onto the local cluster [e2e-istio.sh](e2e-istio.sh)
* deploy Flagger in the istio-system namespace [e2e-istio.sh](e2e-istio.sh)
* create a test namespace with Istio injection enabled [e2e-tests.sh](e2e-tests.sh)
* deploy the load tester in the test namespace [e2e-tests.sh](e2e-tests.sh)
* deploy a demo workload (podinfo) in the test namespace [e2e-tests.sh](e2e-tests.sh)
* test the canary initialization [e2e-tests.sh](e2e-tests.sh)
* test the canary analysis and promotion using weighted traffic and the load testing webhook [e2e-tests.sh](e2e-tests.sh)
* test the A/B testing analysis and promotion using cookies filters and pre/post rollout webhooks [e2e-tests.sh](e2e-tests.sh)

### CircleCI e2e Linkerd workflow

* install latest stable kubectl [e2e-kind.sh](e2e-kind.sh)
* install Kubernetes Kind [e2e-kind.sh](e2e-kind.sh)
* create local Kubernetes cluster with kind [e2e-kind.sh](e2e-kind.sh)
* install Linkerd [e2e-linkerd.sh](e2e-linkerd.sh)
* load Flagger image onto the local cluster [e2e-linkerd.sh](e2e-linkerd.sh)
* deploy Flagger in the linkerd namespace with Kustomize [e2e-linkerd.sh](e2e-linkerd.sh)
* create a test namespace with Linkerd injection enabled [e2e-linkerd-tests.sh](e2e-linkerd-tests.sh)
* deploy the load tester in the test namespace [e2e-linkerd-tests.sh](e2e-linkerd-tests.sh)
* deploy a demo workload (podinfo) in the test namespace [e2e-linkerd-tests.sh](e2e-linkerd-tests.sh)
* test the canary initialization with port discovery enabled and service target port [e2e-linkerd-tests.sh](e2e-linkerd-tests.sh)
* test the canary analysis and promotion using gRPC acceptance tests and HTTP load tests [e2e-linkerd-tests.sh](e2e-linkerd-tests.sh)
* test the canary rollback on HTTP 500 errors [e2e-linkerd-tests.sh](e2e-linkerd-tests.sh)

### CircleCI e2e NGINX ingress workflow

* install latest stable kubectl [e2e-kind.sh](e2e-kind.sh)
* install Kubernetes Kind [e2e-kind.sh](e2e-kind.sh)
* create local Kubernetes cluster with kind [e2e-kind.sh](e2e-kind.sh)
* install latest stable Helm CLI [e2e-nginx.sh](e2e-nginx.sh)
* deploy Tiller on the local cluster [e2e-nginx.sh](e2e-nginx.sh)
* install NGINX ingress with Helm [e2e-nginx.sh](e2e-nginx.sh)
* load Flagger image onto the local cluster [e2e-nginx.sh](e2e-nginx.sh)
* install Flagger and Prometheus in the ingress-nginx namespace [e2e-nginx.sh](e2e-nginx.sh)
* create a test namespace [e2e-nginx-tests.sh](e2e-nginx-tests.sh)
* deploy the load tester in the test namespace [e2e-nginx-tests.sh](e2e-nginx-tests.sh)
* deploy the demo workload (podinfo) and ingress in the test namespace [e2e-nginx-tests.sh](e2e-nginx-tests.sh)
* test the canary initialization [e2e-nginx-tests.sh](e2e-nginx-tests.sh)
* test the canary analysis and promotion using weighted traffic and the load testing webhook [e2e-nginx-tests.sh](e2e-nginx-tests.sh)
* test the A/B testing analysis and promotion using header filters and pre/post rollout webhooks [e2e-nginx-tests.sh](e2e-nginx-tests.sh)
* cleanup test environment [e2e-nginx-cleanup.sh](e2e-nginx-cleanup.sh)
* install NGINX Ingress and Flagger with custom ingress annotations prefix [e2e-nginx-custom-annotations.sh](e2e-nginx-custom-annotations.sh)
* repeat the canary and A/B testing workflow [e2e-nginx-tests.sh](e2e-nginx-tests.sh)