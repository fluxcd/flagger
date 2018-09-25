TAG?=latest

build:
	docker build -t stefanprodan/steerer:$(TAG) . -f Dockerfile

push:
	docker push stefanprodan/steerer:$(TAG)

test:
	go test ./...

verify-codegen:
	./hack/verify-codegen.sh
