TAG?=latest
VERSION?=$(shell grep 'VERSION' pkg/version/version.go | awk '{ print $$4 }' | tr -d '"')
VERSION_MINOR:=$(shell grep 'VERSION' pkg/version/version.go | awk '{ print $$4 }' | tr -d '"' | rev | cut -d'.' -f2- | rev)
PATCH:=$(shell grep 'VERSION' pkg/version/version.go | awk '{ print $$4 }' | tr -d '"' | awk -F. '{print $$NF}')
SOURCE_DIRS = cmd pkg/apis pkg/controller pkg/server pkg/logging pkg/version
LT_VERSION?=$(shell grep 'VERSION' cmd/loadtester/main.go | awk '{ print $$4 }' | tr -d '"' | head -n1)

run:
	go run cmd/flagger/* -kubeconfig=$$HOME/.kube/config -log-level=info \
	-metrics-server=https://prometheus.istio.weavedx.com \
	-slack-url=https://hooks.slack.com/services/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 \
	-slack-channel="devops-alerts"

build:
	docker build -t stefanprodan/flagger:$(TAG) . -f Dockerfile

push:
	docker tag stefanprodan/flagger:$(TAG) quay.io/stefanprodan/flagger:$(VERSION)
	docker push quay.io/stefanprodan/flagger:$(VERSION)

fmt:
	gofmt -l -s -w $(SOURCE_DIRS)

test-fmt:
	gofmt -l -s $(SOURCE_DIRS) | grep ".*\.go"; if [ "$$?" = "0" ]; then exit 1; fi

test-codegen:
	./hack/verify-codegen.sh

test: test-fmt test-codegen
	go test ./...

helm-package:
	cd charts/ && helm package ./*
	mv charts/*.tgz docs/
	helm repo index docs --url https://stefanprodan.github.io/flagger --merge ./docs/index.yaml

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
	echo "Version $$next set in code, deployment and charts"

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

reset-test:
	kubectl delete -f ./artifacts/namespaces
	kubectl apply -f ./artifacts/namespaces
	kubectl apply -f ./artifacts/canaries

loadtester-push:
	docker build -t quay.io/stefanprodan/flagger-loadtester:$(LT_VERSION) . -f Dockerfile.loadtester
	docker push quay.io/stefanprodan/flagger-loadtester:$(LT_VERSION)