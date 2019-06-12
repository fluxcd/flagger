FROM golang:1.12

RUN mkdir -p /go/src/github.com/weaveworks/flagger/

WORKDIR /go/src/github.com/weaveworks/flagger

COPY . .

#RUN GO111MODULE=on go mod download

RUN GIT_COMMIT=$(git rev-list -1 HEAD) && \
     GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags "-s -w \
    -X github.com/weaveworks/flagger/pkg/version.REVISION=${GIT_COMMIT}" \
    -a -installsuffix cgo -o flagger ./cmd/flagger/*

FROM alpine:3.9

RUN addgroup -S flagger \
    && adduser -S -g flagger flagger \
    && apk --no-cache add ca-certificates

WORKDIR /home/flagger

COPY --from=0 /go/src/github.com/weaveworks/flagger/flagger .

RUN chown -R flagger:flagger ./

USER flagger

ENTRYPOINT ["./flagger"]

