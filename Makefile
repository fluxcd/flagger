TAG?=latest

build:
	docker build -t stefanprodan/steerer:$(TAG) . -f Dockerfile

push:
	docker push stefanprodan/steerer:$(TAG)

test:
	go test ./...

verify-codegen:
	./hack/verify-codegen.sh

helm:
	cd chart/ && helm package steerer/
	mv chart/*.tgz docs/
	helm repo index docs --url https://stefanprodan.github.io/steerer --merge ./docs/index.yaml
