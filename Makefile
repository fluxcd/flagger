TAG?=latest
VERSION?=$(shell grep 'VERSION' pkg/version/version.go | awk '{ print $$4 }' | tr -d '"')
VERSION_MINOR:=$(shell grep 'VERSION' pkg/version/version.go | awk '{ print $$4 }' | tr -d '"' | rev | cut -d'.' -f2- | rev)
PATCH:=$(shell grep 'VERSION' pkg/version/version.go | awk '{ print $$4 }' | tr -d '"' | awk -F. '{print $$NF}')


build:
	docker build -t stefanprodan/steerer:$(TAG) . -f Dockerfile

push:
	docker push stefanprodan/steerer:$(TAG)

test:
	go test ./...

verify-codegen:
	./hack/verify-codegen.sh

helm-package:
	cd chart/ && helm package steerer/
	mv chart/*.tgz docs/
	helm repo index docs --url https://stefanprodan.github.io/steerer --merge ./docs/index.yaml

helm-up:
	helm upgrade --install steerer ./chart/steerer --namespace=istio-system

version-set:
	@next="$(TAG)" && \
	current="$(VERSION)" && \
	sed -i '' "s/$$current/$$next/g" pkg/version/version.go && \
	sed -i '' "s/steerer:$$current/steerer:$$next/g" artifacts/steerer/deployment.yaml && \
	sed -i '' "s/tag: $$current/tag: $$next/g" chart/steerer/values.yaml && \
	sed -i '' "s/appVersion: $$current/appVersion: $$next/g" chart/steerer/Chart.yaml && \
	echo "Version $$next set in code, deployment and chart"

version-up:
	@next="$(VERSION_MINOR).$$(($(PATCH) + 1))" && \
	current="$(VERSION)" && \
	sed -i '' "s/$$current/$$next/g" pkg/version/version.go && \
	sed -i '' "s/steerer:$$current/steerer:$$next/g" artifacts/steerer/deployment.yaml && \
	sed -i '' "s/tag: $$current/tag: $$next/g" chart/steerer/values.yaml && \
	sed -i '' "s/appVersion: $$current/appVersion: $$next/g" chart/steerer/Chart.yaml && \
	echo "Version $$next set in code, deployment and chart"

dev-up: version-up
	@echo "Starting build/push/deploy pipeline for $(VERSION)"
	docker build -t stefanprodan/steerer:$(VERSION) . -f Dockerfile
	docker push stefanprodan/steerer:$(VERSION)
	helm upgrade --install steerer ./chart/steerer --namespace=istio-system


