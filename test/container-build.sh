#!/usr/bin/env bash

set -o errexit

cp /tmp/workspace/flagger .
chmod +x flagger

docker build -t weaveworks/flagger:latest . -f ./test/Dockerfile.ci

BRANCH_COMMIT=${CIRCLE_BRANCH}-$(echo ${CIRCLE_SHA1} | head -c7)

if [[ -z "$DOCKER_PASS" ]]; then
    echo "No Docker Hub credentials, skipping image push";
else
    echo $DOCKER_PASS | docker login -u=$DOCKER_USER --password-stdin;
    docker tag weaveworks/flagger:latest weaveworks/flagger:${BRANCH_COMMIT};
    docker push weaveworks/flagger:${BRANCH_COMMIT};
fi

if [[ -z "$CIRCLE_TAG" ]]; then
    echo "Not a release, skipping image push";
else
    echo $DOCKER_PASS | docker login -u=$DOCKER_USER --password-stdin;
    docker tag weaveworks/flagger:latest weaveworks/flagger:${CIRCLE_TAG};
    docker push weaveworks/flagger:${CIRCLE_TAG};
fi
