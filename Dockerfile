FROM golang:1.15-alpine as builder

ARG TARGETPLATFORM

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
RUN CGO_ENABLED=0 go build -a -o flagger ./cmd/flagger

FROM alpine:3.12

RUN apk --no-cache add ca-certificates

USER nobody

COPY --from=builder --chown=nobody:nobody /workspace/flagger .

ENTRYPOINT ["./flagger"]
