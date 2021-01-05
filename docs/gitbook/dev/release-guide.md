# Release Guide

This document describes how to release Flagger.

## Release

To release a new Flagger version (e.g. `2.0.0`) follow these steps:

* create a branch `git checkout -b prep-2.0.0`
* set the version in code and manifests `TAG=2.0.0 make version-set`
* commit changes and merge PR
* checkout master `git checkout main && git pull`
* tag master `make release`

## CI

After the tag has been pushed to GitHub, the CI release pipeline does the following:

* creates a GitHub release
* pushes the Flagger binary and change log to GitHub release
* pushes the Flagger container image to Docker Hub
* pushes the Helm chart to github-pages branch
* GitHub pages publishes the new chart version on the Helm repository

## Docs

The documentation [website](https://docs.flagger.app) is built from the `docs` branch.

After a Flagger release, publish the docs with:

* `git checkout main && git pull`
* `git checkout docs`
* `git rebase main`
* `git push origin docs`
