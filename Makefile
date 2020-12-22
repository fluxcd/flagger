TAG?=latest
VERSION?=$(shell grep 'VERSION' pkg/version/version.go | awk '{ print $$4 }' | tr -d '"')
LT_VERSION?=$(shell grep 'VERSION' cmd/loadtester/main.go | awk '{ print $$4 }' | tr -d '"' | head -n1)

build:
	CGO_ENABLED=0 go build -a -o ./bin/flagger ./cmd/flagger

fmt:
	gofmt -l -s -w ./
	goimports -l -w ./

test-fmt:
	gofmt -l -s ./ | grep ".*\.go"; if [ "$$?" = "0" ]; then exit 1; fi
	goimports -l ./ | grep ".*\.go"; if [ "$$?" = "0" ]; then exit 1; fi

codegen:
	./hack/update-codegen.sh

test-codegen:
	./hack/verify-codegen.sh

test: test-fmt test-codegen
	go test ./...

crd:
	cat artifacts/flagger/crd.yaml > charts/flagger/crds/crd.yaml
	cat artifacts/flagger/crd.yaml > kustomize/base/flagger/crd.yaml

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

release:
	git tag "v$(VERSION)"
	git push origin "v$(VERSION)"

loadtester-build:
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ./bin/loadtester ./cmd/loadtester/*
	docker build -t ghcr.io/fluxcd/flagger-loadtester:$(LT_VERSION) . -f Dockerfile.loadtester

loadtester-push:
	docker push ghcr.io/fluxcd/flagger-loadtester:$(LT_VERSION)
