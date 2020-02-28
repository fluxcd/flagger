TAG?=latest
VERSION?=$(shell grep 'VERSION' pkg/version/version.go | awk '{ print $$4 }' | tr -d '"')
VERSION_MINOR:=$(shell grep 'VERSION' pkg/version/version.go | awk '{ print $$4 }' | tr -d '"' | rev | cut -d'.' -f2- | rev)
PATCH:=$(shell grep 'VERSION' pkg/version/version.go | awk '{ print $$4 }' | tr -d '"' | awk -F. '{print $$NF}')
LT_VERSION?=$(shell grep 'VERSION' cmd/loadtester/main.go | awk '{ print $$4 }' | tr -d '"' | head -n1)
TS=$(shell date +%Y-%m-%d_%H-%M-%S)

run:
	GO111MODULE=on go run cmd/flagger/* -kubeconfig=$$HOME/.kube/config -log-level=info -mesh-provider=istio -namespace=test-istio \
	-metrics-server=https://prometheus.istio.flagger.dev

run-appmesh:
	GO111MODULE=on go run cmd/flagger/* -kubeconfig=$$HOME/.kube/config -log-level=info -mesh-provider=appmesh \
	-metrics-server=http://acfc235624ca911e9a94c02c4171f346-1585187926.us-west-2.elb.amazonaws.com:9090

run-nginx:
	GO111MODULE=on go run cmd/flagger/* -kubeconfig=$$HOME/.kube/config -log-level=info -mesh-provider=nginx -namespace=nginx \
	-metrics-server=http://prometheus-weave.istio.weavedx.com

run-smi:
	GO111MODULE=on go run cmd/flagger/* -kubeconfig=$$HOME/.kube/config -log-level=info -mesh-provider=smi:istio -namespace=smi \
	-metrics-server=https://prometheus.istio.weavedx.com

run-gloo:
	GO111MODULE=on go run cmd/flagger/* -kubeconfig=$$HOME/.kube/config -log-level=info -mesh-provider=gloo -namespace=gloo \
	-metrics-server=https://prometheus.istio.weavedx.com

run-nop:
	GO111MODULE=on go run cmd/flagger/* -kubeconfig=$$HOME/.kube/config -log-level=info -mesh-provider=none -namespace=bg \
	-metrics-server=https://prometheus.istio.weavedx.com

run-linkerd:
	GO111MODULE=on go run cmd/flagger/* -kubeconfig=$$HOME/.kube/config -log-level=info -mesh-provider=linkerd -namespace=dev \
	-metrics-server=https://prometheus.linkerd.flagger.dev

build:
	GIT_COMMIT=$$(git rev-list -1 HEAD) && GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build  -ldflags "-s -w -X github.com/weaveworks/flagger/pkg/version.REVISION=$${GIT_COMMIT}" -a -installsuffix cgo -o ./bin/flagger ./cmd/flagger/*
	docker build -t weaveworks/flagger:$(TAG) . -f Dockerfile

push:
	docker tag weaveworks/flagger:$(TAG) weaveworks/flagger:$(VERSION)
	docker push weaveworks/flagger:$(VERSION)

fmt:
	gofmt -l -s -w ./
	goimports -l -w ./

test-fmt:
	gofmt -l -s ./ | grep ".*\.go"; if [ "$$?" = "0" ]; then exit 1; fi
	goimports -l ./ | grep ".*\.go"; if [ "$$?" = "0" ]; then exit 1; fi

test-codegen:
	./hack/verify-codegen.sh

test: test-fmt test-codegen
	go test ./...

crd:
	cat artifacts/flagger/crd.yaml > charts/flagger/crds/crd.yaml
	cat artifacts/flagger/crd.yaml > kustomize/base/flagger/crd.yaml

helm-package:
	cd charts/ && helm package ./*
	mv charts/*.tgz bin/
	curl -s https://raw.githubusercontent.com/weaveworks/flagger/gh-pages/index.yaml > ./bin/index.yaml
	helm repo index bin --url https://flagger.app --merge ./bin/index.yaml

helm-up:
	helm upgrade --install flagger ./charts/flagger --namespace=istio-system --set crd.create=false
	helm upgrade --install flagger-grafana ./charts/grafana --namespace=istio-system

version-set:
	@next="$(TAG)" && \
	current="$(VERSION)" && \
	sed -i '' "s/$$current/$$next/g" pkg/version/version.go && \
	sed -i '' "s/flagger:$$current/flagger:$$next/g" artifacts/flagger/deployment.yaml && \
	sed -i '' "s/tag: $$current/tag: $$next/g" charts/flagger/values.yaml && \
	sed -i '' "s/appVersion: $$current/appVersion: $$next/g" charts/flagger/Chart.yaml && \
	sed -i '' "s/version: $$current/version: $$next/g" charts/flagger/Chart.yaml && \
	sed -i '' "s/newTag: $$current/newTag: $$next/g" kustomize/base/flagger/kustomization.yaml && \
	echo "Version $$next set in code, deployment, chart and kustomize"

version-up:
	@next="$(VERSION_MINOR).$$(($(PATCH) + 1))" && \
	current="$(VERSION)" && \
	sed -i '' "s/$$current/$$next/g" pkg/version/version.go && \
	sed -i '' "s/flagger:$$current/flagger:$$next/g" artifacts/flagger/deployment.yaml && \
	sed -i '' "s/tag: $$current/tag: $$next/g" charts/flagger/values.yaml && \
	sed -i '' "s/appVersion: $$current/appVersion: $$next/g" charts/flagger/Chart.yaml && \
	echo "Version $$next set in code, deployment and chart"

dev-up: version-up
	@echo "Starting build/push/deploy pipeline for $(VERSION)"
	docker build -t quay.io/stefanprodan/flagger:$(VERSION) . -f Dockerfile
	docker push quay.io/stefanprodan/flagger:$(VERSION)
	kubectl apply -f ./artifacts/flagger/crd.yaml
	helm upgrade -i flagger ./charts/flagger --namespace=istio-system --set crd.create=false

release:
	git tag $(VERSION)
	git push origin $(VERSION)

release-set: fmt version-set helm-package
	git add .
	git commit -m "Release $(VERSION)"
	git push origin master
	git tag $(VERSION)
	git push origin $(VERSION)

release-notes:
	cd /tmp && GH_REL_URL="https://github.com/buchanae/github-release-notes/releases/download/0.2.0/github-release-notes-linux-amd64-0.2.0.tar.gz" && \
    curl -sSL $${GH_REL_URL} | tar xz && sudo mv github-release-notes /usr/local/bin/

reset-test:
	kubectl delete -f ./artifacts/namespaces
	kubectl apply -f ./artifacts/namespaces
	kubectl apply -f ./artifacts/canaries

loadtester-run: loadtester-build
	docker build -t weaveworks/flagger-loadtester:$(LT_VERSION) . -f Dockerfile.loadtester
	docker rm -f tester || true
	docker run -dp 8888:9090 --name tester weaveworks/flagger-loadtester:$(LT_VERSION)

loadtester-build:
	GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ./bin/loadtester ./cmd/loadtester/*

loadtester-push:
	docker build -t weaveworks/flagger-loadtester:$(LT_VERSION) . -f Dockerfile.loadtester
	docker push weaveworks/flagger-loadtester:$(LT_VERSION)
