# Flagger end-to-end testing

The e2e testing infrastructure is powered by GitHub Actions and [Kubernetes Kind](https://github.com/kubernetes-sigs/kind).

### e2e workflow

* create local Kubernetes cluster with KinD
* build Flagger container and load the image in KinD
* install the service mesh or ingress provider
* deploy Flagger
* create test namespace, workloads and load tester
* test the canary initialization (port discovery and metadata)
* test the canary release (progressive traffic shifting, headers routing, mirroring, analysis, promotion, rollback)
* test webhooks (conformance, load testing, pre/post rollout)
