ARG GO_VERSION=1.24
ARG XX_VERSION=1.6.1

FROM --platform=$BUILDPLATFORM tonistiigi/xx:${XX_VERSION} AS xx
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder

# copy build utilities
COPY --from=xx / /

ARG TARGETPLATFORM
ARG REVISON

WORKDIR /workspace

# copy modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# cache modules
RUN go mod download

# copy source code
COPY cmd/ cmd/
COPY pkg/ pkg/

# build
ENV CGO_ENABLED=0
RUN xx-go build \
    -ldflags "-s -w -X github.com/fluxcd/flagger/pkg/version.REVISION=${REVISON}" \
    -a -o flagger ./cmd/flagger

FROM alpine:3.21

RUN apk --no-cache add ca-certificates

USER nobody

COPY --from=builder --chown=nobody:nobody /workspace/flagger .

ENTRYPOINT ["./flagger"]
