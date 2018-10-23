FROM golang:1.10

RUN mkdir -p /go/src/github.com/stefanprodan/flagger/

WORKDIR /go/src/github.com/stefanprodan/flagger

COPY . .

RUN GIT_COMMIT=$(git rev-list -1 HEAD) && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w \
    -X github.com/stefanprodan/flagger/pkg/version.REVISION=${GIT_COMMIT}" \
    -a -installsuffix cgo -o flagger ./cmd/flagger/*

FROM alpine:3.8

RUN addgroup -S flagger \
    && adduser -S -g flagger flagger \
    && apk --no-cache add ca-certificates

WORKDIR /home/flagger

COPY --from=0 /go/src/github.com/stefanprodan/flagger/flagger .

RUN chown -R flagger:flagger ./

USER flagger

ENTRYPOINT ["./flagger"]

