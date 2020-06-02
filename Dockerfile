FROM alpine:3.12

RUN apk --no-cache add ca-certificates

USER nobody

COPY --chown=nobody:nobody /bin/flagger .

ENTRYPOINT ["./flagger"]
