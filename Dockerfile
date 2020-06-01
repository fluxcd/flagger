FROM alpine:3.12.0

RUN apk --no-cache add ca-certificates

WORKDIR /home/flagger

USER nobody

COPY --chown=nobody:nobody /bin/flagger .

ENTRYPOINT ["./flagger"]
