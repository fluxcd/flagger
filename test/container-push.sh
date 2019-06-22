#!/usr/bin/env bash

set -o errexit

BRANCH_COMMIT=${CIRCLE_BRANCH}-$(echo ${CIRCLE_SHA1} | head -c7)

if [[ -z "$DOCKER_PASS" ]]; then
    echo "No Docker Hub credentials, skipping image push";
else
    echo $DOCKER_PASS | docker login -u=$DOCKER_USER --password-stdin;
    docker tag test/flagger:latest weaveworks/flagger:${BRANCH_COMMIT};
    docker push weaveworks/flagger:${BRANCH_COMMIT};
fi

if [[ -z "$CIRCLE_TAG" ]]; then
    echo "Not a release, skipping image push";
else
    echo $DOCKER_PASS | docker login -u=$DOCKER_USER --password-stdin;
    docker tag test/flagger:latest weaveworks/flagger:${CIRCLE_TAG};
    docker tag test/flagger-loadtester:latest weaveworks/flagger-loadtester:${CIRCLE_TAG};
    docker push weaveworks/flagger:${CIRCLE_TAG};
    docker push weaveworks/flagger-loadtester:${CIRCLE_TAG};
fi
