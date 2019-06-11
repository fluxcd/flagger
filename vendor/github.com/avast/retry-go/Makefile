SOURCE_FILES?=$$(go list ./... | grep -v /vendor/)
TEST_PATTERN?=.
TEST_OPTIONS?=
DEP?=$$(which dep)
VERSION?=$$(cat VERSION)

ifeq ($(OS),Windows_NT)
	DEP_VERS=dep-windows-amd64
else
	DEP_VERS=dep-linux-amd64
endif

setup: ## Install all the build and lint dependencies
	# fix of gopkg.in issue (https://github.com/niemeyer/gopkg/issues/50)
	git config --global http.https://gopkg.in.followRedirects true
	go get -u gopkg.in/alecthomas/gometalinter.v1
	go get -u github.com/pierrre/gotestcover
	go get -u golang.org/x/tools/cmd/cover
	go get -u github.com/robertkrimen/godocdown/godocdown
	gometalinter.v1 --install
	@if [ "$(DEP)" = "" ]; then\
		curl -L https://github.com/golang/dep/releases/download/v0.3.1/$(DEP_VERS) >| $$GOPATH/bin/dep;\
		chmod +x $$GOPATH/bin/dep;\
	fi
	dep ensure

generate: ## Generate README.md
	godocdown >| README.md

test: generate ## Run all the tests
	gotestcover $(TEST_OPTIONS) -covermode=atomic -coverprofile=coverage.txt $(SOURCE_FILES) -run $(TEST_PATTERN) -timeout=2m

cover: test ## Run all the tests and opens the coverage report
	go tool cover -html=coverage.txt

fmt: ## gofmt and goimports all go files
	find . -name '*.go' -not -wholename './vendor/*' | while read -r file; do gofmt -w -s "$$file"; goimports -w "$$file"; done

lint: ## Run all the linters
	gometalinter.v1 --vendor --disable-all \
		--enable=deadcode \
		--enable=ineffassign \
		--enable=gosimple \
		--enable=staticcheck \
		--enable=gofmt \
		--enable=goimports \
		--enable=dupl \
		--enable=misspell \
		--enable=errcheck \
		--enable=vet \
		--deadline=10m \
		./...

ci: test lint  ## Run all the tests and code checks

build:
	go build

release: ## Release new version
	git tag | grep -q $(VERSION) && echo This version was released! Increase VERSION! || git tag $(VERSION) && git push origin $(VERSION) && git tag v$(VERSION) && git push origin v$(VERSION)

# Absolutely awesome: http://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := build
