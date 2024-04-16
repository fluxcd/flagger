# Release Guide

This document describes how to release Flagger.

## Release

### Flagger 

To release a new Flagger version (e.g. `2.0.0`) follow these steps:

* create a branch `git checkout -b release-2.0.0`
* set the version in code and manifests `TAG=2.0.0 make version-set`
  * (if running from Mac OS, modify the makefile -> `sed -i '' 's/foo/bar/' file  # Note the '' as a *separate argument*`)
* commit changes and merge PR
* checkout main `git checkout main && git pull`
* tag main `make release`

### Flagger load tester

To release a new Flagger load tester version (e.g. `2.0.0`) follow these steps:

* create a branch `git checkout -b release-ld-2.0.0`
* set the version in code (`cmd/loadtester/main.go#VERSION`)
* set the version in the Helm chart (`charts/loadtester/Chart.yaml` and `values.yaml`)
* set the version in manifests (`kustomize/tester/deployment.yaml`)
* commit changes and push the branch upstream
* in GitHub UI, navigate to Actions and run the `push-ld` workflow selecting the release branch
* after the workflow finishes, open the PR which will run the e2e tests using the new tester version
* merge the PR if the tests pass

## CI

After the tag has been pushed to GitHub, the CI release pipeline does the following:

* creates a GitHub release
* pushes the Flagger binary and change log to GitHub release
* pushes the Flagger container image to GitHub Container Registry
* pushed the Flagger install manifests to GitHub Container Registry
* signs all OCI artifacts and release assets with Cosign and GitHub OIDC
* pushes the Helm chart to github-pages branch
* GitHub pages publishes the new chart version on the Helm repository

## Docs

The documentation [website](https://docs.flagger.app) is built from the `docs` branch.

After a Flagger release, publish the docs with:

* `git checkout main && git pull`
* `git checkout docs`
* `git rebase main`
* `git push origin docs`

Lastly open a PR with all the docs changes on [fluxcd/website](https://github.com/fluxcd/website) to 
update [fluxcd.io/flagger](https://fluxcd.io/flagger/).
