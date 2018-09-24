FROM golang:1.10

RUN mkdir -p /go/src/github.com/stefanprodan/steerer/

WORKDIR /go/src/github.com/stefanprodan/steerer

COPY . .

RUN VERSION=$(git describe --all --exact-match `git rev-parse HEAD` | grep tags | sed 's/tags\///') && \
    GIT_COMMIT=$(git rev-list -1 HEAD) && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w \
    -X github.com/stefanprodan/steerer/pkg/version.VERSION=${VERSION} \
    -X github.com/stefanprodan/steerer/pkg/version.REVISION=${GIT_COMMIT}" \
    -a -installsuffix cgo -o steerer ./cmd/controller/*

FROM alpine:3.8

RUN addgroup -S app \
    && adduser -S -g app app \
    && apk --no-cache add ca-certificates

WORKDIR /home/app

COPY --from=0 /go/src/github.com/stefanprodan/steerer/steerer .

RUN chown -R app:app ./

USER app

ENTRYPOINT ["./steerer"]
CMD ["-logtostderr"]
