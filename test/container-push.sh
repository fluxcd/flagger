#!/usr/bin/env bash

set -o errexit


push () {
    echo $DOCKER_PASS | docker login -u=$DOCKER_USER --password-stdin

    if [[ -z "$CIRCLE_TAG" ]]; then
        BRANCH_COMMIT=${CIRCLE_BRANCH}-$(echo ${CIRCLE_SHA1} | head -c7);
        docker tag test/flagger:latest weaveworks/flagger:${BRANCH_COMMIT};
        docker push weaveworks/flagger:${BRANCH_COMMIT};
    else
        docker tag test/flagger:latest weaveworks/flagger:${CIRCLE_TAG};
        docker tag test/flagger-loadtester:latest weaveworks/flagger-loadtester:${CIRCLE_TAG};
        docker push weaveworks/flagger:${CIRCLE_TAG};
        docker push weaveworks/flagger-loadtester:${CIRCLE_TAG};
    fi
}

if [[ -z "$DOCKER_PASS" ]]; then
    echo "No Docker Hub credentials, skipping image push";
else
    push
fi

