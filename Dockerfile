FROM golang:1.10

RUN mkdir -p /go/src/github.com/stefanprodan/flagger/

WORKDIR /go/src/github.com/stefanprodan/flagger

COPY . .

RUN GIT_COMMIT=$(git rev-list -1 HEAD) && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w \
    -X github.com/stefanprodan/flagger/pkg/version.REVISION=${GIT_COMMIT}" \
    -a -installsuffix cgo -o flagger ./cmd/flagger/*

FROM alpine:3.8

RUN addgroup -S app \
    && adduser -S -g app app \
    && apk --no-cache add ca-certificates

WORKDIR /home/app

COPY --from=0 /go/src/github.com/stefanprodan/flagger/flagger .

RUN chown -R app:app ./

USER app

ENTRYPOINT ["./flagger"]

